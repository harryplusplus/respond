package respond

import (
	"reflect"
	"strings"
	"testing"
	"unicode"
)

// CheckMapstructureTags verifies that all exported fields of v and its
// nested structs (map values, slice elements, pointer targets) have a
// non-empty, valid snake_case mapstructure tag.
func CheckMapstructureTags(t *testing.T, v any, prefix string) {
	for _, err := range CheckMapstructureTagsErrors(v, prefix) {
		t.Error(err)
	}
}

// CheckMapstructureTagsErrors returns all mapstructure tag issues found in v.
func CheckMapstructureTagsErrors(v any, prefix string) []string {
	var errs []string
	collectTagErrors(v, prefix, &errs)
	return errs
}

func collectTagErrors(v any, prefix string, errs *[]string) {
	typ := indirectType(reflect.TypeOf(v))
	if typ.Kind() != reflect.Struct {
		return
	}

	for field := range typ.Fields() {
		if !field.IsExported() {
			continue
		}

		tag := field.Tag.Get("mapstructure")
		if tag == "" || strings.TrimSpace(tag) == "" {
			*errs = append(*errs, prefix+field.Name+": missing or empty mapstructure tag")
			continue
		}
		if tag == "-" {
			continue
		}
		if !isValidSnakeCase(tag) {
			*errs = append(*errs, prefix+field.Name+": mapstructure tag "+tag+" is not valid snake_case")
		}

		elemType := elemType(field.Type)
		if elemType != nil && elemType.Kind() == reflect.Struct {
			collectTagErrors(reflect.New(elemType).Elem().Interface(), prefix+field.Name+".", errs)
		}
	}
}

func indirectType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func elemType(typ reflect.Type) reflect.Type {
	switch typ.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return typ.Elem()
	case reflect.Pointer:
		e := typ.Elem()
		if e.Kind() == reflect.Struct {
			return e
		}
	}
	return nil
}

func isValidSnakeCase(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if unicode.IsUpper(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			return false
		}
		if i == 0 && r == '_' {
			return false
		}
	}
	return true
}
