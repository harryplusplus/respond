package goblin

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigYAMLTags(t *testing.T) {
	ty := reflect.TypeFor[GoblinConfig]()
	for _, err := range checkYAMLTags(ty, ty.Name()) {
		t.Error(err)
	}
}

func checkYAMLTags(ty reflect.Type, path string) []error {
	var errs []error

	switch ty.Kind() {
	case reflect.Struct:
		for f := range ty.Fields() {
			if !f.IsExported() {
				continue
			}

			fieldPath := fmt.Sprintf("%s.%s", path, f.Name)
			tag := f.Tag.Get("yaml")
			if tag == "" {
				errs = append(errs, fmt.Errorf("field %s: disallow: missing yaml tag", fieldPath))
				continue
			}

			if tag == "-" {
				continue
			}

			expected, err := toSnakeCase(f.Name)
			if err != nil {
				errs = append(errs, fmt.Errorf("field %s: disallow: %v", fieldPath, err))
				continue
			}

			if tag != expected {
				errs = append(errs, fmt.Errorf("field %s: disallow: yaml key %q does not match field name (expected %q)",
					fieldPath, tag, expected))
			}

			errs = append(errs, checkYAMLTags(f.Type, fieldPath)...)
		}
	case reflect.Map:
		if ty.Key().Kind() != reflect.String {
			errs = append(errs, fmt.Errorf("field %s: disallow: map key type %s (must be string)", path, ty.Key()))
			return errs
		}
		errs = append(errs, checkYAMLTags(ty.Elem(), path)...)
	case reflect.Slice, reflect.Pointer:
		errs = append(errs, checkYAMLTags(ty.Elem(), path)...)
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
		reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32,
		reflect.Float64, reflect.Interface:
		// noop
	default:
		errs = append(errs, fmt.Errorf("field: %s: disallow: kind %s is not allowed", path, ty.Kind()))
	}

	return errs
}

