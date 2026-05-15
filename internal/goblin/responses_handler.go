package goblin

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"
)

type ResponsesHandler struct {
	log        *slog.Logger
	logWithSrc *slog.Logger
	cfg        *GoblinConfig
}

func newResponsesHandler(cfg *GoblinConfig) *ResponsesHandler {
	log, logWithSrc := newComponentLogger("responses")
	return &ResponsesHandler{log: log, logWithSrc: logWithSrc, cfg: cfg}
}

func (h *ResponsesHandler) handlePostResponses() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "" {
			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil || mediaType != "application/json" {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		var req ResponsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.log.Error("failed to decode request", "error", err)
			http.Error(w, fmt.Sprintf("invalid JSON body: %s", err), http.StatusBadRequest)
			return
		}

		if !req.Stream {
			http.Error(w, "stream must be true", http.StatusBadRequest)
			return
		}

		providerName, modelName, err := parseModelSlug(req.Model)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		provider, ok := h.cfg.Providers[providerName]
		if !ok {
			h.log.Error("unknown provider", "provider", providerName)
			http.Error(w, fmt.Sprintf("unknown provider %q", providerName), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		flusher, canFlush := w.(http.Flusher)
		responseID, err := generateResponseID()
		if err != nil {
			http.Error(w, "failed to generate response ID", http.StatusInternalServerError)
			return
		}

		if err := emitCreated(w, responseID); err != nil {
			h.logWithSrc.Error("failed to write SSE event", "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}

		h.processRequest(w, r, req, provider, modelName, responseID, flusher, canFlush)
	}
}

func (h *ResponsesHandler) processRequest(
	w io.Writer,
	r *http.Request,
	req ResponsesRequest,
	provider Provider,
	modelName string,
	responseID string,
	flusher http.Flusher,
	canFlush bool,
) {
	messages := toChatMessages(req.Input)
	messages = prependInstructions(messages, req.Instructions)
	tools := convertTools(req.Tools)

	chatReq := ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
		StreamOptions: &StreamOptions{
			IncludeUsage: true,
		},
	}
	if len(tools) > 0 {
		chatReq.Tools = tools
		chatReq.ToolChoice = req.ToolChoice
		if req.ParallelToolCalls {
			chatReq.ParallelToolCalls = new(true)
		}
	}

	chatBody, err := json.Marshal(chatReq)
	if err != nil {
		h.logWithSrc.Error("failed to marshal chat request", "error", err)
		if err := emitFailed(w, "upstream_error", err.Error()); err != nil {
			h.logWithSrc.Error("failed to emit error event", "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}
		return
	}

	chatURL := provider.BaseURL + "/chat/completions"
	upstreamReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, chatURL, bytes.NewReader(chatBody))
	if err != nil {
		h.logWithSrc.Error("failed to create upstream request", "error", err)
		if err := emitFailed(w, "upstream_error", err.Error()); err != nil {
			h.logWithSrc.Error("failed to emit error event", "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}
		return
	}
	upstreamReq.Header.Set("Content-Type", "application/json")
	upstreamReq.Header.Set("Accept", "text/event-stream")
	if provider.EnvKey != "" {
		apiKey := resolveAPIKey(provider.EnvKey)
		if apiKey != "" {
			upstreamReq.Header.Set("Authorization", "Bearer "+apiKey)
		}
	}

	upstreamResp, err := http.DefaultClient.Do(upstreamReq)
	if err != nil {
		h.log.Error("upstream request failed", "error", err)
		if err := emitFailed(w, "upstream_error", err.Error()); err != nil {
			h.logWithSrc.Error("failed to emit error event", "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}
		return
	}
	defer func() {
		if err := upstreamResp.Body.Close(); err != nil {
			h.logWithSrc.Error("failed to close upstream response body", "error", err)
		}
	}()

	if upstreamResp.StatusCode != http.StatusOK {
		errBody, readErr := io.ReadAll(upstreamResp.Body)
		if readErr != nil {
			h.logWithSrc.Error("failed to read upstream error body", "error", readErr)
		}
		h.log.Error("upstream returned error", "status", upstreamResp.StatusCode, "body", string(errBody))
		if err := emitFailed(w, "upstream_error", fmt.Sprintf("HTTP %d: %s", upstreamResp.StatusCode, string(errBody))); err != nil {
			h.logWithSrc.Error("failed to emit error event", "error", err)
			return
		}
		if canFlush {
			flusher.Flush()
		}
		return
	}

	h.processUpstreamStream(w, upstreamResp.Body, responseID, flusher, canFlush)
}

func (h *ResponsesHandler) processUpstreamStream(
	w io.Writer,
	upstreamBody io.Reader,
	responseID string,
	flusher http.Flusher,
	canFlush bool,
) {
	scanner := bufio.NewScanner(upstreamBody)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	var (
		currentItemID        string
		accumulatedContent   strings.Builder
		accumulatedToolCalls = make(map[int]*toolCallAccum)
		hasContent           bool
		hasToolCalls         bool
		finalUsage           *ResponseUsage
	)

	startNewMessage := func() error {
		id, err := generateMessageID()
		if err != nil {
			return err
		}
		currentItemID = id
		accumulatedContent.Reset()
		accumulatedToolCalls = make(map[int]*toolCallAccum)
		hasContent = false
		hasToolCalls = false
		return nil
	}

	flushCurrentItem := func() error {
		if currentItemID == "" {
			return nil
		}

		if hasContent {
			item := OutputItem{
				Type: "message",
				Role: "assistant",
				Content: []ContentItem{
					{Type: "output_text", Text: accumulatedContent.String()},
				},
			}
			if err := emitOutputItemDone(w, item, currentItemID); err != nil {
				return fmt.Errorf("emit output item done: %w", err)
			}
		}

		if hasToolCalls && len(accumulatedToolCalls) > 0 {
			for _, tc := range accumulatedToolCalls {
				item := OutputItem{
					Type:      "function_call",
					Name:      tc.name,
					Arguments: tc.arguments,
					CallID:    tc.id,
				}
				tcItemID, err := generateFunctionCallItemID()
				if err != nil {
					return fmt.Errorf("generate function call item ID: %w", err)
				}
				if err := emitOutputItemAdded(w, item, tcItemID); err != nil {
					return fmt.Errorf("emit output item added: %w", err)
				}
				if err := emitOutputItemDone(w, item, tcItemID); err != nil {
					return fmt.Errorf("emit output item done: %w", err)
				}
			}
		}

		currentItemID = ""
		return nil
	}

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")
		data = strings.TrimSpace(data)

		if data == "[DONE]" {
			continue
		}

		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			h.log.Debug("failed to parse chunk", "data", data, "error", err)
			continue
		}

		if chunk.Usage != nil {
			finalUsage = &ResponseUsage{
				InputTokens:           chunk.Usage.PromptTokens,
				OutputTokens:          chunk.Usage.CompletionTokens,
				TotalTokens:           chunk.Usage.TotalTokens,
				CachedInputTokens:     0,
				ReasoningOutputTokens: 0,
			}
			if chunk.Usage.PromptTokensDetails != nil {
				finalUsage.CachedInputTokens = chunk.Usage.PromptTokensDetails.CachedTokens
			}
			if chunk.Usage.CompletionTokensDetails != nil {
				finalUsage.ReasoningOutputTokens = chunk.Usage.CompletionTokensDetails.ReasoningTokens
			}
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			if delta.Content != nil && *delta.Content != "" {
				if currentItemID == "" {
					if err := startNewMessage(); err != nil {
						h.log.Error("failed to generate message ID", "error", err)
						return
					}
					item := OutputItem{
						Type:    "message",
						Role:    "assistant",
						Content: []ContentItem{},
					}
					if err := emitOutputItemAdded(w, item, currentItemID); err != nil {
						h.log.Error("failed to write SSE event", "error", err)
						return
					}
				}
				accumulatedContent.WriteString(*delta.Content)
				hasContent = true
				if err := emitOutputTextDelta(w, *delta.Content); err != nil {
					h.log.Error("failed to write SSE event", "error", err)
					return
				}
				if canFlush {
					flusher.Flush()
				}
			}

			if len(delta.ToolCalls) > 0 {
				if currentItemID == "" {
					if err := startNewMessage(); err != nil {
						h.log.Error("failed to generate message ID", "error", err)
						return
					}
				}
				for _, tcDelta := range delta.ToolCalls {
					idx := tcDelta.Index

					if _, exists := accumulatedToolCalls[idx]; !exists {
						fallbackID := tcDelta.ID
						if fallbackID == "" {
							id, err := generateCallID()
							if err != nil {
								h.log.Error("failed to generate call ID", "error", err)
								return
							}
							fallbackID = id
						}
						accumulatedToolCalls[idx] = &toolCallAccum{
							id:        fallbackID,
							name:      "",
							arguments: "",
						}
					}

					tc := accumulatedToolCalls[idx]

					if tcDelta.ID != "" {
						tc.id = tcDelta.ID
					}
					if tcDelta.Function != nil {
						if tcDelta.Function.Name != "" {
							tc.name += tcDelta.Function.Name
						}
						if tcDelta.Function.Arguments != "" {
							tc.arguments += tcDelta.Function.Arguments
							hasToolCalls = true
							if canFlush {
								flusher.Flush()
							}
						}
					}
				}
			}

			if choice.FinishReason != nil && *choice.FinishReason != "" {
				if err := flushCurrentItem(); err != nil {
					h.logWithSrc.Error("failed to flush current item", "error", err)
					return
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		h.logWithSrc.Error("error reading upstream stream", "error", err)
	}

	if err := flushCurrentItem(); err != nil {
		h.logWithSrc.Error("failed to flush current item", "error", err)
	}
	if err := emitCompleted(w, responseID, finalUsage); err != nil {
		h.logWithSrc.Error("failed to write SSE event", "error", err)
		return
	}
	if canFlush {
		flusher.Flush()
	}
}

type toolCallAccum struct {
	id        string
	name      string
	arguments string
}

func parseModelSlug(slug string) (string, string, error) {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid model slug %q (expected provider/model)", slug)
	}
	return parts[0], parts[1], nil
}

func resolveAPIKey(envKey string) string {
	if envKey == "" {
		return ""
	}
	return os.Getenv(envKey)
}

const hexChars = "0123456789abcdef"

const base62 = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, n*2)
	for i, v := range b {
		out[i*2] = hexChars[v>>4]
		out[i*2+1] = hexChars[v&0x0f]
	}
	return string(out), nil
}

// Sample: resp_0309c0d6cb4ff519016a032143c2288191b3759a2e031f11b2
func generateResponseID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "resp_" + h, nil
}

// Sample: msg_0309c0d6cb4ff519016a03214e4e7c8191bf036ec8113050a7
func generateMessageID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "msg_" + h, nil
}

// Sample: fc_0309c0d6cb4ff519016a032152eb1c819182b3994c61de195b
func generateFunctionCallItemID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "fc_" + h, nil
}

// Sample: call_ueWI5DaDk7YLNXdK8uBWyUTg
//
// Uses rejection sampling (byte < 248 → byte % 62) for unbiased
// 24-char [a-zA-Z0-9] output.
func generateCallID() (string, error) {
	out := make([]byte, 24)
	i := 0
	for i < 24 {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, v := range buf {
			if v < 248 {
				out[i] = base62[v%62]
				i++
				if i == 24 {
					break
				}
			}
		}
	}
	return "call_" + string(out), nil
}

func writeSSE(w io.Writer, event string, data map[string]any) error {
	line, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(w, "event: "+event+"\n"); err != nil {
		return err
	}
	_, err = io.WriteString(w, "data: "+string(line)+"\n\n")
	return err
}

func emitCreated(w io.Writer, id string) error {
	return writeSSE(w, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id": id,
		},
	})
}

