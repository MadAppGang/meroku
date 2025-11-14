package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/aymerick/raymond"
)

// isTruthy checks if a value is considered "truthy" in Handlebars context
func isTruthy(value interface{}) bool {
	if value == nil {
		return false
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.String:
		return v.String() != ""
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() != 0
	case reflect.Float32, reflect.Float64:
		return v.Float() != 0
	case reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() > 0
	default:
		// For other types, non-nil is truthy
		return true
	}
}

func registerCustomHelpers() {
	// Register custom helper for array to JSON string conversion
	raymond.RegisterHelper("array", func(items interface{}) string {
		// Handle different input types more gracefully
		if items == nil {
			panic("array helper: received nil value")
		}
		
		// Use reflection to check if it's actually a slice
		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			panic(fmt.Sprintf("array helper: expected slice or array, got %T", items))
		}
		
		// Convert map[interface{}]interface{} to map[string]interface{} for JSON compatibility
		converted := convertToJSONCompatible(items)
		
		jsonBytes, err := json.Marshal(converted)
		if err != nil {
			panic(fmt.Sprintf("array helper: failed to marshal to JSON: %v", err))
		}
		return string(jsonBytes)
	})

	//   [{ "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint }, ...]
	raymond.RegisterHelper("envToEnvArray", func(t any) string {
		fmt.Printf("t: %+v\n", t)
		text, ok := t.(string)
		if !ok {
			fmt.Printf("could not convert envToEnvArray, expecting string, but type of t: %T\n", t)
			return "[]"
		}
		lines := strings.Split(strings.TrimSpace(text), "\n")
		result := make(map[string]string)

		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])
				// Remove surrounding quotes if present
				value = strings.Trim(value, "\"'")
				result[key] = value
			}
		}

		var tfMap strings.Builder
		tfMap.WriteString("[\n")
		for key, value := range result {
			tfMap.WriteString(fmt.Sprintf("    { \"name\" : \"%s\", \"value\" : \"%s\" },\n", key, value))
		}
		tfMap.WriteString(" ]")

		return tfMap.String()
	})

	// Helper for OR logic - used for Aurora capacity where 0 is valid
	// Check if value exists (not nil) - distinguishes between "value is 0/false" vs "value is missing"
	// Usage: {{#if (exists postgres.min_capacity)}}...{{/if}}
	raymond.RegisterHelper("exists", func(value interface{}) bool {
		return value != nil
	})

	// Logical OR - returns true if either argument is truthy
	// Usage: {{#if (or a b)}}...{{/if}}
	raymond.RegisterHelper("or", func(a, b interface{}) bool {
		return isTruthy(a) || isTruthy(b)
	})

	// Equality check - compares two values
	// Usage: {{#if (eq value 0)}}...{{/if}}
	raymond.RegisterHelper("eq", func(a, b interface{}) bool {
		// Handle nil cases
		if a == nil && b == nil {
			return true
		}
		if a == nil || b == nil {
			return false
		}

		// Use reflect to compare values properly
		va := reflect.ValueOf(a)
		vb := reflect.ValueOf(b)

		// If types are different, try string comparison
		if va.Kind() != vb.Kind() {
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		}

		// For same types, use direct comparison
		switch va.Kind() {
		case reflect.Bool:
			return va.Bool() == vb.Bool()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return va.Int() == vb.Int()
		case reflect.Float32, reflect.Float64:
			return va.Float() == vb.Float()
		case reflect.String:
			return va.String() == vb.String()
		default:
			return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
		}
	})

	raymond.RegisterHelper("compare", func(lvalue, operator string, rvalue string, options *raymond.Options) interface{} {
		result := false

		switch operator {
		case "==":
			result = (lvalue == rvalue)
		case "!=":
			result = (lvalue != rvalue)
		case ">":
			result = (lvalue > rvalue)
		case "<":
			result = (lvalue < rvalue)
		case ">=":
			result = (lvalue >= rvalue)
		case "<=":
			result = (lvalue <= rvalue)
		default:
			// Invalid operator
			return options.Inverse()
		}

		if result {
			return options.Fn()
		}
		return options.Inverse()
	})

	raymond.RegisterHelper("len", func(value interface{}) int {
		if value == nil {
			return 0
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Array, reflect.Slice, reflect.Map:
			return v.Len()
		case reflect.String:
			return len(v.String())
		default:
			fmt.Printf("len helper: unsupported type %T\n", value)
			return 0
		}
	})

	raymond.RegisterHelper("default", func(value any, defaultValue any) any {
		// Only return default if value is nil (field missing from YAML)
		// Since we use map[string]interface{} for template data:
		//   - Missing field → nil
		//   - Field with value 0/false/"" → that actual value
		if value == nil {
			return defaultValue
		}

		// Special handling for empty strings - treat as "not set"
		v := reflect.ValueOf(value)
		if v.Kind() == reflect.String && v.String() == "" {
			return defaultValue
		}

		// Special handling for empty slices/maps - treat as "not set"
		if (v.Kind() == reflect.Slice || v.Kind() == reflect.Map) && v.Len() == 0 {
			return defaultValue
		}

		// For all other types (including bool, int, float), the value is valid
		// This allows: false, 0, 0.0 to be legitimate values
		return value
	})

	raymond.RegisterHelper("notEmpty", func(value interface{}, options *raymond.Options) interface{} {
		if value == nil {
			return options.Inverse()
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				return options.Inverse()
			}
		case reflect.Slice, reflect.Map, reflect.Array:
			if v.Len() == 0 {
				return options.Inverse()
			}
		case reflect.Bool:
			if !v.Bool() {
				return options.Inverse()
			}
		}

		return options.Fn()
	})

	raymond.RegisterHelper("notZero", func(value interface{}, options *raymond.Options) interface{} {
		if value == nil {
			return options.Inverse()
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				return options.Inverse()
			}
		case reflect.Slice, reflect.Map, reflect.Array:
			if v.Len() == 0 {
				return options.Inverse()
			}
		case reflect.Bool:
			if !v.Bool() {
				return options.Inverse()
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if v.Int() == 0 {
				return options.Inverse()
			}
		case reflect.Float32, reflect.Float64:
			if v.Float() == 0 {
				return options.Inverse()
			}
		}

		return options.Fn()
	})

	raymond.RegisterHelper("isDefined", func(value interface{}, options *raymond.Options) interface{} {
		// Check if value is nil - this is the ONLY condition for undefined
		// Empty strings, 0, false, empty arrays etc. are all considered "defined"
		if value == nil {
			return options.Inverse()
		}
		return options.Fn()
	})

	raymond.RegisterHelper("mmap", func(value interface{}) string {
		if value == nil {
			return "{}"
		}

		// Handle case when input is a slice of maps with name/value keys
		if slice, ok := value.([]interface{}); ok {
			var builder strings.Builder
			builder.WriteString("{\n")

			for _, item := range slice {
				if m, ok := item.(map[string]interface{}); ok {
					// Extract name and value from each map
					if name, hasName := m["name"]; hasName {
						if value, hasValue := m["value"]; hasValue {
							strValue := fmt.Sprintf("%v", value)

							// Handle boolean values properly
							if strValue == "true" || strValue == "false" {
								builder.WriteString(fmt.Sprintf("  %s = %s\n", name, strValue))
							} else {
								// Quote all other values
								builder.WriteString(fmt.Sprintf("  %s = \"%s\"\n", name, strValue))
							}
						}
					}
				}
			}

			builder.WriteString("}")
			return builder.String()
		}

		// If it's already a map, format it as Terraform map
		if m, ok := value.(map[string]interface{}); ok {
			var builder strings.Builder
			builder.WriteString("{\n")

			for k, v := range m {
				strValue := fmt.Sprintf("%v", v)

				// Handle boolean values properly
				if strValue == "true" || strValue == "false" {
					builder.WriteString(fmt.Sprintf("  %s = %s\n", k, strValue))
				} else {
					// Quote all other values
					builder.WriteString(fmt.Sprintf("  %s = \"%s\"\n", k, strValue))
				}
			}

			builder.WriteString("}")
			return builder.String()
		}

		return "{}"
	})

	raymond.RegisterHelper("envArray", func(value interface{}) string {
		if value == nil {
			return "[]"
		}

		var builder strings.Builder
		builder.WriteString("[\n")

		// Handle case when input is already a slice of maps with name/value keys
		if slice, ok := value.([]interface{}); ok {
			for _, item := range slice {
				if m, ok := item.(map[string]interface{}); ok {
					if name, hasName := m["name"]; hasName {
						if val, hasValue := m["value"]; hasValue {
							builder.WriteString(fmt.Sprintf("    { \"name\" : \"%v\", \"value\" : \"%v\" },\n", name, val))
						}
					}
				}
			}
		} else if m, ok := value.(map[string]interface{}); ok {
			// If it's a regular map, convert to name/value format
			for k, v := range m {
				builder.WriteString(fmt.Sprintf("    { \"name\" : \"%s\", \"value\" : \"%v\" },\n", k, v))
			}
		} else if m, ok := value.(map[interface{}]interface{}); ok {
			// Handle map[interface{}]interface{} from YAML unmarshaling
			for k, v := range m {
				builder.WriteString(fmt.Sprintf("    { \"name\" : \"%v\", \"value\" : \"%v\" },\n", k, v))
			}
		}

		builder.WriteString("  ]")
		return builder.String()
	})
}

// convertToJSONCompatible converts map[interface{}]interface{} to map[string]interface{}
// This is necessary because YAML unmarshaling creates map[interface{}]interface{} which
// is not compatible with JSON marshaling
func convertToJSONCompatible(data interface{}) interface{} {
	switch v := data.(type) {
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for key, value := range v {
			m[fmt.Sprintf("%v", key)] = convertToJSONCompatible(value)
		}
		return m
	case []interface{}:
		for i, item := range v {
			v[i] = convertToJSONCompatible(item)
		}
		return v
	default:
		return v
	}
}
