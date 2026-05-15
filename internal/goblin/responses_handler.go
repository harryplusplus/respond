package goblin

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

type responsesHandler struct {
	log        *slog.Logger
	logWithSrc *slog.Logger
	cfg        *GoblinConfig
}

type sseWriter struct {
	writer  io.Writer
	flusher http.Flusher
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	f, _ := w.(http.Flusher)
	return &sseWriter{writer: w, flusher: f}
}

func (s *sseWriter) emit(event string, data map[string]any) error {
	line, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(s.writer, "event: "+event+"\n"); err != nil {
		return err
	}
	if _, err := io.WriteString(s.writer, "data: "+string(line)+"\n\n"); err != nil {
		return err
	}
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

func (s *sseWriter) emitCreated(id string) error {
	return s.emit("response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id": id,
		},
	})
}

func (s *sseWriter) emitCompleted(id string, usage *responses.ResponseUsage) error {
	resp := map[string]any{
		"id":     id,
		"status": "completed",
	}
	if usage != nil {
		resp["usage"] = map[string]any{
			"input_tokens":            usage.InputTokens,
			"output_tokens":           usage.OutputTokens,
			"total_tokens":            usage.TotalTokens,
			"cached_input_tokens":     usage.InputTokensDetails.CachedTokens,
			"reasoning_output_tokens": usage.OutputTokensDetails.ReasoningTokens,
		}
	}
	return s.emit("response.completed", map[string]any{
		"type":     "response.completed",
		"response": resp,
	})
}

