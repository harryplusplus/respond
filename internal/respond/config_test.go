package respond

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfigMapstructureTags(t *testing.T) {
	ty := reflect.TypeFor[Config]()
	for _, err := range checkMapstructureTags(ty, ty.Name()) {
		t.Error(err)
	}
}

func checkMapstructureTags(ty reflect.Type, path string) []error {
	var errs []error

	switch ty.Kind() {
	case reflect.Struct:
		for f := range ty.Fields() {
			if !f.IsExported() {
				continue
			}

			fieldPath := fmt.Sprintf("%s.%s", path, f.Name)
			tag := f.Tag.Get("mapstructure")
			if tag == "" {
				errs = append(errs, fmt.Errorf("field %s: disallow: missing mapstructure tag", fieldPath))
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
				errs = append(errs, fmt.Errorf("field %s: disallow: mapstructure key %q does not match field name (expected %q)",
					fieldPath, tag, expected))
			}

			errs = append(errs, checkMapstructureTags(f.Type, fieldPath)...)
		}
	case reflect.Map:
		if ty.Key().Kind() != reflect.String {
			errs = append(errs, fmt.Errorf("field %s: disallow: map key type %s (must be string)", path, ty.Key()))
			return errs
		}
		errs = append(errs, checkMapstructureTags(ty.Elem(), path)...)
	case reflect.Slice:
		errs = append(errs, checkMapstructureTags(ty.Elem(), path)...)
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
		reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32,
		reflect.Float64:
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

// -- test helpers for TestCheckMapstructureTags --

type validStructAllTagged struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int    `mapstructure:"field_two"`
}

type missingTagStruct struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int
}

type wrongTagStruct struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int    `mapstructure:"wrong"`
}

type nonStringMapKey struct {
	Fields map[int]string `mapstructure:"fields"`
}

type disallowedKindPtr struct {
	Data *string `mapstructure:"data"`
}

type skipTagStruct struct {
	FieldOne string `mapstructure:"-"`
}

type unexportedFieldStruct struct {
	FieldOne string `mapstructure:"field_one"`
	internal int
}

type nestedValidStruct struct {
	Inner validStructAllTagged `mapstructure:"inner"`
}

type sliceValidStruct struct {
	Items []validStructAllTagged `mapstructure:"items"`
}

type disallowedKindInterface struct {
	Data any `mapstructure:"data"`
}