func emitCompleted(w io.Writer, id string, usage *ResponseUsage) error {
	resp := map[string]any{
		"id":     id,
		"status": "completed",
	}
	if usage != nil {
		resp["usage"] = map[string]any{
			"input_tokens":            usage.InputTokens,
			"output_tokens":           usage.OutputTokens,
			"total_tokens":            usage.TotalTokens,
			"cached_input_tokens":     usage.CachedInputTokens,
			"reasoning_output_tokens": usage.ReasoningOutputTokens,
		}
	}
	return writeSSE(w, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": resp,
	})
}

func emitFailed(w io.Writer, code, message string) error {
	id, err := generateResponseID()
	if err != nil {
		return err
	}
	return writeSSE(w, "response.failed", map[string]any{
		"type": "response.failed",
		"response": map[string]any{
			"id":     id,
			"status": "failed",
			"error": map[string]string{
				"code":    code,
				"message": message,
			},
		},
	})
}

func emitOutputItemAdded(w io.Writer, item OutputItem, itemID string) error {
	itemMap := map[string]any{
		"id":     itemID,
		"type":   item.Type,
		"status": "in_progress",
	}
	switch item.Type {
	case "message":
		itemMap["role"] = item.Role
		itemMap["content"] = item.Content
	case "function_call":
		itemMap["name"] = item.Name
		itemMap["arguments"] = item.Arguments
		itemMap["call_id"] = item.CallID
	}
	return writeSSE(w, "response.output_item.added", map[string]any{
		"type": "response.output_item.added",
		"item": itemMap,
	})
}

