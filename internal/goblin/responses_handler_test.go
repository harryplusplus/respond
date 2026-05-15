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

	"github.com/openai/openai-go/v3/responses"
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

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("upstream: failed to read body: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var raw map[string]any
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Errorf("upstream: failed to decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if raw["stream"] != true {
			t.Error("upstream: expected stream=true")
		}
		so, ok := raw["stream_options"].(map[string]any)
		if !ok || so["include_usage"] != true {
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

func mustMarshalReq(t *testing.T, input []responses.ResponseInputItemUnionParam, model string, stream bool) []byte {
	t.Helper()
	req := map[string]any{
		"model":  model,
		"input":  input,
		"stream": stream,
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func textContentItem(text string) responses.ResponseInputContentUnionParam {
	return responses.ResponseInputContentParamOfInputText(text)
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

	body := mustMarshalReq(t, []responses.ResponseInputItemUnionParam{
		{OfMessage: &responses.EasyInputMessageParam{
			Role: "user",
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					textContentItem("hi"),
				},
			},
		}},
	}, "test/some-model", true)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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
			} else if r, ok := data["response"].(map[string]any); ok {
				if usage, ok := r["usage"].(map[string]any); ok {
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

	body := mustMarshalReq(t, []responses.ResponseInputItemUnionParam{
		{OfMessage: &responses.EasyInputMessageParam{
			Role: "user",
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					textContentItem("whats the weather"),
				},
			},
		}},
	}, "test/some-model", true)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

	body := mustMarshalReq(t, nil, "test/some-model", false)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

	body := mustMarshalReq(t, []responses.ResponseInputItemUnionParam{
		{OfMessage: &responses.EasyInputMessageParam{
			Role: "user",
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: []responses.ResponseInputContentUnionParam{
					textContentItem("hi"),
				},
			},
		}},
	}, "test/some-model", true)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

	req := map[string]any{
		"model":        "test/some-model",
		"instructions": "you are a helpful assistant",
		"input": []any{
			map[string]any{
				"type": "message",
				"role": "user",
				"content": []any{
					map[string]any{"type": "input_text", "text": "hi"},
				},
			},
		},
		"stream": true,
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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

	body := mustMarshalReq(t, nil, "test/some-model", true)

	resp, err := http.Post(srv.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close() //nolint:errcheck

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
	defer resp.Body.Close() //nolint:errcheck

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

func TestSSEWriter_emit(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emit("response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id": "resp_abc",
		},
	}); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2\n%s", len(lines), body)
	}
	if !strings.HasPrefix(lines[0], "event: response.created") {
		t.Errorf("line 0 = %q, want event prefix", lines[0])
	}
	if !strings.HasPrefix(lines[1], "data: ") {
		t.Errorf("line 1 = %q, want data prefix", lines[1])
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Errorf("output %q does not end with double newline", body)
	}
}

func TestSSEWriter_emit_NilData(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emit("response.incomplete", nil); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data: null") {
		t.Errorf("output %q should contain data: null", body)
	}
}

func TestSSEWriter_EmitCreated(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitCreated("resp_abc123"); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.created") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "resp_abc123") {
		t.Errorf("missing response ID: %s", body)
	}
}

func TestSSEWriter_EmitCompleted(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	usage := &responses.ResponseUsage{
		InputTokens:  10,
		OutputTokens: 20,
		TotalTokens:  30,
	}
	usage.InputTokensDetails.CachedTokens = 5
	usage.OutputTokensDetails.ReasoningTokens = 3
	if err := sw.emitCompleted("resp_abc123", usage); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.completed") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "input_tokens") {
		t.Errorf("missing input_tokens: %s", body)
	}
	if !strings.Contains(body, "cached_input_tokens") {
		t.Errorf("missing cached_input_tokens: %s", body)
	}
	if !strings.Contains(body, "reasoning_output_tokens") {
		t.Errorf("missing reasoning_output_tokens: %s", body)
	}
}

func TestSSEWriter_EmitCompleted_NilUsage(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitCompleted("resp_abc123", nil); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.completed") {
		t.Errorf("missing event name: %s", body)
	}
	if strings.Contains(body, "usage") {
		t.Errorf("should not contain usage when nil: %s", body)
	}
}

func TestSSEWriter_EmitOutputTextDelta(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitOutputTextDelta("Hello, "); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_text.delta") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "Hello, ") {
		t.Errorf("missing delta text: %s", body)
	}
}

func TestSSEWriter_EmitOutputItemAdded(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitOutputItemAdded(map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []map[string]any{{"type": "output_text", "text": ""}},
	}, "msg_abc"); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_item.added") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "msg_abc") {
		t.Errorf("missing item id: %s", body)
	}
}

func TestSSEWriter_EmitOutputItemDone(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitOutputItemDone(map[string]any{
		"type":    "message",
		"role":    "assistant",
		"content": []map[string]any{{"type": "output_text", "text": "hello"}},
	}, "msg_abc"); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.output_item.done") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "hello") {
		t.Errorf("missing content: %s", body)
	}
}

func TestSSEWriter_EmitFailed(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := newSSEWriter(rec)
	if err := sw.emitFailed("resp_abc", "upstream_error", "something went wrong"); err != nil {
		t.Fatal(err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "event: response.failed") {
		t.Errorf("missing event name: %s", body)
	}
	if !strings.Contains(body, "upstream_error") {
		t.Errorf("missing error code: %s", body)
	}
	if !strings.Contains(body, "something went wrong") {
		t.Errorf("missing error message: %s", body)
	}
}
