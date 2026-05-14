package respond

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestConfigMapstructureTags(t *testing.T) {
	ty := reflect.TypeFor[Config]()
	checkMapstructureTags(t, ty, ty.Name())
}

func checkMapstructureTags(t *testing.T, ty reflect.Type, path string) {
	switch ty.Kind() {
	case reflect.Struct:
		for f := range ty.Fields() {
			if !f.IsExported() {
				continue
			}

			path := fmt.Sprintf("%s.%s", path, f.Name)
			tag := f.Tag.Get("mapstructure")
			if tag == "" {
				t.Errorf("field %s: missing mapstructure tag", path)
				continue
			}

			if tag == "-" {
				continue
			}

			expected, err := toSnakeCase(f.Name)
			if err != nil {
				t.Errorf("field %s: %v", path, err)
				continue
			}

			if tag != expected {
				t.Errorf("field %s: mapstructure key %q does not match field name (expected %q)",
					path, tag, expected)
			}

			checkMapstructureTags(t, f.Type, path)
		}
	case reflect.Map:
		if ty.Key().Kind() != reflect.String {
			t.Errorf("field %s: invalid key type %s", path, ty.Key())
			return
		}
		checkMapstructureTags(t, ty.Elem(), path)
	case reflect.Slice:
		checkMapstructureTags(t, ty.Elem(), path)
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8,
		reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32,
		reflect.Float64:
		// noop
	default:
		t.Errorf("field: %s: unexpected kind: %s", path, ty.Kind())
	}
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