func TestCheckMapstructureTags(t *testing.T) {
	tests := []struct {
		name     string
		ty       reflect.Type
		wantErrs []string
	}{
		{name: "valid_struct", ty: reflect.TypeFor[validStructAllTagged](), wantErrs: nil},
		{name: "missing_tag", ty: reflect.TypeFor[missingTagStruct](), wantErrs: []string{
			"field missingTagStruct.FieldTwo: disallow: missing mapstructure tag",
		}},
		{name: "wrong_tag", ty: reflect.TypeFor[wrongTagStruct](), wantErrs: []string{
			`field wrongTagStruct.FieldTwo: disallow: mapstructure key "wrong" does not match field name (expected "field_two")`,
		}},
		{name: "non_string_map_key", ty: reflect.TypeFor[nonStringMapKey](), wantErrs: []string{
			"field nonStringMapKey.Fields: disallow: map key type int (must be string)",
		}},
		{name: "disallowed_kind_ptr", ty: reflect.TypeFor[disallowedKindPtr](), wantErrs: []string{
			"field: disallowedKindPtr.Data: disallow: kind ptr is not allowed",
		}},
		{name: "skip_with_-_tag", ty: reflect.TypeFor[skipTagStruct](), wantErrs: nil},
		{name: "unexported_field_skip", ty: reflect.TypeFor[unexportedFieldStruct](), wantErrs: nil},
		{name: "nested_valid_struct", ty: reflect.TypeFor[nestedValidStruct](), wantErrs: nil},
		{name: "slice_of_valid_structs", ty: reflect.TypeFor[sliceValidStruct](), wantErrs: nil},
		{name: "disallowed_kind_interface", ty: reflect.TypeFor[disallowedKindInterface](), wantErrs: []string{
			"field: disallowedKindInterface.Data: disallow: kind interface is not allowed",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := checkMapstructureTags(tt.ty, tt.ty.Name())
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

func TestRespondDir(t *testing.T) {
	t.Run("env_set", func(t *testing.T) {
		t.Setenv(respondHomeEnv, "/custom/path")
		got, err := respondDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != "/custom/path" {
			t.Errorf("respondDir() = %q, want %q", got, "/custom/path")
		}
	})

	t.Run("env_unset", func(t *testing.T) {
		t.Setenv(respondHomeEnv, "")
		got, err := respondDir()
		if err != nil {
			t.Fatal(err)
		}
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".respond")
		if got != want {
			t.Errorf("respondDir() = %q, want %q", got, want)
		}
	})
}

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want string
	}{
		{name: "localhost_default", cfg: Config{Address: "localhost:8080"}, want: "http://localhost:8080"},
		{name: "ipv4", cfg: Config{Address: "127.0.0.1:8080"}, want: "http://127.0.0.1:8080"},
		{name: "all_zeros", cfg: Config{Address: "0.0.0.0:9999"}, want: "http://0.0.0.0:9999"},
		{name: "port_one", cfg: Config{Address: "localhost:1"}, want: "http://localhost:1"},
		{name: "port_max", cfg: Config{Address: "localhost:65535"}, want: "http://localhost:65535"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.baseURL()
			if got != tt.want {
				t.Errorf("Config{Address: %q}.baseURL() = %q, want %q", tt.cfg.Address, got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	t.Run("with_config_file", func(t *testing.T) {
		home := t.TempDir()
		data := []byte("address: 127.0.0.1:9999\n")
		if err := os.WriteFile(respondConfigPath(home), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(respondHomeEnv, home)

		cfg, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Address != "127.0.0.1:9999" {
			t.Errorf("Address = %q, want %q", cfg.Address, "127.0.0.1:9999")
		}
	})

	t.Run("without_config_file", func(t *testing.T) {
		t.Setenv(respondHomeEnv, t.TempDir())

		cfg, err := loadConfig()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Address != "0.0.0.0:8080" {
			t.Errorf("default Address = %q, want %q", cfg.Address, "0.0.0.0:8080")
		}
	})

	t.Run("invalid_yaml", func(t *testing.T) {
		home := t.TempDir()
		if err := os.WriteFile(respondConfigPath(home), []byte(": : invalid"), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(respondHomeEnv, home)

		if _, err := loadConfig(); err == nil {
			t.Error("loadConfig() expected error, got nil")
		}
	})

	t.Run("validation_error", func(t *testing.T) {
		home := t.TempDir()
		data := []byte("address: example.com:8080\n")
		if err := os.WriteFile(respondConfigPath(home), data, 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv(respondHomeEnv, home)

		if _, err := loadConfig(); err == nil {
			t.Error("loadConfig() expected error for invalid host, got nil")
		}
	})
}

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid_default",
			cfg: Config{
				Address: "localhost:8080",
			},
		},
		{
			name: "valid_ipv4",
			cfg: Config{
				Address: "127.0.0.1:8080",
			},
		},
		{
			name:    "empty_host",
			cfg:     Config{Address: ":8080"},
			wantErr: true,
		},
		{
			name: "valid_all_zeros",
			cfg: Config{
				Address: "0.0.0.0:8080",
			},
		},
		{
			name: "valid_private_ip",
			cfg: Config{
				Address: "192.168.1.1:8080",
			},
		},
		{
			name: "valid_port_min",
			cfg: Config{
				Address: "localhost:1",
			},
		},
		{
			name: "valid_port_max",
			cfg: Config{
				Address: "localhost:65535",
			},
		},
		{
			name:    "empty_address",
			cfg:     Config{Address: ""},
			wantErr: true,
		},
		{
			name:    "hostname_instead_of_ip",
			cfg:     Config{Address: "example.com:8080"},
			wantErr: true,
		},
		{
			name:    "missing_colon_and_port",
			cfg:     Config{Address: "localhost"},
			wantErr: true,
		},
		{
			name:    "non_ip_string",
			cfg:     Config{Address: "not-an-ip:8080"},
			wantErr: true,
		},
		{
			name:    "port_zero",
			cfg:     Config{Address: "localhost:0"},
			wantErr: true,
		},
		{
			name:    "port_negative",
			cfg:     Config{Address: "localhost:-1"},
			wantErr: true,
		},
		{
			name:    "port_too_high",
			cfg:     Config{Address: "localhost:65536"},
			wantErr: true,
		},
		{
			name:    "port_max_int",
			cfg:     Config{Address: "localhost:999999"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := parseConfig(&tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseConfig(%+v) expected error, got nil", tt.cfg)
				}
				return
			}
			if err != nil {
				t.Errorf("parseConfig(%+v) unexpected error: %v", tt.cfg, err)
			}
		})
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