func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "pascal_case",
			input: "FooBar",
			want:  "foo_bar",
		},
		{
			name:  "camel_case",
			input: "fooBar",
			want:  "foo_bar",
		},
		{
			name:  "acronym_then_word",
			input: "FOOBar",
			want:  "foo_bar",
		},
		{
			name:  "word_then_acronym",
			input: "FooBAR",
			want:  "foo_bar",
		},
		{
			name:  "all_caps",
			input: "FOOBAR",
			want:  "foobar",
		},
		{
			name:  "single_uppercase",
			input: "F",
			want:  "f",
		},
		{
			name:  "single_lowercase",
			input: "f",
			want:  "f",
		},
		{
			name:  "with_numbers",
			input: "Foo123Bar",
			want:  "foo123_bar",
		},
		{
			name:  "with_trailing_numbers",
			input: "FooBar123",
			want:  "foo_bar123",
		},
		{
			name:  "empty_string",
			input: "",
			want:  "",
		},
		{
			name:    "invalid_character_space",
			input:   "hello world",
			wantErr: true,
		},
		{
			name:    "invalid_character_hyphen",
			input:   "foo-bar",
			wantErr: true,
		},
		{
			name:    "invalid_character_underscore",
			input:   "snake_case",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toSnakeCase(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("toSnakeCase(%q) expected error, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("toSnakeCase(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("toSnakeCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

type validStructAllTagged struct {
	FieldOne string `yaml:"field_one"`
	FieldTwo int    `yaml:"field_two"`
}

type missingTagStruct struct {
	FieldOne string `yaml:"field_one"`
	FieldTwo int
}

type wrongTagStruct struct {
	FieldOne string `yaml:"field_one"`
	FieldTwo int    `yaml:"wrong"`
}

type nonStringMapKey struct {
	Fields map[int]string `yaml:"fields"`
}

type disallowedKindPtr struct {
	Data *string `yaml:"data"`
}

type skipTagStruct struct {
	FieldOne string `yaml:"-"`
}

type unexportedFieldStruct struct {
	FieldOne string `yaml:"field_one"`
}

type nestedValidStruct struct {
	Inner validStructAllTagged `yaml:"inner"`
}

type sliceValidStruct struct {
	Items []validStructAllTagged `yaml:"items"`
}

type disallowedKindInterface struct {
	Data any `yaml:"data"`
}

func TestCheckYAMLTags(t *testing.T) {
	tests := []struct {
		name     string
		ty       reflect.Type
		wantErrs []string
	}{
		{name: "valid_struct", ty: reflect.TypeFor[validStructAllTagged](), wantErrs: nil},
		{name: "missing_tag", ty: reflect.TypeFor[missingTagStruct](), wantErrs: []string{
			"field missingTagStruct.FieldTwo: disallow: missing yaml tag",
		}},
		{name: "wrong_tag", ty: reflect.TypeFor[wrongTagStruct](), wantErrs: []string{
			`field wrongTagStruct.FieldTwo: disallow: yaml key "wrong" does not match field name (expected "field_two")`,
		}},
		{name: "non_string_map_key", ty: reflect.TypeFor[nonStringMapKey](), wantErrs: []string{
			"field nonStringMapKey.Fields: disallow: map key type int (must be string)",
		}},
		{name: "pointer_to_string", ty: reflect.TypeFor[disallowedKindPtr](), wantErrs: nil},
		{name: "skip_with_-_tag", ty: reflect.TypeFor[skipTagStruct](), wantErrs: nil},
		{name: "unexported_field_skip", ty: reflect.TypeFor[unexportedFieldStruct](), wantErrs: nil},
		{name: "nested_valid_struct", ty: reflect.TypeFor[nestedValidStruct](), wantErrs: nil},
		{name: "slice_of_valid_structs", ty: reflect.TypeFor[sliceValidStruct](), wantErrs: nil},
		{name: "interface_any", ty: reflect.TypeFor[disallowedKindInterface](), wantErrs: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := checkYAMLTags(tt.ty, tt.ty.Name())
			if len(errs) != len(tt.wantErrs) {
				t.Fatalf("got %d errors, want %d\n%s", len(errs), len(tt.wantErrs), formatErrors(errs))
			}
			for i, err := range errs {
				if err.Error() != tt.wantErrs[i] {
					t.Errorf("error %d:\ngot:  %s\nwant: %s", i, err.Error(), tt.wantErrs[i])
				}
			}
		})
	}
}

func formatErrors(errs []error) string {
	if len(errs) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, err := range errs {
		b.WriteString("  - ")
		b.WriteString(err.Error())
		b.WriteByte('\n')
	}
	return b.String()
}

func TestGoblinDir(t *testing.T) {
	t.Run("env_set", func(t *testing.T) {
		t.Setenv(goblinHomeEnv, "/custom/path")
		got, err := goblinDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != "/custom/path" {
			t.Errorf("goblinDir() = %q, want %q", got, "/custom/path")
		}
	})

	t.Run("env_unset", func(t *testing.T) {
		t.Setenv(goblinHomeEnv, "")
		got, err := goblinDir()
		if err != nil {
			t.Fatal(err)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".goblin")
		if got != want {
			t.Errorf("goblinDir() = %q, want %q", got, want)
		}
	})
}

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  GoblinConfig
		want string
	}{
		{name: "localhost_default", cfg: GoblinConfig{Address: "localhost:8080"}, want: "http://localhost:8080"},
		{name: "ipv4", cfg: GoblinConfig{Address: "127.0.0.1:8080"}, want: "http://127.0.0.1:8080"},
		{name: "all_zeros", cfg: GoblinConfig{Address: "0.0.0.0:9999"}, want: "http://0.0.0.0:9999"},
		{name: "port_one", cfg: GoblinConfig{Address: "localhost:1"}, want: "http://localhost:1"},
		{name: "port_max", cfg: GoblinConfig{Address: "localhost:65535"}, want: "http://localhost:65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.baseURL()
			if got != tt.want {
				t.Errorf("GoblinConfig{Address: %q}.baseURL() = %q, want %q", tt.cfg.Address, got, tt.want)
			}
		})
	}
}

func TestLoadGoblinConfig(t *testing.T) {
	t.Run("with_config_file", func(t *testing.T) {
		home := t.TempDir()
		data := []byte("address: 127.0.0.1:9999\n")
		if err := os.WriteFile(goblinConfigPath(home), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(goblinHomeEnv, home)

		cfg, err := loadGoblinConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Address != "127.0.0.1:9999" {
			t.Errorf("Address = %q, want %q", cfg.Address, "127.0.0.1:9999")
		}
	})

	t.Run("without_config_file", func(t *testing.T) {
		t.Setenv(goblinHomeEnv, t.TempDir())

		cfg, err := loadGoblinConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Address != "localhost:8080" {
			t.Errorf("default Address = %q, want %q", cfg.Address, "localhost:8080")
		}
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(goblinConfigPath(home), []byte(": : invalid"), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(goblinHomeEnv, home)

		if _, err := loadGoblinConfig(); err == nil {
			t.Error("loadGoblinConfig() expected error, got nil")
		}
	})

	t.Run("validation_error", func(t *testing.T) {
		home := t.TempDir()
		data := []byte("address: example.com:8080\n")
		if err := os.WriteFile(goblinConfigPath(home), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(goblinHomeEnv, home)

		if _, err := loadGoblinConfig(); err == nil {
			t.Error("loadGoblinConfig() expected error for invalid host, got nil")
		}
	})
}

func TestParseGoblinConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     GoblinConfig
		wantErr bool
	}{
		{
			name: "valid_default",
			cfg: GoblinConfig{
				Address: "localhost:8080",
			},
		},
		{
			name: "valid_ipv4",
			cfg: GoblinConfig{
				Address: "127.0.0.1:8080",
			},
		},
		{
			name:    "empty_host",
			cfg:     GoblinConfig{Address: ":8080"},
			wantErr: true,
		},
		{
			name: "valid_all_zeros",
			cfg: GoblinConfig{
				Address: "0.0.0.0:8080",
			},
		},
		{
			name: "valid_private_ip",
			cfg: GoblinConfig{
				Address: "192.168.1.1:8080",
			},
		},
		{
			name: "valid_port_min",
			cfg: GoblinConfig{
				Address: "localhost:1",
			},
		},
		{
			name: "valid_port_max",
			cfg: GoblinConfig{
				Address: "localhost:65535",
			},
		},
		{
			name: "empty_address_defaults_to_localhost",
			cfg:  GoblinConfig{Address: ""},
		},
		{
			name:    "hostname_instead_of_ip",
			cfg:     GoblinConfig{Address: "example.com:8080"},
			wantErr: true,
		},
		{
			name:    "missing_colon_and_port",
			cfg:     GoblinConfig{Address: "localhost"},
			wantErr: true,
		},
		{
			name:    "non_ip_string",
			cfg:     GoblinConfig{Address: "not-an-ip:8080"},
			wantErr: true,
		},
		{
			name:    "port_zero",
			cfg:     GoblinConfig{Address: "localhost:0"},
			wantErr: true,
		},
		{
			name:    "port_negative",
			cfg:     GoblinConfig{Address: "localhost:-1"},
			wantErr: true,
		},
		{
			name:    "port_too_high",
			cfg:     GoblinConfig{Address: "localhost:65536"},
			wantErr: true,
		},
		{
			name:    "port_max_int",
			cfg:     GoblinConfig{Address: "localhost:999999"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseGoblinConfig(&tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseGoblinConfig(%+v) expected error, got nil", tt.cfg)
				}
				return
			}
			if err != nil {
				t.Errorf("parseGoblinConfig(%+v) unexpected error: %v", tt.cfg, err)
			}
		})
	}
}

func TestHydrateModels_NilModel(t *testing.T) {
	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"test": {
				Models: map[string]*ModelInfo{
					"my-model": nil,
				},
			},
		},
	}
	HydrateModels(cfg)

	m := cfg.Providers["test"].Models["my-model"]
	if m.Slug != "test/my-model" {
		t.Errorf("Slug = %q, want %q", m.Slug, "test/my-model")
	}
	if m.Priority == nil || *m.Priority != 1 {
		t.Error("Priority should default to 1")
	}
	if m.Visibility != ModelVisibilityList {
		t.Errorf("Visibility = %q, want %q", m.Visibility, ModelVisibilityList)
	}
}

