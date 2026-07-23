package milx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

func CanonicalJSON(v any) ([]byte, error) {
	if err := rejectForbidden(v); err != nil {
		return nil, err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, &MILXError{Code: "GPH_MILX_OUTPUT_INVALID", Stage: "canonical", SanitizedSummary: "value cannot be serialized"}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var x any
	if err = dec.Decode(&x); err != nil {
		return nil, err
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("trailing JSON")
	}
	if err = rejectNumbers(x); err != nil {
		return nil, err
	}
	out, err := json.Marshal(canonicalValue(x))
	if err != nil {
		return nil, err
	}
	if len(out) > MaxFrameBytes {
		return nil, &MILXError{Code: "GPH_MILX_OUTPUT_INVALID", Stage: "canonical", SanitizedSummary: "canonical payload exceeds frame limit"}
	}
	return out, nil
}
func canonicalValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		y := make(map[string]any, len(x))
		for _, k := range keys {
			y[k] = canonicalValue(x[k])
		}
		return y
	case []any:
		for i := range x {
			x[i] = canonicalValue(x[i])
		}
		return x
	default:
		return v
	}
}
func rejectNumbers(v any) error {
	switch x := v.(type) {
	case json.Number:
		if bytes.ContainsAny([]byte(x.String()), ".eE") {
			return fmt.Errorf("non-integer number")
		}
	case []any:
		for _, e := range x {
			if err := rejectNumbers(e); err != nil {
				return err
			}
		}
	case map[string]any:
		for _, e := range x {
			if err := rejectNumbers(e); err != nil {
				return err
			}
		}
	}
	return nil
}
func rejectForbidden(v any) error {
	forbidden := map[string]bool{"network": true, "mcp": true, "secrets": true, "secret": true, "write": true, "db_handle": true, "database_write_handle": true, "arbitrary_host_path": true}
	var walk func(reflect.Value) error
	walk = func(r reflect.Value) error {
		if !r.IsValid() {
			return nil
		}
		if r.Kind() == reflect.Interface || r.Kind() == reflect.Pointer {
			return walk(r.Elem())
		}
		switch r.Kind() {
		case reflect.Struct:
			for i := 0; i < r.NumField(); i++ {
				name := r.Type().Field(i).Name
				if forbidden[strings.ToLower(name)] || forbidden[strings.ToLower(jsonName(r.Type().Field(i).Tag.Get("json")))] {
					return fmt.Errorf("forbidden field")
				}
				if err := walk(r.Field(i)); err != nil {
					return err
				}
			}
		case reflect.Map:
			for _, k := range r.MapKeys() {
				if k.Kind() == reflect.String && forbidden[strings.ToLower(k.String())] {
					return fmt.Errorf("forbidden key")
				}
				if err := walk(r.MapIndex(k)); err != nil {
					return err
				}
			}
		case reflect.Slice, reflect.Array:
			for i := 0; i < r.Len(); i++ {
				if err := walk(r.Index(i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(reflect.ValueOf(v)); err != nil {
		return &MILXError{Code: "GPH_MILX_CAPABILITY_DENIED", Stage: "canonical", SanitizedSummary: "forbidden capability or field"}
	}
	return nil
}
func jsonName(s string) string {
	if i := bytes.IndexByte([]byte(s), ','); i >= 0 {
		s = s[:i]
	}
	return s
}
func DecodeCanonical(data []byte, out any) error {
	if len(data) > MaxFrameBytes || bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) || !utf8Valid(data) {
		return fmt.Errorf("invalid canonical JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	if err := rejectNumbers(value); err != nil {
		return err
	}
	canonical, err := json.Marshal(canonicalValue(value))
	if err != nil || !bytes.Equal(data, canonical) {
		return fmt.Errorf("invalid canonical JSON")
	}
	return json.Unmarshal(data, out)
}
func utf8Valid(b []byte) bool { return utf8.Valid(b) && json.Valid(b) }
