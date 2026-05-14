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
			name:  "pascal case",
			input: "FooBar",
			want:  "foo_bar",
		},
		{
			name:  "camel case",
			input: "fooBar",
			want:  "foo_bar",
		},
		{
			name:  "acronym then word",
			input: "FOOBar",
			want:  "foo_bar",
		},
		{
			name:  "word then acronym",
			input: "FooBAR",
			want:  "foo_bar",
		},
		{
			name:  "all caps",
			input: "FOOBAR",
			want:  "foobar",
		},
		{
			name:  "single uppercase",
			input: "F",
			want:  "f",
		},
		{
			name:  "single lowercase",
			input: "f",
			want:  "f",
		},
		{
			name:  "with numbers",
			input: "Foo123Bar",
			want:  "foo123_bar",
		},
		{
			name:  "with trailing numbers",
			input: "FooBar123",
			want:  "foo_bar123",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:    "invalid character space",
			input:   "hello world",
			wantErr: true,
		},
		{
			name:    "invalid character hyphen",
			input:   "foo-bar",
			wantErr: true,
		},
		{
			name:    "invalid character underscore",
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
		{name: "valid struct", ty: reflect.TypeFor[validStructAllTagged](), wantErrs: nil},
		{name: "missing tag", ty: reflect.TypeFor[missingTagStruct](), wantErrs: []string{
			"field missingTagStruct.FieldTwo: disallow: missing mapstructure tag",
		}},
		{name: "wrong tag", ty: reflect.TypeFor[wrongTagStruct](), wantErrs: []string{
			`field wrongTagStruct.FieldTwo: disallow: mapstructure key "wrong" does not match field name (expected "field_two")`,
		}},
		{name: "non-string map key", ty: reflect.TypeFor[nonStringMapKey](), wantErrs: []string{
			"field nonStringMapKey.Fields: disallow: map key type int (must be string)",
		}},
		{name: "disallowed kind ptr", ty: reflect.TypeFor[disallowedKindPtr](), wantErrs: []string{
			"field: disallowedKindPtr.Data: disallow: kind ptr is not allowed",
		}},
		{name: "skip with - tag", ty: reflect.TypeFor[skipTagStruct](), wantErrs: nil},
		{name: "unexported field skip", ty: reflect.TypeFor[unexportedFieldStruct](), wantErrs: nil},
		{name: "nested valid struct", ty: reflect.TypeFor[nestedValidStruct](), wantErrs: nil},
		{name: "slice of valid structs", ty: reflect.TypeFor[sliceValidStruct](), wantErrs: nil},
		{name: "disallowed kind interface", ty: reflect.TypeFor[disallowedKindInterface](), wantErrs: []string{
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

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name: "valid default",
			cfg: Config{
				Host: "localhost",
				Port: 8080,
			},
		},
		{
			name: "valid ipv4",
			cfg: Config{
				Host: "127.0.0.1",
				Port: 8080,
			},
		},
		{
			name: "valid all zeros",
			cfg: Config{
				Host: "0.0.0.0",
				Port: 8080,
			},
		},
		{
			name: "valid private ip",
			cfg: Config{
				Host: "192.168.1.1",
				Port: 8080,
			},
		},
		{
			name: "valid port min",
			cfg: Config{
				Host: "localhost",
				Port: 1,
			},
		},
		{
			name: "valid port max",
			cfg: Config{
				Host: "localhost",
				Port: 65535,
			},
		},
		{
			name:    "empty host",
			cfg:     Config{Host: "", Port: 8080},
			wantErr: true,
		},
		{
			name:    "hostname instead of ip",
			cfg:     Config{Host: "example.com", Port: 8080},
			wantErr: true,
		},
		{
			name:    "non ip string",
			cfg:     Config{Host: "not-an-ip", Port: 8080},
			wantErr: true,
		},
		{
			name:    "invalid ip with spaces",
			cfg:     Config{Host: " 192.168.1.1 ", Port: 8080},
			wantErr: true,
		},
		{
			name:    "port zero",
			cfg:     Config{Host: "localhost", Port: 0},
			wantErr: true,
		},
		{
			name:    "port negative",
			cfg:     Config{Host: "localhost", Port: -1},
			wantErr: true,
		},
		{
			name:    "port too high",
			cfg:     Config{Host: "localhost", Port: 65536},
			wantErr: true,
		},
		{
			name:    "port max int",
			cfg:     Config{Host: "localhost", Port: 999999},
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
