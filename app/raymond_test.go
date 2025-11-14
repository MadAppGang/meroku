package main

import (
	"testing"

	"github.com/aymerick/raymond"
)

// TestHelpers tests all custom Handlebars helpers
func TestHelpers(t *testing.T) {
	// Register helpers before running tests
	registerCustomHelpers()

	tests := []struct {
		name     string
		template string
		data     map[string]interface{}
		expected string
	}{
		// exists helper tests
		{
			name:     "exists: value is 0 (should be true)",
			template: `{{#if (exists value)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"value": 0},
			expected: "YES",
		},
		{
			name:     "exists: value is false (should be true)",
			template: `{{#if (exists value)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"value": false},
			expected: "YES",
		},
		{
			name:     "exists: value is empty string (should be true)",
			template: `{{#if (exists value)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"value": ""},
			expected: "YES",
		},
		{
			name:     "exists: value is missing (should be false)",
			template: `{{#if (exists value)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{},
			expected: "NO",
		},
		{
			name:     "exists: value is 0.5 (should be true)",
			template: `{{#if (exists value)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"value": 0.5},
			expected: "YES",
		},

		// or helper tests
		{
			name:     "or: both false",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": false, "b": false},
			expected: "NO",
		},
		{
			name:     "or: first true",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": true, "b": false},
			expected: "YES",
		},
		{
			name:     "or: second true",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": false, "b": true},
			expected: "YES",
		},
		{
			name:     "or: both true",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": true, "b": true},
			expected: "YES",
		},
		{
			name:     "or: 0 and non-zero",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 0, "b": 5},
			expected: "YES",
		},
		{
			name:     "or: both 0",
			template: `{{#if (or a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 0, "b": 0},
			expected: "NO",
		},

		// eq helper tests
		{
			name:     "eq: integers equal",
			template: `{{#if (eq a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 5, "b": 5},
			expected: "YES",
		},
		{
			name:     "eq: integers not equal",
			template: `{{#if (eq a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 5, "b": 10},
			expected: "NO",
		},
		{
			name:     "eq: zero equals zero",
			template: `{{#if (eq a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 0, "b": 0},
			expected: "YES",
		},
		{
			name:     "eq: float equals zero",
			template: `{{#if (eq a 0)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": 0.0},
			expected: "YES",
		},
		{
			name:     "eq: booleans equal",
			template: `{{#if (eq a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": false, "b": false},
			expected: "YES",
		},
		{
			name:     "eq: strings equal",
			template: `{{#if (eq a b)}}YES{{else}}NO{{/if}}`,
			data:     map[string]interface{}{"a": "hello", "b": "hello"},
			expected: "YES",
		},

		// default helper tests
		{
			name:     "default: value is 0 (should use value)",
			template: `{{default value 10}}`,
			data:     map[string]interface{}{"value": 0},
			expected: "0",
		},
		{
			name:     "default: value is false (should use value)",
			template: `{{default value true}}`,
			data:     map[string]interface{}{"value": false},
			expected: "false",
		},
		{
			name:     "default: value is nil (should use default)",
			template: `{{default value 10}}`,
			data:     map[string]interface{}{},
			expected: "10",
		},
		{
			name:     "default: value is empty string (should use default)",
			template: `{{default value "fallback"}}`,
			data:     map[string]interface{}{"value": ""},
			expected: "fallback",
		},
		{
			name:     "default: value is non-empty string (should use value)",
			template: `{{default value "fallback"}}`,
			data:     map[string]interface{}{"value": "hello"},
			expected: "hello",
		},

		// Combined helper tests (Aurora capacity use case)
		{
			name:     "Aurora: min_capacity is 0 (pause when idle)",
			template: `{{#if (or min_capacity (eq min_capacity 0))}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{"min_capacity": 0},
			expected: "min_capacity = 0",
		},
		{
			name:     "Aurora: min_capacity is 0.5",
			template: `{{#if (or min_capacity (eq min_capacity 0))}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{"min_capacity": 0.5},
			expected: "min_capacity = 0.5",
		},
		{
			name:     "Aurora: min_capacity is missing",
			template: `{{#if (or min_capacity (eq min_capacity 0))}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{},
			expected: "",
		},

		// New exists pattern (better than or+eq)
		{
			name:     "exists pattern: min_capacity is 0",
			template: `{{#if (exists min_capacity)}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{"min_capacity": 0},
			expected: "min_capacity = 0",
		},
		{
			name:     "exists pattern: min_capacity is 0.5",
			template: `{{#if (exists min_capacity)}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{"min_capacity": 0.5},
			expected: "min_capacity = 0.5",
		},
		{
			name:     "exists pattern: min_capacity is missing",
			template: `{{#if (exists min_capacity)}}min_capacity = {{min_capacity}}{{/if}}`,
			data:     map[string]interface{}{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := raymond.Render(tt.template, tt.data)
			if err != nil {
				t.Fatalf("Error rendering template: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q but got %q", tt.expected, result)
			}
		})
	}
}

// TestIsTruthy tests the isTruthy helper function
func TestIsTruthy(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"nil is falsy", nil, false},
		{"false is falsy", false, false},
		{"true is truthy", true, true},
		{"0 is falsy", 0, false},
		{"1 is truthy", 1, true},
		{"0.0 is falsy", 0.0, false},
		{"0.5 is truthy", 0.5, true},
		{"empty string is falsy", "", false},
		{"non-empty string is truthy", "hello", true},
		{"empty slice is falsy", []int{}, false},
		{"non-empty slice is truthy", []int{1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTruthy(tt.value)
			if result != tt.expected {
				t.Errorf("isTruthy(%v) = %v, expected %v", tt.value, result, tt.expected)
			}
		})
	}
}
