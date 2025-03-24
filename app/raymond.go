package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/aymerick/raymond"
)

func registerCustomHelpers() {
	// Register custom helper for array to JSON string conversion
	raymond.RegisterHelper("array", func(items []any) string {
		jsonBytes, err := json.Marshal(items)
		if err != nil {
			fmt.Printf("Error marshaling array to JSON: %+v\n", err)
			return "[]"
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
		if value == nil {
			return defaultValue
		}

		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.String:
			if v.String() == "" {
				return defaultValue
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if v.Int() == 0 {
				return defaultValue
			}
		case reflect.Float32, reflect.Float64:
			if v.Float() == 0 {
				return defaultValue
			}
		case reflect.Slice, reflect.Map:
			if v.Len() == 0 {
				return defaultValue
			}
		}

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
		}

		builder.WriteString("  ]")
		return builder.String()
	})
}
