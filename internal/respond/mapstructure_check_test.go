package respond

import (
	"testing"
)

func TestCheckMapstructureTagsErrors(t *testing.T) {
	tests := []struct {
		name string
		v    any
		want int // expected number of errors
	}{
		{
			name: "valid flat struct",
			v: struct {
				Name string `mapstructure:"name"`
				Age  int    `mapstructure:"age"`
			}{},
			want: 0,
		},
		{
			name: "missing tag",
			v: struct {
				Name string `mapstructure:"name"`
				Age  int
			}{},
			want: 1,
		},
		{
			name: "hyphen skip",
			v: struct {
				Name   string `mapstructure:"name"`
				Secret string `mapstructure:"-"`
			}{},
			want: 0,
		},
		{
			name: "empty tag",
			v: struct {
				Name string `mapstructure:""`
			}{},
			want: 1,
		},
		{
			name: "uppercase in tag",
			v: struct {
				Name string `mapstructure:"FullName"`
			}{},
			want: 1,
		},
		{
			name: "leading underscore",
			v: struct {
				Name string `mapstructure:"_name"`
			}{},
			want: 1,
		},
		{
			name: "invalid character",
			v: struct {
				Name string `mapstructure:"my-name"`
			}{},
			want: 1,
		},
		{
			name: "unexported field ignored",
			v: struct {
				Public  string `mapstructure:"public"`
				private string
			}{},
			want: 0,
		},
		{
			name: "nested valid",
			v: struct {
				Outer struct {
					Inner string `mapstructure:"inner"`
				} `mapstructure:"outer"`
			}{},
			want: 0,
		},
		{
			name: "nested missing tag",
			v: struct {
				Outer struct {
					Inner string
				} `mapstructure:"outer"`
			}{},
			want: 1,
		},
		{
			name: "map value struct valid",
			v: struct {
				Items map[string]struct {
					Label string `mapstructure:"label"`
				} `mapstructure:"items"`
			}{},
			want: 0,
		},
		{
			name: "map value struct bad tag",
			v: struct {
				Items map[string]struct {
					Label string
				} `mapstructure:"items"`
			}{},
			want: 1,
		},
		{
			name: "slice elem struct valid",
			v: struct {
				Items []struct {
					Label string `mapstructure:"label"`
				} `mapstructure:"items"`
			}{},
			want: 0,
		},
		{
			name: "slice elem struct bad tag",
			v: struct {
				Items []struct {
					Label string
				} `mapstructure:"items"`
			}{},
			want: 1,
		},
		{
			name: "pointer to struct valid",
			v: struct {
				Item *struct {
					Label string `mapstructure:"label"`
				} `mapstructure:"item"`
			}{},
			want: 0,
		},
		{
			name: "pointer to struct bad tag",
			v: struct {
				Item *struct {
					Label string
				} `mapstructure:"item"`
			}{},
			want: 1,
		},
		{
			name: "empty struct",
			v:    struct{}{},
			want: 0,
		},
		{
			name: "non-struct value",
			v:    "hello",
			want: 0,
		},
		{
			name: "nil pointer",
			v:    (*struct{})(nil),
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckMapstructureTagsErrors(tt.v, "")
			if len(got) != tt.want {
				t.Errorf("got %d errors, want %d\n  errors: %v", len(got), tt.want, got)
			}
		})
	}
}

// Smoke test: CheckMapstructureTags doesn't panic on valid structs.
func TestCheckMapstructureTagsCalled(t *testing.T) {
	v := struct {
		Name string `mapstructure:"name"`
	}{}
	// This should not call t.Errorf for valid input.
	CheckMapstructureTags(t, v, "")
}
