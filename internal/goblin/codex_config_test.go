package goblin

import (
	"os"
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

func initGoblinConfig(t *testing.T, address string) *GoblinConfig {
	t.Helper()
	goblinHome := t.TempDir()
	data := []byte("address: " + address + "\n")
	if err := os.WriteFile(goblinConfigPath(goblinHome), data, 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(goblinHomeEnv, goblinHome)
	cfg, err := loadGoblinConfig()
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

func TestApplyGoblinConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     map[string]any
		baseURL string
		want    bool
		check   func(t *testing.T, cc *codexConfig)
	}{
		{
			name: "already_correct",
			cfg: map[string]any{
				"model_provider": "goblin",
				"model_providers": map[string]any{
					"goblin": map[string]any{
						"base_url": "http://0.0.0.0:9999",
						"name":     "goblin",
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
					"goblin": map[string]any{
						"base_url": "http://0.0.0.0:9999",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cc *codexConfig) {
				if cc.modelProvider() != goblinProviderID {
					t.Errorf(`model_provider = %q, want "goblin"`, cc.modelProvider())
				}
				if n := cc.goblinProvider().name(); n != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, n)
				}
			},
		},
		{
			name: "wrong_model_provider",
			cfg: map[string]any{
				"model_provider": "foo",
				"model_providers": map[string]any{
					"goblin": map[string]any{
						"base_url": "http://0.0.0.0:9999",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cc *codexConfig) {
				if cc.modelProvider() != goblinProviderID {
					t.Errorf(`model_provider = %q, want "goblin"`, cc.modelProvider())
				}
				if n := cc.goblinProvider().name(); n != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, n)
				}
			},
		},
		{
			name: "missing_model_providers_map",
			cfg: map[string]any{
				"model_provider": "goblin",
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cc *codexConfig) {
				p := cc.goblinProvider()
				if p.baseURL() != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, p.baseURL())
				}
				if p.name() != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, p.name())
				}
			},
		},
		{
			name: "missing_goblin_entry",
			cfg: map[string]any{
				"model_provider": "goblin",
				"model_providers": map[string]any{
					"foo": map[string]any{
						"base_url": "http://localhost:4444",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cc *codexConfig) {
				p := cc.goblinProvider()
				if p.baseURL() != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, p.baseURL())
				}
				if p.name() != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, p.name())
				}
			},
		},
		{
			name: "wrong_base_url",
			cfg: map[string]any{
				"model_provider": "goblin",
				"model_providers": map[string]any{
					"goblin": map[string]any{
						"base_url": "http://localhost:8080",
					},
				},
			},
			baseURL: "http://0.0.0.0:9999",
			want:    true,
			check: func(t *testing.T, cc *codexConfig) {
				p := cc.goblinProvider()
				if p.baseURL() != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, p.baseURL())
				}
				if p.name() != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, p.name())
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
			check: func(t *testing.T, cc *codexConfig) {
				if cc.modelProvider() != goblinProviderID {
					t.Errorf(`model_provider = %q, want "goblin"`, cc.modelProvider())
				}
				p := cc.goblinProvider()
				if p.baseURL() != "http://0.0.0.0:9999" {
					t.Errorf(`base_url = %q, want "http://0.0.0.0:9999"`, p.baseURL())
				}
				if p.name() != goblinProviderID {
					t.Errorf(`provider name = %q, want "goblin"`, p.name())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cc := newCodexConfig(tt.cfg)
			got := cc.applyGoblinConfig(tt.baseURL)
			if got != tt.want {
				t.Errorf("applyGoblinConfig = %v, want %v", got, tt.want)
			}
			if tt.check != nil {
				tt.check(t, cc)
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
	cfg := initGoblinConfig(t, "0.0.0.0:9999")

	if err := runConfigCodex(cfg); err != nil {
		t.Fatal(err)
	}

	bakPath := codexConfigPath(codexHome) + ".bak"
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("expected .bak file to exist: %v", err)
	}

	assertCodexConfig(t, codexConfigPath(codexHome), map[string]any{
		"model_provider": "goblin",
	})

	data, err := os.ReadFile(codexConfigPath(codexHome))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	p := newCodexConfig(raw).goblinProvider()
	if p.name() != goblinProviderID {
		t.Errorf("provider name = %q, want %q", p.name(), goblinProviderID)
	}
}

func TestRunCodexConfig_UpdatesWhenWrongProvider(t *testing.T) {
	codexHome := t.TempDir()
	setupCodexConfig(t, codexHome, map[string]any{
		"model":          "bar",
		"model_provider": "foo",
		"model_providers": map[string]any{
			"goblin": map[string]any{
				"base_url": "http://localhost:8080",
			},
		},
	})
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initGoblinConfig(t, "127.0.0.1:8081")

	if err := runConfigCodex(cfg); err != nil {
		t.Fatal(err)
	}

	assertCodexConfig(t, codexConfigPath(codexHome), map[string]any{
		"model_provider": "goblin",
		"model":          "bar",
	})

	data, err := os.ReadFile(codexConfigPath(codexHome))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	p := newCodexConfig(raw).goblinProvider()
	if p.baseURL() != "http://127.0.0.1:8081" {
		t.Errorf("base_url = %v, want http://127.0.0.1:8081", p.baseURL())
	}
	if p.name() != goblinProviderID {
		t.Errorf("provider name = %q, want %q", p.name(), goblinProviderID)
	}
}

func TestRunCodexConfig_SkipsWriteWhenAlreadyCorrect(t *testing.T) {
	codexHome := t.TempDir()
	initial := map[string]any{
		"model":          "bar",
		"model_provider": "goblin",
		"model_providers": map[string]any{
			"goblin": map[string]any{
				"base_url": "http://0.0.0.0:9999",
				"name":     "goblin",
			},
		},
	}
	cfgPath := setupCodexConfig(t, codexHome, initial)
	t.Setenv(codexHomeEnv, codexHome)
	cfg := initGoblinConfig(t, "0.0.0.0:9999")

	origStat, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	origMod := origStat.ModTime()

	if err := runConfigCodex(cfg); err != nil {
		t.Fatal(err)
	}

	bakPath := codexConfigPath(codexHome) + ".bak"
	if _, err := os.Stat(bakPath); err == nil {
		t.Error("unexpected .bak file: config was not modified")
	}

	newStat, err := os.Stat(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
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
	cfg := initGoblinConfig(t, "0.0.0.0:9999")

	bakPath := codexConfigPath(codexHome) + ".bak"
	if err := os.WriteFile(bakPath, []byte("stale backup"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runConfigCodex(cfg); err != nil {
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

	data, err := os.ReadFile(codexConfigPath(codexHome))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	p := newCodexConfig(raw).goblinProvider()
	if p.name() != goblinProviderID {
		t.Errorf("provider name = %q, want %q", p.name(), goblinProviderID)
	}
	if p.baseURL() != "http://0.0.0.0:9999" {
		t.Errorf("base_url = %v, want http://0.0.0.0:9999", p.baseURL())
	}
}