func TestHydrateModels_PreservesExplicitFields(t *testing.T) {
	p := new(3)
	cfg := &GoblinConfig{
		Providers: map[string]Provider{
			"crof": {
				Models: map[string]*ModelInfo{
					"kimi": {
						DisplayName: "Kimi K2.6",
						Priority:    p,
						Visibility:  "hidden",
					},
				},
			},
		},
	}
	HydrateModels(cfg)

	m := cfg.Providers["crof"].Models["kimi"]
	if m.Slug != "crof/kimi" {
		t.Errorf("Slug = %q, want %q", m.Slug, "crof/kimi")
	}
	if m.DisplayName != "Kimi K2.6" {
		t.Errorf("DisplayName = %q, want %q", m.DisplayName, "Kimi K2.6")
	}
	if *m.Priority != 3 {
		t.Errorf("Priority = %d, want 3", *m.Priority)
	}
	if m.Visibility != "hidden" {
		t.Errorf("Visibility = %q, want %q", m.Visibility, "hidden")
	}
}

func toSnakeCase(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z':
			if i > 0 {
				prev := s[i-1]
				next := byte(0)
				if i+1 < len(s) {
					next = s[i+1]
				}
				// e.g.) Fo(oBa)r, FO(OBa)r, Fo(oBA)R -> foo_bar
				if (prev >= 'a' && prev <= 'z') || (next >= 'a' && next <= 'z') {
					b.WriteByte('_')
				}
			}
			b.WriteByte(c + 32) // to lowercase
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9':
			b.WriteByte(c)
		default:
			return "", fmt.Errorf("invalid character %q in field name %q", c, s)
		}
	}
	return b.String(), nil
}