func emitOutputItemDone(w io.Writer, item OutputItem, itemID string) error {
	itemMap := map[string]any{
		"id":     itemID,
		"type":   item.Type,
		"status": "completed",
	}
	switch item.Type {
	case "message":
		itemMap["role"] = item.Role
		itemMap["content"] = item.Content
	case "function_call":
		itemMap["name"] = item.Name
		itemMap["arguments"] = item.Arguments
		itemMap["call_id"] = item.CallID
	}
	return writeSSE(w, "response.output_item.done", map[string]any{
		"type": "response.output_item.done",
		"item": itemMap,
	})
}

func emitOutputTextDelta(w io.Writer, delta string) error {
	return writeSSE(w, "response.output_text.delta", map[string]any{
		"type":  "response.output_text.delta",
		"delta": delta,
	})
}

func toChatMessages(input []InputItem) []ChatMessage {
	if len(input) == 0 {
		return nil
	}

	messages := make([]ChatMessage, 0, len(input))

	for _, item := range input {
		switch item.Type {
		case "message":
			msg := convertMessageItem(item)
			if msg != nil {
				messages = append(messages, *msg)
			}

		case "function_call":
			msg := ChatMessage{
				Role:    "assistant",
				Content: nil,
				ToolCalls: []ToolCall{{
					ID:   item.CallID,
					Type: "function",
					Function: ToolCallFunc{
						Name:      item.Name,
						Arguments: item.Arguments,
					},
				}},
			}
			messages = append(messages, msg)

		case "function_call_output":
			text := outputToText(&item)
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    strOrNil(text),
				ToolCallID: strOrNil(item.CallID),
			})

		case "custom_tool_call_output":
			text := outputToText(&item)
			messages = append(messages, ChatMessage{
				Role:       "tool",
				Content:    strOrNil(text),
				ToolCallID: strOrNil(item.CallID),
			})
		}
	}

	return messages
}

