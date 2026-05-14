package respond

import (
	"os"
	"strconv"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func setupCodexConfig(t *testing.T, codexHome string, cfg map[string]any) string {
	t.Helper()
	data, err := toml.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := codexConfigPath(codexHome)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func initRespondConfig(t *testing.T, host string, port int) *Config {
	t.Helper()
	respondHome := t.TempDir()
	data := []byte("host: " + host + "\nport: " + strconv.Itoa(port) + "\n")
	if err := os.WriteFile(respondConfigPath(respondHome), data, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(respondHomeEnv, respondHome)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func assertCodexConfig(t *testing.T, path string, wantMap map[string]any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := toml.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal result: %v\n%s", err, data)
	}

	for k, want := range wantMap {
		got, ok := result[k]
		if !ok {
			t.Errorf("key %q not found in result", k)
			continue
		}
		switch w := want.(type) {
		case string:
			if got != w {
				t.Errorf("result[%q] = %v, want %v", k, got, w)
			}
		case map[string]any:
			gotMap, ok := got.(map[string]any)
			if !ok {
				t.Errorf("result[%q] is not a map, got %T", k, got)
				continue
			}
			for sk, sv := range w {
				if gotMap[sk] != sv {
					t.Errorf("result[%q][%q] = %v, want %v", k, sk, gotMap[sk], sv)
				}
			}
		}
	}
}

func TestApplyRespondConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]any
		baseURL string
		want    bool
		check   func(t *testing.T, cfg map[string]any)
	}{
		{
			name: "already_correct",
			cfg: map[string]any{
				"model_provider": "respond",
				"model_providers": map[string]any{
					"respond": map[string]any{
						"base_url": "http://0.0.0.0:9999",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    false,
		},
		{
			name: "missing_model_provider",
			cfg: map[string]any{
				"model_providers": map[string]any{
					"respond": map[string]any{
						"base_url": "http://0.0.0.0:9999",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexModelProvider(cfg) != respondProviderID {
					t.Errorf(`model_provider = %q, want "respond"`, codexModelProvider(cfg))
				}
			},
		},
		{
			name: "wrong_model_provider",
			cfg: map[string]any{
				"model_provider": "foo",
				"model_providers": map[string]any{
					"respond": map[string]any{
						"base_url": "http://0.0.0.0:9999",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexModelProvider(cfg) != respondProviderID {
					t.Errorf(`model_provider = %q, want "respond"`, codexModelProvider(cfg))
				}
			},
		},
		{
			name: "missing_model_providers_map",
			cfg: map[string]any{
				"model_provider": "respond",
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexProviderBaseURL(codexProvider(cfg, respondProviderID)) != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, codexProviderBaseURL(codexProvider(cfg, respondProviderID)))
				}
			},
		},
		{
			name: "missing_respond_entry",
			cfg: map[string]any{
				"model_provider": "respond",
				"model_providers": map[string]any{
					"foo": map[string]any{
						"base_url": "http://localhost:4444",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexProviderBaseURL(codexProvider(cfg, respondProviderID)) != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, codexProviderBaseURL(codexProvider(cfg, respondProviderID)))
				}
			},
		},
		{
			name: "wrong_base_url",
			cfg: map[string]any{
				"model_provider": "respond",
				"model_providers": map[string]any{
					"respond": map[string]any{
						"base_url": "http://localhost:8080",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexProviderBaseURL(codexProvider(cfg, respondProviderID)) != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, codexProviderBaseURL(codexProvider(cfg, respondProviderID)))
				}
			},
		},
		{
			name: "multiple_changes",
			cfg: map[string]any{
				"model": "bar",
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cfg map[string]any) {
				if codexModelProvider(cfg) != respondProviderID {
					t.Errorf(`model_provider = %q, want "respond"`, codexModelProvider(cfg))
				}
				if codexProviderBaseURL(codexProvider(cfg, respondProviderID)) != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, codexProviderBaseURL(codexProvider(cfg, respondProviderID)))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyRespondConfig(tt.cfg, tt.baseURL)
			if got != tt.want {
				t.Errorf("applyRespondConfig = %v, want %v", got, tt.want)
			}
			if tt.check != nil {
				tt.check(t, tt.cfg)
			}
		})
	}
}

func TestRunCodexConfig_UpdatesWhenMissingProvider(t *testing.T) {
	codexHome := t.TempDir()
	setupCodexConfig(t, codexHome, map[string]any{
		"model": "bar",
		"model_providers": map[string]any{
			"foo": map[string]any{
				"base_url": "http://localhost:4444",
			},
		},
	})
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initRespondConfig(t, "0.0.0.0", 9999)

	if err := RunCodexConfig(cfg); err != nil {
		t.Fatal(err)
	}

	bakPath := codexConfigPath(codexHome) + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("expected .bak file to exist: %v", err)
	}

	assertCodexConfig(t, codexConfigPath(codexHome), map[string]any{
		"model_provider": "respond",
	})
}

func TestRunCodexConfig_UpdatesWhenWrongProvider(t *testing.T) {
	codexHome := t.TempDir()
	setupCodexConfig(t, codexHome, map[string]any{
		"model":          "bar",
		"model_provider": "foo",
		"model_providers": map[string]any{
			"respond": map[string]any{
				"base_url": "http://localhost:8080",
			},
		},
	})
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initRespondConfig(t, "127.0.0.1", 8081)

	if err := RunCodexConfig(cfg); err != nil {
		t.Fatal(err)
	}

	assertCodexConfig(t, codexConfigPath(codexHome), map[string]any{
		"model_provider": "respond",
		"model":          "bar",
	})

	data, _ := os.ReadFile(codexConfigPath(codexHome))
	var result map[string]any
	toml.Unmarshal(data, &result)
	if codexProviderBaseURL(codexProvider(result, respondProviderID)) != "http://127.0.0.1:8081" {
		t.Errorf("base_url = %v, want http://127.0.0.1:8081", codexProviderBaseURL(codexProvider(result, respondProviderID)))
	}
}

func TestRunCodexConfig_SkipsWriteWhenAlreadyCorrect(t *testing.T) {
	codexHome := t.TempDir()
	initial := map[string]any{
		"model":          "bar",
		"model_provider": "respond",
		"model_providers": map[string]any{
			"respond": map[string]any{
				"base_url": "http://0.0.0.0:9999",
			},
		},
	}
	cfgPath := setupCodexConfig(t, codexHome, initial)
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initRespondConfig(t, "0.0.0.0", 9999)

	origStat, _ := os.Stat(cfgPath)
	origMod := origStat.ModTime()

	if err := RunCodexConfig(cfg); err != nil {
		t.Fatal(err)
	}

	bakPath := codexConfigPath(codexHome) + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		t.Error("unexpected .bak file: config was not modified")
	}

	newStat, _ := os.Stat(cfgPath)
	if !newStat.ModTime().Equal(origMod) {
		t.Error("config.toml was rewritten even though no change was needed")
	}
}

func TestRunCodexConfig_CleansUpOldBak(t *testing.T) {
	codexHome := t.TempDir()
	setupCodexConfig(t, codexHome, map[string]any{
		"model": "bar",
		"model_providers": map[string]any{
			"foo": map[string]any{
				"base_url": "http://localhost:4444",
			},
		},
	})
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initRespondConfig(t, "0.0.0.0", 9999)

	bakPath := codexConfigPath(codexHome) + ".bak"
	os.WriteFile(bakPath, []byte("stale backup"), 0644)

	if err := RunCodexConfig(cfg); err != nil {
		t.Fatal(err)
	}

	bakData, err := os.ReadFile(bakPath)
	if err != nil {
		t.Fatal("expected new .bak to exist:", err)
	}
	var bakCfg map[string]any
	if err := toml.Unmarshal(bakData, &bakCfg); err != nil {
		t.Fatalf("unmarshal bak: %v\n%s", err, bakData)
	}
	if bakCfg["model"] != "bar" {
		t.Error("bak should contain original config, not stale data")
	}
}
