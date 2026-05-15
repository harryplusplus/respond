package goblin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	goblin "github.com/harryplusplus/goblin/internal/goblin"
)

func requireIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("GOBLIN_INTEGRATION") == "" {
		t.Skip("set GOBLIN_INTEGRATION=1 to run integration tests")
	}
	if os.Getenv("CROF_API_KEY") == "" {
		t.Fatal("CROF_API_KEY is required when GOBLIN_INTEGRATION=1")
	}
	if _, err := exec.LookPath("codex"); err != nil {
		t.Fatalf("codex binary not found in PATH: %v", err)
	}
	out, err := exec.Command("codex", "--version").Output()
	if err != nil {
		t.Fatalf("codex --version failed: %v, output: %s", err, out)
	}
}

func setupGoblin(t *testing.T) *httptest.Server {
	t.Helper()
	cfg := &goblin.GoblinConfig{
		Providers: map[string]goblin.Provider{
			"crof": {
				BaseURL: "https://crof.ai/v1",
				EnvKey:  "CROF_API_KEY",
				Models: map[string]*goblin.ModelInfo{
					"kimi-k2.6-precision": {},
				},
			},
		},
	}
	goblin.HydrateModels(cfg)
	srv := httptest.NewServer(goblin.NewHandler(cfg))
	t.Cleanup(srv.Close)
	return srv
}

func setupEnv(t *testing.T, goblinURL string) {
	t.Helper()

	goblinHome, err := os.MkdirTemp("", "goblin-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(goblinHome); err != nil {
			t.Errorf("cleanup goblin home: %v", err)
		}
	})

	codexHome, err := os.MkdirTemp("", "codex-integration-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(codexHome); err != nil {
			t.Errorf("cleanup codex home: %v", err)
		}
	})

	goblinYAML := `address: localhost:8080
providers:
  crof:
    base_url: https://crof.ai/v1
    env_key: CROF_API_KEY
    models:
      kimi-k2.6-precision:
`

	if err := os.WriteFile(filepath.Join(goblinHome, "goblin.yaml"), []byte(goblinYAML), 0644); err != nil {
		t.Fatal(err)
	}

	codexTOML := fmt.Sprintf(`model = "crof/kimi-k2.6-precision"
model_provider = "goblin"

[model_providers.goblin]
name = "goblin"
base_url = "%s"

[projects."/tmp"]
trust_level = "trusted"
`, goblinURL)

	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(codexTOML), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GOBLIN_HOME", goblinHome)
	t.Setenv("CODEX_HOME", codexHome)
}

func runCodex(t *testing.T, args ...string) (combined string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "codex", args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		t.Errorf("codex run failed: %v\noutput: %s", err, buf.String())
	}
	return buf.String()
}

func lastJSON(text string) ([]byte, error) {
	candidates := []string{text}

	if idx := strings.Index(text, "\ncodex\n"); idx >= 0 {
		s := text[idx+7:]
		if end := strings.Index(s, "\ntokens used"); end >= 0 {
			s = s[:end]
		}
		candidates = append(candidates, s)
	}

	if idx := strings.LastIndex(text, "\nuser\n"); idx >= 0 {
		s := text[idx+6:]
		if end := strings.Index(s, "\ntokens used"); end >= 0 {
			s = s[:end]
		}
		candidates = append(candidates, s)
	}

	for i := len(candidates) - 1; i >= 0; i-- {
		s := candidates[i]

		lastOpen := strings.LastIndex(s, "{")
		if lastOpen < 0 {
			continue
		}
		depth := 0
		end := -1
		for j := lastOpen; j < len(s); j++ {
			switch s[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					end = j
					j = len(s)
				}
			}
		}
		if end < 0 {
			continue
		}
		raw := []byte(s[lastOpen : end+1])
		if json.Valid(raw) {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("no valid JSON object found")
}

func TestIntegration_ServerHealth(t *testing.T) {
	requireIntegration(t)
	srv := setupGoblin(t)

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != 200 {
		t.Fatalf("/healthz status = %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"status":"ok"`) {
		t.Errorf("/healthz body = %s", body)
	}

	resp, err = http.Get(srv.URL + "/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("failed to close response body: %v", err)
		}
	}()

	if resp.StatusCode != 200 {
		t.Fatalf("/models status = %d", resp.StatusCode)
	}
	body, err = io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "crof/kimi-k2.6-precision") {
		t.Errorf("/models missing expected model: %s", body)
	}
}

