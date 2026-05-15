package goblin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestModelsHandler_Empty(t *testing.T) {
	srv := httptest.NewServer(newTestHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if len(body.Models) != 0 {
		t.Errorf("Models = %d, want 0", len(body.Models))
	}
}

func TestModelsHandler_WithModels(t *testing.T) {
	cfg := &Config{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"model-a": {Priority: new(10)},
					"model-b": {Priority: new(20)},
				},
			},
		},
	}
	HydrateModels(cfg)
	handler := NewHandler(cfg)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var body ModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if len(body.Models) != 2 {
		t.Fatalf("Models = %d, want 2", len(body.Models))
	}

	if body.Models[0].Slug != "test/model-a" {
		t.Errorf("Models[0].Slug = %q, want %q", body.Models[0].Slug, "test/model-a")
	}
	if body.Models[1].Slug != "test/model-b" {
		t.Errorf("Models[1].Slug = %q, want %q", body.Models[1].Slug, "test/model-b")
	}
}

func TestModelsHandler_MethodNotAllowed(t *testing.T) {
	handler := newTestHandler()
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/models", "text/plain", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusMethodNotAllowed)
	}
}