func convertMessageItem(item InputItem) *ChatMessage {
	switch item.Role {
	case "system":
		text := joinInputText(item.Content)
		return &ChatMessage{Role: "system", Content: strOrNil(text)}

	case "user":
		textParts := filterInputText(item.Content)
		imageParts := filterInputImages(item.Content)

		if len(imageParts) == 0 {
			text := joinInputText(item.Content)
			return &ChatMessage{Role: "user", Content: strOrNil(text)}
		}

		parts := make([]ContentPart, 0, len(textParts)+len(imageParts))
		for _, t := range textParts {
			parts = append(parts, ContentPart{Type: "text", Text: t.Text})
		}
		for _, img := range imageParts {
			parts = append(parts, ContentPart{
				Type: "image_url",
				ImageURL: &ImageURL{
					URL:    img.ImageURL,
					Detail: img.Detail,
				},
			})
		}
		return &ChatMessage{Role: "user", Content: parts}

	case "assistant":
		text := joinOutputText(item.Content)
		return &ChatMessage{Role: "assistant", Content: strOrNil(text)}

	default:
		return nil
	}
}

func prependInstructions(messages []ChatMessage, instructions string) []ChatMessage {
	if instructions == "" {
		return messages
	}

	result := make([]ChatMessage, 0, len(messages)+1)
	result = append(result, ChatMessage{
		Role:    "system",
		Content: strOrNil(instructions),
	})
	result = append(result, messages...)
	return result
}

