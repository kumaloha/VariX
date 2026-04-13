package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"gopkg.in/yaml.v3"
)

// LoadYAML reads a single YAML file relative to projectRoot into T.
// T should be a struct with yaml tags. Pointer fields preserve nil vs
// zero-value distinction, enabling layered config via Overlay.
func LoadYAML[T any](projectRoot, relPath string) (T, error) {
	var result T
	path := filepath.Join(projectRoot, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("config: read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return result, nil
}

// Overlay merges src into dst using pointer-field overlay semantics:
//   - Pointer fields: overwrite only when src is non-nil
//   - Struct fields: recurse into nested structs
//   - Map fields: merge keys (src overwrites dst per-key; dst map created if nil)
//   - Slice fields: replace entirely (no append)
//   - Scalar fields: always overwrite
func Overlay[T any](dst *T, src T) error {
	if dst == nil {
		return fmt.Errorf("config: Overlay called with nil dst")
	}
	dv := reflect.ValueOf(dst).Elem()
	sv := reflect.ValueOf(src)
	return overlayValue(dv, sv)
}

func overlayValue(dst, src reflect.Value) error {
	if dst.Type() != src.Type() {
		return fmt.Errorf("config: overlay type mismatch: %s vs %s", dst.Type(), src.Type())
	}

	switch dst.Kind() {
	case reflect.Struct:
		return overlayStruct(dst, src)
	case reflect.Ptr:
		return overlayPtr(dst, src)
	case reflect.Map:
		return overlayMap(dst, src)
	default:
		// Scalars, slices, arrays, etc: always overwrite.
		dst.Set(src)
		return nil
	}
}

func overlayStruct(dst, src reflect.Value) error {
	t := dst.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		df := dst.Field(i)
		svf := src.Field(i)
		if err := overlayValue(df, svf); err != nil {
			return fmt.Errorf("field %s: %w", sf.Name, err)
		}
	}
	return nil
}

func overlayPtr(dst, src reflect.Value) error {
	// Only overwrite if src pointer is non-nil.
	if src.IsNil() {
		return nil
	}
	dst.Set(src)
	return nil
}

func overlayMap(dst, src reflect.Value) error {
	if src.IsNil() {
		return nil
	}
	// Ensure dst map is initialized.
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}
	for _, key := range src.MapKeys() {
		srcVal := src.MapIndex(key)
		dstVal := dst.MapIndex(key)

		// If both sides have a struct value, recurse to merge fields.
		if dstVal.IsValid() && srcVal.Kind() == reflect.Struct && dstVal.Kind() == reflect.Struct {
			// Map values aren't addressable, so copy into a temp, overlay, put back.
			merged := reflect.New(srcVal.Type()).Elem()
			merged.Set(dstVal)
			if err := overlayValue(merged, srcVal); err != nil {
				return fmt.Errorf("map key %v: %w", key.Interface(), err)
			}
			dst.SetMapIndex(key, merged)
		} else {
			dst.SetMapIndex(key, srcVal)
		}
	}
	return nil
}
