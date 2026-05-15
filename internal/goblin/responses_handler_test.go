package goblin

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func isAlphaNum(c rune) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func fakeUpstream(t *testing.T, chunks []string, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("upstream: failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !req.Stream {
			t.Error("upstream: expected stream=true")
		}
		if req.StreamOptions == nil || !req.StreamOptions.IncludeUsage {
			t.Error("upstream: expected stream_options.include_usage=true")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(statusCode)

		for _, chunk := range chunks {
			if _, err := fmt.Fprintf(w, "data: %s\n\n", chunk); err != nil {
				t.Error(err)
			}
		}
		if _, err := fmt.Fprintf(w, "data: [DONE]\n\n"); err != nil {
			t.Error(err)
		}
	}))
}

func fakeUpstreamError(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(statusCode)
		if _, err := io.WriteString(w, body); err != nil {
			t.Error(err)
		}
	}))
}

func readSSEEvents(t *testing.T, body io.Reader) []map[string]any {
	t.Helper()
	var events []map[string]any
	scanner := bufio.NewScanner(body)
	var currentEvent string
	var currentData bytes.Buffer

	flush := func() {
		if currentEvent != "" && currentData.Len() > 0 {
			var data any
			if err := json.Unmarshal(currentData.Bytes(), &data); err == nil {
				events = append(events, map[string]any{
					"event": currentEvent,
					"data":  data,
				})
			}
		}
		currentEvent = ""
		currentData.Reset()
	}

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			currentEvent = strings.TrimPrefix(line, "event: ")
		} else if strings.HasPrefix(line, "data: ") {
			currentData.WriteString(strings.TrimPrefix(line, "data: "))
		}
	}
	flush()

	return events
}

func TestHandleResponses_SimpleTextStream(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	}
	upstream := fakeUpstream(t, chunks, 0)
	defer upstream.Close()

	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				BaseURL: upstream.URL,
			},
		},
	}

	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model: "test/some-model",
		Input: []InputItem{
			{Type: "message", Role: "user", Content: []ContentItem{
				{Type: "input_text", Text: "hi"},
			}},
		},
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want %q", ct, "text/event-stream")
	}

	events := readSSEEvents(t, resp.Body)

	if len(events) < 3 {
		t.Fatalf("got %d events, want at least 3", len(events))
	}

	if events[0]["event"] != "response.created" {
		t.Errorf("events[0].event = %v, want response.created", events[0]["event"])
	}

	var foundCreated, foundCompleted bool
	var deltaCount int
	for _, ev := range events {
		switch ev["event"] {
		case "response.created":
			foundCreated = true
		case "response.output_text.delta":
			deltaCount++
		case "response.completed":
			foundCompleted = true
			data, ok := ev["data"].(map[string]any)
			if !ok {
				t.Errorf("completed data is not a map")
			} else if resp, ok := data["response"].(map[string]any); ok {
				if usage, ok := resp["usage"].(map[string]any); ok {
					if usage["total_tokens"] != float64(15) {
						t.Errorf("total_tokens = %v, want 15", usage["total_tokens"])
					}
				} else {
					t.Error("completed missing usage")
				}
			}
		}
	}

	if !foundCreated {
		t.Error("missing response.created event")
	}
	if deltaCount != 2 {
		t.Errorf("output_text.delta count = %d, want 2", deltaCount)
	}
	if !foundCompleted {
		t.Error("missing response.completed event")
	}
}

func TestHandleResponses_WithToolCall(t *testing.T) {
	chunks := []string{
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":null},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":""}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"city\":\"seoul\"}"}}]},"finish_reason":null}]}`,
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":20,"completion_tokens":10,"total_tokens":30}}`,
	}
	upstream := fakeUpstream(t, chunks, 0)
	defer upstream.Close()

	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				BaseURL: upstream.URL,
			},
		},
	}

	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model: "test/some-model",
		Input: []InputItem{
			{Type: "message", Role: "user", Content: []ContentItem{
				{Type: "input_text", Text: "whats the weather"},
			}},
		},
		Tools: []Tool{
			{Type: "function", Function: &ToolFunction{Name: "get_weather"}},
		},
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	events := readSSEEvents(t, resp.Body)

	var foundFunctionCallDone bool
	var foundCompleted bool
	for _, ev := range events {
		switch ev["event"] {
		case "response.output_item.done":
			data, ok := ev["data"].(map[string]any)
			if !ok {
				continue
			}
			item, ok := data["item"].(map[string]any)
			if !ok {
				continue
			}
			if item["type"] == "function_call" {
				foundFunctionCallDone = true
				if item["name"] != "get_weather" {
					t.Errorf("function_call name = %v, want get_weather", item["name"])
				}
			}
		case "response.completed":
			foundCompleted = true
		}
	}

	if !foundFunctionCallDone {
		t.Error("missing function_call output_item.done event")
	}
	if !foundCompleted {
		t.Error("missing response.completed event")
	}
}

