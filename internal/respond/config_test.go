package respond

import (
	"fmt"
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

type _validStructAllTagged struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int    `mapstructure:"field_two"`
}

type _missingTagStruct struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int
}

type _wrongTagStruct struct {
	FieldOne string `mapstructure:"field_one"`
	FieldTwo int    `mapstructure:"wrong"`
}

type _nonStringMapKey struct {
	Fields map[int]string `mapstructure:"fields"`
}

type _disallowedKindPtr struct {
	Data *string `mapstructure:"data"`
}

type _skipTagStruct struct {
	FieldOne string `mapstructure:"-"`
}

type _unexportedFieldStruct struct {
	FieldOne string `mapstructure:"field_one"`
	internal int
}

type _nestedValidStruct struct {
	Inner _validStructAllTagged `mapstructure:"inner"`
}

type _sliceValidStruct struct {
	Items []_validStructAllTagged `mapstructure:"items"`
}

type _disallowedKindInterface struct {
	Data any `mapstructure:"data"`
}

func TestCheckMapstructureTags(t *testing.T) {
	tests := []struct {
		name     string
		ty       reflect.Type
		wantErrs []string
	}{
		{name: "valid struct", ty: reflect.TypeFor[_validStructAllTagged](), wantErrs: nil},
		{name: "missing tag", ty: reflect.TypeFor[_missingTagStruct](), wantErrs: []string{
			"field _missingTagStruct.FieldTwo: disallow: missing mapstructure tag",
		}},
		{name: "wrong tag", ty: reflect.TypeFor[_wrongTagStruct](), wantErrs: []string{
			`field _wrongTagStruct.FieldTwo: disallow: mapstructure key "wrong" does not match field name (expected "field_two")`,
		}},
		{name: "non-string map key", ty: reflect.TypeFor[_nonStringMapKey](), wantErrs: []string{
			"field _nonStringMapKey.Fields: disallow: map key type int (must be string)",
		}},
		{name: "disallowed kind ptr", ty: reflect.TypeFor[_disallowedKindPtr](), wantErrs: []string{
			"field: _disallowedKindPtr.Data: disallow: kind ptr is not allowed",
		}},
		{name: "skip with - tag", ty: reflect.TypeFor[_skipTagStruct](), wantErrs: nil},
		{name: "unexported field skip", ty: reflect.TypeFor[_unexportedFieldStruct](), wantErrs: nil},
		{name: "nested valid struct", ty: reflect.TypeFor[_nestedValidStruct](), wantErrs: nil},
		{name: "slice of valid structs", ty: reflect.TypeFor[_sliceValidStruct](), wantErrs: nil},
		{name: "disallowed kind interface", ty: reflect.TypeFor[_disallowedKindInterface](), wantErrs: []string{
			"field: _disallowedKindInterface.Data: disallow: kind interface is not allowed",
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
