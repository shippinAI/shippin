package api

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

func contractValue(docs map[string]map[string]any, file string, s map[string]any, v any) error {
	if ref, ok := s["$ref"].(string); ok {
		p, key, _ := strings.Cut(ref, "#/")
		if p != "" {
			file = filepath.ToSlash(filepath.Join(filepath.Dir(file), p))
		}
		return contractValue(docs, file, docs[file][key].(map[string]any), v)
	}
	if all, ok := s["allOf"].([]any); ok {
		for _, sub := range all {
			if err := contractValue(docs, file, sub.(map[string]any), v); err != nil {
				return err
			}
		}
	}
	if options, ok := s["oneOf"].([]any); ok {
		for _, sub := range options {
			if contractValue(docs, file, sub.(map[string]any), v) == nil {
				return nil
			}
		}
		return fmt.Errorf("no allowed type: %v", v)
	}
	if values, ok := s["enum"].([]any); ok {
		found := false
		for _, value := range values {
			found = found || reflect.DeepEqual(value, v)
		}
		if !found {
			return fmt.Errorf("outside enum: %v", v)
		}
	}
	if typ, ok := s["type"].(string); ok {
		valid := true
		switch typ {
		case "object":
			_, valid = v.(map[string]any)
		case "array":
			_, valid = v.([]any)
		case "string":
			_, valid = v.(string)
		case "integer":
			n, ok := v.(float64)
			valid = ok && n == float64(int64(n))
		case "boolean":
			_, valid = v.(bool)
		case "null":
			valid = v == nil
		}
		if !valid {
			return fmt.Errorf("expected %s got %T", typ, v)
		}
	}
	if obj, ok := v.(map[string]any); ok {
		if required, ok := s["required"].([]any); ok {
			for _, key := range required {
				if _, ok := obj[key.(string)]; !ok {
					return fmt.Errorf("missing required %s", key)
				}
			}
		}
		if props, ok := s["properties"].(map[string]any); ok {
			for key, value := range obj {
				if ps, ok := props[key].(map[string]any); ok {
					if err := contractValue(docs, file, ps, value); err != nil {
						return fmt.Errorf("%s: %w", key, err)
					}
				}
			}
		}
	}
	return nil
}