func TestHandleResponses_StreamMustBeTrue(t *testing.T) {
	cfg := &GoblinConfig{}
	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model:  "test/some-model",
		Input:  []InputItem{},
		Stream: false,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestHandleResponses_UpstreamError(t *testing.T) {
	upstream := fakeUpstreamError(t, http.StatusBadRequest, `{"error":"bad request"}`)
	defer upstream.Close()

	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				BaseURL: upstream.URL,
			},
		},
	}

	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model: "test/some-model",
		Input: []InputItem{
			{Type: "message", Role: "user", Content: []ContentItem{
				{Type: "input_text", Text: "hi"},
			}},
		},
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (SSE even on error)", resp.StatusCode)
	}

	events := readSSEEvents(t, resp.Body)
	if len(events) == 0 {
		t.Fatal("expected at least one SSE event")
	}

	var foundFailed bool
	for _, ev := range events {
		if ev["event"] == "response.failed" {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Error("missing response.failed event")
	}
}

func TestHandleResponses_WithInstructions(t *testing.T) {
	upstream := fakeUpstream(t, []string{
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7}}`,
	}, 0)
	defer upstream.Close()

	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				BaseURL: upstream.URL,
			},
		},
	}

	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model:        "test/some-model",
		Instructions: "you are a helpful assistant",
		Input: []InputItem{
			{Type: "message", Role: "user", Content: []ContentItem{
				{Type: "input_text", Text: "hi"},
			}},
		},
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	events := readSSEEvents(t, resp.Body)
	if len(events) < 2 {
		t.Fatalf("got %d events, want at least 2", len(events))
	}

	var foundCompleted bool
	for _, ev := range events {
		if ev["event"] == "response.completed" {
			foundCompleted = true
			break
		}
	}
	if !foundCompleted {
		t.Error("missing response.completed event")
	}
}

func TestHandleResponses_EmptyInput(t *testing.T) {
	upstream := fakeUpstream(t, []string{
		`{"id":"chatcmpl-abc","object":"chat.completion.chunk","created":1234567890,"model":"test-model","choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
	}, 0)
	defer upstream.Close()

	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				BaseURL: upstream.URL,
			},
		},
	}

	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	reqBody := ResponsesRequest{
		Model:  "test/some-model",
		Input:  []InputItem{},
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestHandleResponses_InvalidJSON(t *testing.T) {
	cfg := &GoblinConfig{}
	handler := newResponsesHandler(cfg).handlePostResponses()
	srv := httptest.NewServer(http.HandlerFunc(handler))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`not json`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestGenerateResponseID(t *testing.T) {
	id, err := generateResponseID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "resp_") {
		t.Errorf("id %q does not start with resp_", id)
	}
	if len(id) != 5+50 {
		t.Errorf("id %q has length %d, want %d", id, len(id), 5+50)
	}
}

func TestGenerateMessageID(t *testing.T) {
	id, err := generateMessageID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "msg_") {
		t.Errorf("id %q does not start with msg_", id)
	}
	if len(id) != 4+50 {
		t.Errorf("id %q has length %d, want %d", id, len(id), 4+50)
	}
}

func TestGenerateFunctionCallID(t *testing.T) {
	id, err := generateFunctionCallItemID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "fc_") {
		t.Errorf("id %q does not start with fc_", id)
	}
	if len(id) != 3+50 {
		t.Errorf("id %q has length %d, want %d", id, len(id), 3+50)
	}
}

func TestGenerateCallID(t *testing.T) {
	id, err := generateCallID()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(id, "call_") {
		t.Errorf("id %q does not start with call_", id)
	}
	if len(id) != 5+24 {
		t.Errorf("id %q has length %d, want %d", id, len(id), 5+24)
	}
	for _, c := range id[5:] {
		if !isAlphaNum(c) {
			t.Errorf("call ID contains non-alphanumeric char %q", c)
		}
	}
}