func (s *sseWriter) emitFailed(id, code, message string) error {
	return s.emit("response.failed", map[string]any{
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

func (s *sseWriter) emitOutputItemAdded(item map[string]any, itemID string) error {
	item["id"] = itemID
	item["status"] = "in_progress"
	return s.emit("response.output_item.added", map[string]any{
		"type": "response.output_item.added",
		"item": item,
	})
}

func (s *sseWriter) emitOutputItemDone(item map[string]any, itemID string) error {
	item["id"] = itemID
	item["status"] = "completed"
	return s.emit("response.output_item.done", map[string]any{
		"type": "response.output_item.done",
		"item": item,
	})
}

func (s *sseWriter) emitOutputTextDelta(delta string) error {
	return s.emit("response.output_text.delta", map[string]any{
		"type":  "response.output_text.delta",
		"delta": delta,
	})
}

func newResponsesHandler(cfg *GoblinConfig) *responsesHandler {
	log, logWithSrc := newComponentLogger("responses")
	return &responsesHandler{log: log, logWithSrc: logWithSrc, cfg: cfg}
}

func (h *responsesHandler) handlePostResponses() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if ct != "" {
			mediaType, _, err := mime.ParseMediaType(ct)
			if err != nil || mediaType != "application/json" {
				http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		var req responses.ResponseNewParams
		if err := json.Unmarshal(body, &req); err != nil {
			h.log.Error("failed to decode request", "error", err)
			http.Error(w, fmt.Sprintf("invalid JSON body: %s", err), http.StatusBadRequest)
			return
		}

		// ResponseNewParams has no Stream field (SDK injects it via NewStreaming),
		// so check it from the same body — body is valid JSON proven above.
		var raw struct {
			Stream bool `json:"stream"`
		}
		if err := json.Unmarshal(body, &raw); err != nil {
			http.Error(w, "invalid JSON body", http.StatusBadRequest)
			return
		}
		if !raw.Stream {
			http.Error(w, "stream must be true", http.StatusBadRequest)
			return
		}

		providerName, modelName, err := parseModelSlug(string(req.Model))
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

		// All pre-200 validation passed. From here on, responses are SSE events only.
		responseID, err := generateResponseID()
		if err != nil {
			http.Error(w, "failed to generate response ID", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)

		sw := newSSEWriter(w)

		if err := sw.emitCreated(responseID); err != nil {
			h.logWithSrc.Error("failed to write initial SSE event", "error", err)
			return
		}

		usage, err := h.streamResponse(r.Context(), sw, req, provider, modelName, responseID)

		var terminalErr error
		if err != nil {
			terminalErr = sw.emitFailed(responseID, "upstream_error", err.Error())
		} else {
			terminalErr = sw.emitCompleted(responseID, usage)
		}
		if terminalErr != nil {
			h.logWithSrc.Error("failed to emit terminal SSE event", "error", terminalErr)
		}
	}
}

func (h *responsesHandler) streamResponse(
	ctx context.Context,
	sw *sseWriter,
	req responses.ResponseNewParams,
	provider Provider,
	modelName string,
	responseID string,
) (*responses.ResponseUsage, error) {
	messages := h.toChatCompletionMessages(req)

	opts := []option.RequestOption{
		option.WithBaseURL(provider.BaseURL),
	}
	if provider.EnvKey != "" {
		if apiKey := os.Getenv(provider.EnvKey); apiKey != "" {
			opts = append(opts, option.WithAPIKey(apiKey))
		}
	}

	params := openai.ChatCompletionNewParams{
		Model:    shared.ChatModel(modelName),
		Messages: messages,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		},
	}

	if len(req.Tools) > 0 {
		params.Tools = toChatTools(req.Tools)
		if v := req.ToolChoice; v.OfToolChoiceMode.Valid() {
			params.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
				OfAuto: param.NewOpt(string(v.OfToolChoiceMode.Value)),
			}
		}
		if v := req.ParallelToolCalls; v.Valid() && v.Value {
			params.ParallelToolCalls = openai.Bool(true)
		}
	}

	svc := openai.NewChatCompletionService(opts...)
	stream := svc.NewStreaming(ctx, params)
	if stream.Err() != nil {
		return nil, fmt.Errorf("upstream request: %w", stream.Err())
	}

	return h.processUpstreamStream(sw, stream, responseID)
}

func (h *responsesHandler) processUpstreamStream(
	sw *sseWriter,
	stream *ssestream.Stream[openai.ChatCompletionChunk],
	responseID string,
) (*responses.ResponseUsage, error) {
	var (
		currentItemID        string
		accumulatedContent   strings.Builder
		accumulatedToolCalls = make(map[int]*toolCallAccum)
		hasContent           bool
		hasToolCalls         bool
		finalUsage           *responses.ResponseUsage
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
			if err := sw.emitOutputItemDone(map[string]any{
				"type":    "message",
				"role":    "assistant",
				"content": []map[string]any{{"type": "output_text", "text": accumulatedContent.String()}},
			}, currentItemID); err != nil {
				return fmt.Errorf("emit output item done: %w", err)
			}
		}

		if hasToolCalls && len(accumulatedToolCalls) > 0 {
			for _, tc := range accumulatedToolCalls {
				item := map[string]any{
					"type":      "function_call",
					"name":      tc.name,
					"arguments": tc.arguments,
					"call_id":   tc.id,
				}
				tcItemID, err := generateFunctionCallItemID()
				if err != nil {
					return fmt.Errorf("generate function call item ID: %w", err)
				}
				if err := sw.emitOutputItemAdded(item, tcItemID); err != nil {
					return fmt.Errorf("emit output item added: %w", err)
				}
				if err := sw.emitOutputItemDone(item, tcItemID); err != nil {
					return fmt.Errorf("emit output item done: %w", err)
				}
			}
		}

		currentItemID = ""
		return nil
	}

	for stream.Next() {
		chunk := stream.Current()

		if chunk.JSON.Usage.Valid() {
			u := chunk.Usage
			usage := &responses.ResponseUsage{
				InputTokens:  u.PromptTokens,
				OutputTokens: u.CompletionTokens,
				TotalTokens:  u.TotalTokens,
			}
			if u.CompletionTokensDetails.ReasoningTokens > 0 {
				usage.OutputTokensDetails.ReasoningTokens = u.CompletionTokensDetails.ReasoningTokens
			}
			finalUsage = usage
		}

		for _, choice := range chunk.Choices {
			delta := choice.Delta

			if delta.Content != "" {
				if currentItemID == "" {
					if err := startNewMessage(); err != nil {
						return nil, fmt.Errorf("start new message: %w", err)
					}
					if err := sw.emitOutputItemAdded(map[string]any{
						"type":    "message",
						"role":    "assistant",
						"content": []map[string]any{},
					}, currentItemID); err != nil {
						return nil, fmt.Errorf("emit output item added: %w", err)
					}
				}
				accumulatedContent.WriteString(delta.Content)
				hasContent = true
				if err := sw.emitOutputTextDelta(delta.Content); err != nil {
					return nil, fmt.Errorf("emit output text delta: %w", err)
				}
			}

			if len(delta.ToolCalls) > 0 {
				if currentItemID == "" {
					if err := startNewMessage(); err != nil {
						return nil, fmt.Errorf("start new message: %w", err)
					}
				}
				for _, tcDelta := range delta.ToolCalls {
					idx := int(tcDelta.Index)

					if _, exists := accumulatedToolCalls[idx]; !exists {
						fallbackID := tcDelta.ID
						if fallbackID == "" {
							id, err := generateCallID()
							if err != nil {
								return nil, fmt.Errorf("generate call ID: %w", err)
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
					if tcDelta.Function.Name != "" {
						tc.name += tcDelta.Function.Name
					}
					if tcDelta.Function.Arguments != "" {
						tc.arguments += tcDelta.Function.Arguments
						hasToolCalls = true
					}
				}
			}

			if choice.FinishReason != "" {
				if err := flushCurrentItem(); err != nil {
					return nil, fmt.Errorf("flush current item: %w", err)
				}
			}
		}
	}

	if err := stream.Err(); err != nil {
		h.logWithSrc.Error("error reading upstream stream", "error", err)
		return nil, fmt.Errorf("upstream stream error: %w", err)
	}

	if err := flushCurrentItem(); err != nil {
		return nil, fmt.Errorf("flush remaining item: %w", err)
	}

	return finalUsage, nil
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

func generateResponseID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "resp_" + h, nil
}

func generateMessageID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "msg_" + h, nil
}

func generateFunctionCallItemID() (string, error) {
	h, err := randomHex(25)
	if err != nil {
		return "", err
	}
	return "fc_" + h, nil
}

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

func (h *responsesHandler) toChatCompletionMessages(req responses.ResponseNewParams) []openai.ChatCompletionMessageParamUnion {
	instructions := req.Instructions
	input := req.Input

	// Count input items for capacity.
	itemCount := 0
	if input.OfInputItemList != nil {
		itemCount = len(input.OfInputItemList)
	} else if input.OfString.Valid() {
		itemCount = 1
	}

	capacity := itemCount
	if instructions.Valid() {
		capacity++
	}

	messages := make([]openai.ChatCompletionMessageParamUnion, 0, capacity)

	if instructions.Valid() {
		messages = append(messages, openai.ChatCompletionMessageParamUnion{
			OfSystem: &openai.ChatCompletionSystemMessageParam{
				Content: openai.ChatCompletionSystemMessageParamContentUnion{
					OfString: param.NewOpt(instructions.Value),
				},
			},
		})
	}

	// String input → single user message.
	if input.OfString.Valid() {
		return append(messages, openai.ChatCompletionMessageParamUnion{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfString: param.NewOpt(input.OfString.Value),
				},
			},
		})
	}

	if input.OfInputItemList == nil {
		return messages
	}

	for _, item := range input.OfInputItemList {
		p := h.toChatMessageParam(item)
		if p != nil {
			messages = append(messages, *p)
		}
	}

	return messages
}

func (h *responsesHandler) toChatMessageParam(item responses.ResponseInputItemUnionParam) *openai.ChatCompletionMessageParamUnion {
	if item.OfMessage != nil {
		return convertMessageItem(item.OfMessage)
	}
	if item.OfFunctionCall != nil {
		fc := item.OfFunctionCall
		asst := openai.ChatCompletionAssistantMessageParam{
			ToolCalls: []openai.ChatCompletionMessageToolCallUnionParam{{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: fc.CallID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      fc.Name,
						Arguments: fc.Arguments,
					},
				},
			}},
		}
		return &openai.ChatCompletionMessageParamUnion{OfAssistant: &asst}
	}
	if item.OfFunctionCallOutput != nil {
		fco := item.OfFunctionCallOutput
		text := outputToText(fco.Output)
		return &openai.ChatCompletionMessageParamUnion{
			OfTool: &openai.ChatCompletionToolMessageParam{
				Content: openai.ChatCompletionToolMessageParamContentUnion{
					OfString: param.NewOpt(text),
				},
				ToolCallID: fco.CallID,
			},
		}
	}
	if item.OfCustomToolCallOutput != nil {
		ctco := item.OfCustomToolCallOutput
		text := customOutputToText(ctco.Output)
		return &openai.ChatCompletionMessageParamUnion{
			OfTool: &openai.ChatCompletionToolMessageParam{
				Content: openai.ChatCompletionToolMessageParamContentUnion{
					OfString: param.NewOpt(text),
				},
				ToolCallID: ctco.CallID,
			},
		}
	}
	if item.OfInputMessage != nil {
		im := item.OfInputMessage
		return convertMessageItem(&responses.EasyInputMessageParam{
			Role: responses.EasyInputMessageRole(im.Role),
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: im.Content,
			},
		})
	}
	if item.OfOutputMessage != nil {
		om := item.OfOutputMessage
		var text string
		for _, c := range om.Content {
			if c.OfOutputText != nil {
				text += c.OfOutputText.Text
			}
		}
		content := openai.ChatCompletionAssistantMessageParamContentUnion{}
		if text != "" {
			content = openai.ChatCompletionAssistantMessageParamContentUnion{
				OfString: param.NewOpt(text),
			}
		}
		return &openai.ChatCompletionMessageParamUnion{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{
				Content: content,
			},
		}
	}
	switch {
	case item.OfFileSearchCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "file_search_call")
	case item.OfComputerCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "computer_call")
	case item.OfComputerCallOutput != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "computer_call_output")
	case item.OfWebSearchCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "web_search_call")
	case item.OfToolSearchCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "tool_search_call")
	case item.OfToolSearchOutput != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "tool_search_output")
	case item.OfReasoning != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "reasoning")
	case item.OfCompaction != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "compaction")
	case item.OfImageGenerationCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "image_generation_call")
	case item.OfCodeInterpreterCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "code_interpreter_call")
	case item.OfLocalShellCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "local_shell_call")
	case item.OfLocalShellCallOutput != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "local_shell_call_output")
	case item.OfShellCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "shell_call")
	case item.OfShellCallOutput != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "shell_call_output")
	case item.OfApplyPatchCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "apply_patch_call")
	case item.OfApplyPatchCallOutput != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "apply_patch_call_output")
	case item.OfMcpListTools != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "mcp_list_tools")
	case item.OfMcpApprovalRequest != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "mcp_approval_request")
	case item.OfMcpApprovalResponse != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "mcp_approval_response")
	case item.OfMcpCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "mcp_call")
	case item.OfCustomToolCall != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "custom_tool_call")
	case item.OfItemReference != nil:
		h.logWithSrc.Warn("unsupported input item variant", "type", "item_reference")
	default:
		h.logWithSrc.Warn("unsupported input item variant", "type", "unknown")
	}
	return nil
}

