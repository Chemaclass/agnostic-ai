package authority

import (
	"reflect"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

// Matching reads the original specs; only completed display values are sanitized.
func redactDisplay(value any) {
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		switch v.Kind() {
		case reflect.Pointer, reflect.Interface:
			if !v.IsNil() {
				walk(v.Elem())
			}
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		case reflect.String:
			if v.CanSet() {
				v.SetString(preview.RedactText(v.String()))
			}
		}
	}
	walk(reflect.ValueOf(value))
}

func displayID(raw string) string {
	if preview.RedactText(raw) != raw {
		return slug(raw)
	}
	return raw
}