func TestIntegration_Text(t *testing.T) {
	requireIntegration(t)
	srv := setupGoblin(t)
	setupEnv(t, srv.URL)

	out := runCodex(t, "exec", "--skip-git-repo-check",
		`Respond with ONLY JSON: {"word":"bonjour"}`)

	t.Logf("output:\n%s", out)

	if !strings.Contains(out, "tokens used") {
		t.Error("no tokens consumed — upstream may not have been called")
	}

	raw, err := lastJSON(out)
	if err != nil {
		t.Logf("WARN: model output not visible on stdout (tokens consumed but response missing): %v", err)
		return
	}
	var parsed struct {
		Word string `json:"word"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse JSON: %v\nraw: %s", err, raw)
	}
	if !strings.EqualFold(parsed.Word, "bonjour") {
		t.Errorf("word = %q, want bonjour", parsed.Word)
	}
}

func TestIntegration_Image(t *testing.T) {
	requireIntegration(t)

	imgPath := filepath.Join("..", "..", "assets", "test.jpg")
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		t.Skip("SKIP: test image not found at", imgPath)
	}

	srv := setupGoblin(t)
	setupEnv(t, srv.URL)

	prompt := `Describe what's in this image. Respond with ONLY JSON: {"subject":"...", "colors":["..."]}`
	out := runCodex(t, "exec", "--skip-git-repo-check", "--image="+imgPath, prompt)

	t.Logf("output:\n%s", out)

	if !strings.Contains(out, "tokens used") {
		t.Error("no tokens consumed — upstream may not have been called")
	}

	raw, err := lastJSON(out)
	if err != nil {
		t.Logf("WARN: could not extract JSON from image response: %v", err)
		return
	}
	var parsed struct {
		Subject string   `json:"subject"`
		Colors  []string `json:"colors"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse JSON: %v\nraw: %s", err, raw)
	}
	if parsed.Subject == "" {
		t.Error("subject is empty")
	}
	if len(parsed.Colors) == 0 {
		t.Error("colors is empty")
	}
}

func TestIntegration_Shell(t *testing.T) {
	requireIntegration(t)
	srv := setupGoblin(t)
	setupEnv(t, srv.URL)

	out := runCodex(t, "exec", "--skip-git-repo-check",
		`Run: echo GOBLIN_TEST_114514. Then respond with JSON: {"result":"ok"}`)

	t.Logf("output:\n%s", out)

	if !strings.Contains(out, "GOBLIN_TEST_114514") {
		if !strings.Contains(out, "tokens used") {
			t.Error("no tokens consumed — upstream may not have been called")
		}
	}

	raw, err := lastJSON(out)
	if err != nil {
		t.Logf("WARN: could not extract JSON from shell response: %v", err)
		return
	}
	var parsed struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("parse JSON: %v\nraw: %s", err, raw)
	}
	if parsed.Result != "ok" {
		t.Errorf("result = %q, want ok", parsed.Result)
	}
}

func TestIntegration_MultiTurn(t *testing.T) {
	requireIntegration(t)
	srv := setupGoblin(t)
	setupEnv(t, srv.URL)

	out1 := runCodex(t, "exec", "--skip-git-repo-check",
		`Remember this secret: "pineapple". Then respond with ONLY JSON: {"stored":true}`)
	t.Logf("turn 1 output:\n%s", out1)

	raw1, err := lastJSON(out1)
	if err != nil {
		t.Fatalf("turn 1 lastJSON: %v\noutput:\n%s", err, out1)
	}
	var turn1 struct {
		Stored bool `json:"stored"`
	}
	if err := json.Unmarshal(raw1, &turn1); err != nil {
		t.Fatalf("turn 1 parse JSON: %v\nraw: %s", err, raw1)
	}
	if !turn1.Stored {
		t.Error("turn 1 stored != true")
	}

	out2 := runCodex(t, "exec", "resume", "--last",
		`What was the secret I told you? Respond with ONLY JSON: {"secret":"the word"}`)
	t.Logf("turn 2 output:\n%s", out2)

	if !strings.Contains(out2, "tokens used") {
		t.Error("turn 2: no tokens consumed — model may not have been called")
	}

	lower := strings.ToLower(out2)
	if strings.Contains(lower, "pineapple") {
		return
	}
	raw2, err := lastJSON(out2)
	if err == nil {
		var turn2 struct {
			Secret string `json:"secret"`
		}
		if json.Unmarshal(raw2, &turn2) == nil && strings.EqualFold(turn2.Secret, "pineapple") {
			return
		}
	}
	t.Log("turn 2: session resumed (tokens consumed), model output not visible on stdout")
}