func convertTools(tools []Tool) []ChatTool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]ChatTool, 0, len(tools))
	for _, t := range tools {
		switch t.Type {
		case "function":
			if t.Function != nil {
				ct := ChatTool{
					Type: "function",
					Function: ChatToolFunction{
						Name:        t.Function.Name,
						Description: t.Function.Description,
						Parameters:  t.Function.Parameters,
					},
				}
				if t.Function.Strict {
					ct.Function.Strict = new(true)
				}
				result = append(result, ct)
			}
		case "namespace":
			for _, child := range t.Tools {
				if child.Type == "function" && child.Function != nil {
					ct := ChatTool{
						Type: "function",
						Function: ChatToolFunction{
							Name:        child.Function.Name,
							Description: child.Function.Description,
							Parameters:  child.Function.Parameters,
						},
					}
					if child.Function.Strict {
						ct.Function.Strict = new(true)
					}
					result = append(result, ct)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func joinInputText(items []ContentItem) string {
	var b strings.Builder
	for _, c := range items {
		if c.Type == "input_text" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func joinOutputText(items []ContentItem) string {
	var b strings.Builder
	for _, c := range items {
		if c.Type == "output_text" {
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func filterInputText(items []ContentItem) []ContentItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]ContentItem, 0, len(items))
	for _, c := range items {
		if c.Type == "input_text" {
			result = append(result, c)
		}
	}
	return result
}

func filterInputImages(items []ContentItem) []ContentItem {
	if len(items) == 0 {
		return nil
	}
	result := make([]ContentItem, 0, len(items))
	for _, c := range items {
		if c.Type == "input_image" {
			result = append(result, c)
		}
	}
	return result
}

func outputToText(item *InputItem) string {
	switch v := item.Output.(type) {
	case string:
		if v == "" {
			return ""
		}
		if v[0] == '[' {
			var items []FunctionCallOutputContentItem
			if err := json.Unmarshal([]byte(v), &items); err == nil {
				return joinFunctionCallOutputText(items)
			}
		}
		return v
	case []any:
		items := make([]FunctionCallOutputContentItem, 0, len(v))
		for _, el := range v {
			if m, ok := el.(map[string]any); ok {
				var c FunctionCallOutputContentItem
				if t, ok := m["type"].(string); ok {
					c.Type = t
				}
				if t, ok := m["text"].(string); ok {
					c.Text = t
				}
				items = append(items, c)
			}
		}
		return joinFunctionCallOutputText(items)
	}
	return ""
}

func joinFunctionCallOutputText(items []FunctionCallOutputContentItem) string {
	var b strings.Builder
	for _, c := range items {
		if c.Type == "input_text" {
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(c.Text)
		}
	}
	return b.String()
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