func TestGenerateCallID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for range 1000 {
		id, err := generateCallID()
		if err != nil {
			t.Fatal(err)
		}
		if seen[id] {
			t.Errorf("duplicate call ID %q", id)
		}
		seen[id] = true
	}
}

func TestWriteSSE(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSSE(&buf, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id": "resp_abc",
		},
	}); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2\n%s", len(lines), output)
	}
	if !strings.HasPrefix(lines[0], "event: response.created") {
		t.Errorf("line 0 = %q, want event prefix", lines[0])
	}
	if !strings.HasPrefix(lines[1], "data: ") {
		t.Errorf("line 1 = %q, want data prefix", lines[1])
	}
	if !strings.HasSuffix(output, "\n\n") {
		t.Errorf("output %q does not end with double newline", output)
	}
}

func TestWriteSSE_EmptyData(t *testing.T) {
	var buf bytes.Buffer
	if err := writeSSE(&buf, "response.incomplete", nil); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "data: null") {
		t.Errorf("output %q should contain data: null", output)
	}
}

func TestEmitCreated(t *testing.T) {
	var buf bytes.Buffer
	if err := emitCreated(&buf, "resp_abc123"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.created") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "resp_abc123") {
		t.Errorf("missing response ID: %s", output)
	}
}

func TestEmitCompleted(t *testing.T) {
	var buf bytes.Buffer
	usage := &ResponseUsage{
		InputTokens:           10,
		OutputTokens:          20,
		TotalTokens:           30,
		CachedInputTokens:     5,
		ReasoningOutputTokens: 3,
	}
	if err := emitCompleted(&buf, "resp_abc123", usage); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.completed") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "input_tokens") {
		t.Errorf("missing input_tokens: %s", output)
	}
	if !strings.Contains(output, "cached_input_tokens") {
		t.Errorf("missing cached_input_tokens: %s", output)
	}
	if !strings.Contains(output, "reasoning_output_tokens") {
		t.Errorf("missing reasoning_output_tokens: %s", output)
	}
}

func TestEmitCompleted_NilUsage(t *testing.T) {
	var buf bytes.Buffer
	if err := emitCompleted(&buf, "resp_abc123", nil); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.completed") {
		t.Errorf("missing event name: %s", output)
	}
	if strings.Contains(output, "usage") {
		t.Errorf("should not contain usage when nil: %s", output)
	}
}

func TestEmitOutputTextDelta(t *testing.T) {
	var buf bytes.Buffer
	if err := emitOutputTextDelta(&buf, "Hello, "); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.output_text.delta") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "Hello, ") {
		t.Errorf("missing delta text: %s", output)
	}
}

func TestEmitOutputItemAdded(t *testing.T) {
	var buf bytes.Buffer
	if err := emitOutputItemAdded(&buf, OutputItem{
		Type: "message",
		Role: "assistant",
		Content: []ContentItem{
			{Type: "output_text", Text: ""},
		},
	}, "msg_abc"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.output_item.added") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "msg_abc") {
		t.Errorf("missing item id: %s", output)
	}
}

func TestEmitOutputItemDone(t *testing.T) {
	var buf bytes.Buffer
	if err := emitOutputItemDone(&buf, OutputItem{
		Type: "message",
		Role: "assistant",
		Content: []ContentItem{
			{Type: "output_text", Text: "hello"},
		},
	}, "msg_abc"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.output_item.done") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "hello") {
		t.Errorf("missing content: %s", output)
	}
}

func TestEmitFailed(t *testing.T) {
	var buf bytes.Buffer
	if err := emitFailed(&buf, "upstream_error", "something went wrong"); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "event: response.failed") {
		t.Errorf("missing event name: %s", output)
	}
	if !strings.Contains(output, "upstream_error") {
		t.Errorf("missing error code: %s", output)
	}
	if !strings.Contains(output, "something went wrong") {
		t.Errorf("missing error message: %s", output)
	}
}