func convertMessageItem(msg *responses.EasyInputMessageParam) *openai.ChatCompletionMessageParamUnion {
	switch msg.Role {
	case "system":
		if msg.Content.OfString.Valid() {
			return &openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: param.NewOpt(msg.Content.OfString.Value),
					},
				},
			}
		}

		var texts []string
		for _, c := range msg.Content.OfInputItemContentList {
			if c.OfInputText != nil && c.OfInputText.Text != "" {
				texts = append(texts, c.OfInputText.Text)
			}
		}
		if len(texts) > 0 {
			parts := make([]openai.ChatCompletionContentPartTextParam, len(texts))
			for i, t := range texts {
				parts[i] = openai.ChatCompletionContentPartTextParam{Text: t}
			}
			return &openai.ChatCompletionMessageParamUnion{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfArrayOfContentParts: parts,
					},
				},
			}
		}
		return &openai.ChatCompletionMessageParamUnion{
			OfSystem: &openai.ChatCompletionSystemMessageParam{},
		}

	case "developer":
		if msg.Content.OfString.Valid() {
			return &openai.ChatCompletionMessageParamUnion{
				OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{
					Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
						OfString: param.NewOpt(msg.Content.OfString.Value),
					},
				},
			}
		}

		var texts []string
		for _, c := range msg.Content.OfInputItemContentList {
			if c.OfInputText != nil && c.OfInputText.Text != "" {
				texts = append(texts, c.OfInputText.Text)
			}
		}
		if len(texts) > 0 {
			parts := make([]openai.ChatCompletionContentPartTextParam, len(texts))
			for i, t := range texts {
				parts[i] = openai.ChatCompletionContentPartTextParam{Text: t}
			}
			return &openai.ChatCompletionMessageParamUnion{
				OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{
					Content: openai.ChatCompletionDeveloperMessageParamContentUnion{
						OfArrayOfContentParts: parts,
					},
				},
			}
		}
		return &openai.ChatCompletionMessageParamUnion{
			OfDeveloper: &openai.ChatCompletionDeveloperMessageParam{},
		}

	case "user":
		if msg.Content.OfString.Valid() {
			return &openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: param.NewOpt(msg.Content.OfString.Value),
					},
				},
			}
		}

		// Walk content list once to preserve item ordering.
		var parts []openai.ChatCompletionContentPartUnionParam
		hasImage := false
		for _, c := range msg.Content.OfInputItemContentList {
			if c.OfInputText != nil && c.OfInputText.Text != "" {
				parts = append(parts, openai.ChatCompletionContentPartUnionParam{
					OfText: &openai.ChatCompletionContentPartTextParam{Text: c.OfInputText.Text},
				})
			}
			if c.OfInputImage != nil {
				url := ""
				if c.OfInputImage.ImageURL.Valid() {
					url = c.OfInputImage.ImageURL.Value
				}
				if url == "" && c.OfInputImage.FileID.Valid() {
					url = c.OfInputImage.FileID.Value
				}
				if url != "" {
					hasImage = true
					parts = append(parts, openai.ChatCompletionContentPartUnionParam{
						OfImageURL: &openai.ChatCompletionContentPartImageParam{
							ImageURL: openai.ChatCompletionContentPartImageImageURLParam{
								URL:    url,
								Detail: string(c.OfInputImage.Detail),
							},
						},
					})
				}
			}
		}

		if !hasImage && len(parts) <= 1 {
			if len(parts) == 1 {
				return &openai.ChatCompletionMessageParamUnion{
					OfUser: &openai.ChatCompletionUserMessageParam{
						Content: openai.ChatCompletionUserMessageParamContentUnion{
							OfString: param.NewOpt(parts[0].OfText.Text),
						},
					},
				}
			}
			return &openai.ChatCompletionMessageParamUnion{
				OfUser: &openai.ChatCompletionUserMessageParam{},
			}
		}

		return &openai.ChatCompletionMessageParamUnion{
			OfUser: &openai.ChatCompletionUserMessageParam{
				Content: openai.ChatCompletionUserMessageParamContentUnion{
					OfArrayOfContentParts: parts,
				},
			},
		}

	case "assistant":
		if msg.Content.OfString.Valid() {
			return &openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(msg.Content.OfString.Value),
					},
				},
			}
		}

		var text string
		for _, c := range msg.Content.OfInputItemContentList {
			if c.OfInputText != nil && c.OfInputText.Text != "" {
				text += c.OfInputText.Text
			}
		}
		if text != "" {
			return &openai.ChatCompletionMessageParamUnion{
				OfAssistant: &openai.ChatCompletionAssistantMessageParam{
					Content: openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(text),
					},
				},
			}
		}
		return &openai.ChatCompletionMessageParamUnion{
			OfAssistant: &openai.ChatCompletionAssistantMessageParam{},
		}

	default:
		return nil
	}
}

func toChatTools(tools []responses.ToolUnionParam) []openai.ChatCompletionToolUnionParam {
	if len(tools) == 0 {
		return nil
	}
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, t := range tools {
		fn := t.OfFunction
		if fn == nil {
			continue
		}
		f := shared.FunctionDefinitionParam{
			Name:        fn.Name,
			Description: fn.Description,
			Parameters:  fn.Parameters,
		}
		if fn.Strict.Valid() && fn.Strict.Value {
			f.Strict = param.NewOpt(true)
		}
		result = append(result, openai.ChatCompletionFunctionTool(f))
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func outputToText(output responses.ResponseInputItemFunctionCallOutputOutputUnionParam) string {
	if output.OfString.Valid() {
		return output.OfString.Value
	}
	return ""
}

func customOutputToText(output responses.ResponseCustomToolCallOutputOutputUnionParam) string {
	if output.OfString.Valid() {
		return output.OfString.Value
	}
	return ""
}
