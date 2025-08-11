package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

	"github.com/aymerick/raymond"
	"gopkg.in/yaml.v3"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage: go run generate_tf.go <yaml_file> <template_file> <output_file>")
		os.Exit(1)
	}

	yamlFile := os.Args[1]
	templateFile := os.Args[2]
	outputFile := os.Args[3]

	// Register custom helpers
	registerHelpers()

	// Read and parse YAML
	yamlData, err := ioutil.ReadFile(yamlFile)
	if err != nil {
		fmt.Printf("Error reading YAML file: %v\n", err)
		os.Exit(1)
	}

	var config map[string]interface{}
	err = yaml.Unmarshal(yamlData, &config)
	if err != nil {
		fmt.Printf("Error parsing YAML: %v\n", err)
		os.Exit(1)
	}

	// Read template
	templateData, err := ioutil.ReadFile(templateFile)
	if err != nil {
		fmt.Printf("Error reading template file: %v\n", err)
		os.Exit(1)
	}

	// Parse and execute template
	tmpl, err := raymond.Parse(string(templateData))
	if err != nil {
		fmt.Printf("Error parsing template: %v\n", err)
		os.Exit(1)
	}

	// Configure raymond to not escape HTML
	raymond.RegisterHelper("raw", func(s string) raymond.SafeString {
		return raymond.SafeString(s)
	})

	result, err := tmpl.Exec(config)
	if err != nil {
		fmt.Printf("Error executing template: %v\n", err)
		os.Exit(1)
	}

	// Fix any HTML entities that might have been escaped
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&amp;", "&")

	// Write output
	err = ioutil.WriteFile(outputFile, []byte(result), 0644)
	if err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated %s\n", outputFile)
}

func registerHelpers() {
	// Register array helper
	raymond.RegisterHelper("array", func(items interface{}) string {
		if items == nil {
			return "[]"
		}

		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			return "[]"
		}

		converted := convertToJSONCompatible(items)
		jsonBytes, err := json.Marshal(converted)
		if err != nil {
			return "[]"
		}
		return string(jsonBytes)
	})

	// Register envArray helper for environment variables
	raymond.RegisterHelper("envArray", func(envMap interface{}) string {
		if envMap == nil {
			return "[]"
		}

		m, ok := envMap.(map[string]interface{})
		if !ok {
			return "[]"
		}

		var result []string
		for key, value := range m {
			result = append(result, fmt.Sprintf(`{ "name" : "%s", "value" : "%v" }`, key, value))
		}

		return "[\n    " + strings.Join(result, ",\n    ") + "\n  ]"
	})

	// Register compare helper
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
		}

		if result {
			return options.Fn()
		}
		return options.Inverse()
	})

	// Register len helper
	raymond.RegisterHelper("len", func(value interface{}) int {
		if value == nil {
			return 0
		}
		v := reflect.ValueOf(value)
		switch v.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map, reflect.String:
			return v.Len()
		default:
			return 0
		}
	})

	// Register default helper
	raymond.RegisterHelper("default", func(value interface{}, defaultValue interface{}) interface{} {
		if value == nil || value == "" || value == false || value == 0 {
			return defaultValue
		}
		return value
	})
}

func convertToJSONCompatible(v interface{}) interface{} {
	switch val := v.(type) {
	case map[interface{}]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			result[fmt.Sprintf("%v", k)] = convertToJSONCompatible(v)
		}
		return result
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, v := range val {
			result[i] = convertToJSONCompatible(v)
		}
		return result
	default:
		return val
	}
}
