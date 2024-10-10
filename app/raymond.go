package main

import (
	"encoding/json"
	"fmt"
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

}