func TestWriteSSE_ToHTTPResponse(t *testing.T) {
	w := httptest.NewRecorder()
	if err := writeSSE(w, "response.created", map[string]any{"type": "response.created"}); err != nil {
		t.Fatal(err)
	}

	body := w.Body.String()
	if !strings.Contains(body, "event: response.created") {
		t.Errorf("body missing event: %s", body)
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestToChatMessages_EmptyInput(t *testing.T) {
	got := toChatMessages(nil)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestToChatMessages_UserMessage(t *testing.T) {
	input := []InputItem{
		{Type: "message", Role: "user", Content: []ContentItem{
			{Type: "input_text", Text: "hello"},
		}},
	}

	got := toChatMessages(input)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("role = %q, want user", got[0].Role)
	}
	if *got[0].Content.(*string) != "hello" {
		t.Errorf("content = %v, want hello", *got[0].Content.(*string))
	}
}

func TestToChatMessages_AssistantMessage(t *testing.T) {
	input := []InputItem{
		{Type: "message", Role: "assistant", Content: []ContentItem{
			{Type: "output_text", Text: "hi there"},
		}},
	}

	got := toChatMessages(input)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "assistant" {
		t.Errorf("role = %q, want assistant", got[0].Role)
	}
	if *got[0].Content.(*string) != "hi there" {
		t.Errorf("content = %v, want hi there", *got[0].Content.(*string))
	}
}

func TestToChatMessages_ToolCall(t *testing.T) {
	input := []InputItem{
		{
			Type:      "function_call",
			Name:      "get_weather",
			Arguments: `{"city":"seoul"}`,
			CallID:    "call_abc",
		},
	}

	got := toChatMessages(input)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "assistant" {
		t.Errorf("role = %q, want assistant", got[0].Role)
	}
	if len(got[0].ToolCalls) != 1 {
		t.Fatalf("got %d tool_calls, want 1", len(got[0].ToolCalls))
	}
	if got[0].ToolCalls[0].ID != "call_abc" {
		t.Errorf("tool_call id = %q, want call_abc", got[0].ToolCalls[0].ID)
	}
	if got[0].ToolCalls[0].Function.Name != "get_weather" {
		t.Errorf("function name = %q, want get_weather", got[0].ToolCalls[0].Function.Name)
	}
	if got[0].ToolCalls[0].Function.Arguments != `{"city":"seoul"}` {
		t.Errorf("arguments = %q", got[0].ToolCalls[0].Function.Arguments)
	}
}

func TestToChatMessages_FunctionCallOutput(t *testing.T) {
	input := []InputItem{
		{
			Type:   "function_call_output",
			CallID: "call_def456",
			Output: "result: 42",
		},
	}

	got := toChatMessages(input)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "tool" {
		t.Errorf("role = %q, want tool", got[0].Role)
	}
	if *got[0].ToolCallID != "call_def456" {
		t.Errorf("tool_call_id = %q, want call_def456", *got[0].ToolCallID)
	}
	if *got[0].Content.(*string) != "result: 42" {
		t.Errorf("content = %q, want result: 42", *got[0].Content.(*string))
	}
}

func TestToChatMessages_FunctionCallOutputWithContentItems(t *testing.T) {
	input := []InputItem{
		{
			Type:   "function_call_output",
			CallID: "call_def456",
			Output: []any{
				map[string]any{"type": "input_text", "text": "result is 42"},
			},
		},
	}

	got := toChatMessages(input)
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "tool" {
		t.Errorf("role = %q, want tool", got[0].Role)
	}
	if *got[0].Content.(*string) != "result is 42" {
		t.Errorf("content = %q, want result is 42", *got[0].Content.(*string))
	}
}

func TestToChatMessages_MultipleMessages(t *testing.T) {
	input := []InputItem{
		{Type: "message", Role: "user", Content: []ContentItem{{Type: "input_text", Text: "a"}}},
		{Type: "message", Role: "assistant", Content: []ContentItem{{Type: "output_text", Text: "b"}}},
		{Type: "message", Role: "user", Content: []ContentItem{{Type: "input_text", Text: "c"}}},
	}

	got := toChatMessages(input)
	if len(got) != 3 {
		t.Fatalf("got %d messages, want 3", len(got))
	}
	if got[0].Role != "user" || *got[0].Content.(*string) != "a" {
		t.Errorf("msg[0] = %v", got[0])
	}
	if got[1].Role != "assistant" || *got[1].Content.(*string) != "b" {
		t.Errorf("msg[1] = %v", got[1])
	}
	if got[2].Role != "user" || *got[2].Content.(*string) != "c" {
		t.Errorf("msg[2] = %v", got[2])
	}
}

func TestPrependInstructions(t *testing.T) {
	messages := []ChatMessage{
		{Role: "user", Content: "hello"},
	}
	got := prependInstructions(messages, "be helpful")
	if len(got) != 2 {
		t.Fatalf("got %d messages, want 2", len(got))
	}
	if got[0].Role != "system" {
		t.Errorf("role = %q, want system", got[0].Role)
	}
	if *got[0].Content.(*string) != "be helpful" {
		t.Errorf("content = %q, want be helpful", *got[0].Content.(*string))
	}
	if got[1].Role != "user" {
		t.Errorf("role = %q, want user", got[1].Role)
	}
}

func TestPrependInstructions_Empty(t *testing.T) {
	messages := []ChatMessage{
		{Role: "user", Content: "hello"},
	}
	got := prependInstructions(messages, "")
	if len(got) != 1 {
		t.Fatalf("got %d messages, want 1", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("role = %q, want user", got[0].Role)
	}
}

func TestConvertTools(t *testing.T) {
	tools := []Tool{
		{Type: "function", Function: &ToolFunction{Name: "get_weather", Description: "Get weather"}},
	}
	got := convertTools(tools)
	if len(got) != 1 {
		t.Fatalf("got %d tools, want 1", len(got))
	}
	if got[0].Type != "function" {
		t.Errorf("type = %q, want function", got[0].Type)
	}
	if got[0].Function.Name != "get_weather" {
		t.Errorf("name = %q, want get_weather", got[0].Function.Name)
	}
	if got[0].Function.Description != "Get weather" {
		t.Errorf("desc = %q, want Get weather", got[0].Function.Description)
	}
}

func TestConvertTools_Empty(t *testing.T) {
	got := convertTools(nil)
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestJoinInputText(t *testing.T) {
	items := []ContentItem{
		{Type: "input_text", Text: "hello"},
		{Type: "input_image", ImageURL: "data:..."},
		{Type: "input_text", Text: "world"},
	}
	got := joinInputText(items)
	if got != "hello\nworld" {
		t.Errorf("got %q, want hello\nworld", got)
	}
}

func TestJoinInputText_Empty(t *testing.T) {
	got := joinInputText(nil)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestJoinOutputText(t *testing.T) {
	items := []ContentItem{
		{Type: "output_text", Text: "hello"},
		{Type: "output_text", Text: "world"},
	}
	got := joinOutputText(items)
	if got != "helloworld" {
		t.Errorf("got %q, want helloworld", got)
	}
}

func TestFilterInputText(t *testing.T) {
	items := []ContentItem{
		{Type: "input_text", Text: "a"},
		{Type: "input_image", ImageURL: "data:img"},
		{Type: "output_text", Text: "b"},
		{Type: "input_text", Text: "c"},
	}
	got := filterInputText(items)
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Text != "a" || got[1].Text != "c" {
		t.Errorf("got %v", got)
	}
}

func TestFilterInputImages(t *testing.T) {
	items := []ContentItem{
		{Type: "input_text", Text: "a"},
		{Type: "input_image", ImageURL: "img1"},
		{Type: "output_text", Text: "b"},
		{Type: "input_image", ImageURL: "img2"},
	}
	got := filterInputImages(items)
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].ImageURL != "img1" || got[1].ImageURL != "img2" {
		t.Errorf("got %v", got)
	}
}

func TestOutputToText_String(t *testing.T) {
	item := &InputItem{Output: "plain text result"}
	got := outputToText(item)
	if got != "plain text result" {
		t.Errorf("got %q, want plain text result", got)
	}
}

func TestOutputToText_ContentItems(t *testing.T) {
	item := &InputItem{Output: []any{
		map[string]any{"type": "input_text", "text": "line1"},
		map[string]any{"type": "input_text", "text": "line2"},
	}}
	got := outputToText(item)
	if got != "line1\nline2" {
		t.Errorf("got %q, want line1\nline2", got)
	}
}

func TestOutputToText_Empty(t *testing.T) {
	item := &InputItem{Output: ""}
	got := outputToText(item)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestOutputToText_Nil(t *testing.T) {
	item := &InputItem{}
	got := outputToText(item)
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
