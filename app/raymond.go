package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

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

// helpersOnce guards helper registration.
//
// raymond.RegisterHelper panics on a duplicate name, so calling this twice in one
// process crashes. Both main.go and handleGenerateCommand call it; today that is
// survivable only because the generate branch exits first, which is a fragile
// thing to rely on.
var helpersOnce sync.Once

func registerCustomHelpers() {
	helpersOnce.Do(registerCustomHelpersOnce)
}

// arrayHelperErr prefixes every error the array helper raises, so that
// describeArrayHelperError can recognise one coming back out of raymond.
const arrayHelperErr = "array helper"

// jsonHelperErr does the same for the json helper.
const jsonHelperErr = "json helper"

func registerCustomHelpersOnce() {
	// array renders a YAML list as a JSON array, which is valid HCL for a
	// list(...) variable.
	//
	// An absent key is an empty list. All twenty {{array ...}} sites in
	// env/main.hbs read an optional field, so a config that simply omits one is
	// well-formed and must still generate. Panicking on nil meant every
	// hand-written or partial YAML crashed generation instead of producing
	// output.
	//
	// A wrong *type* is a genuine configuration error and still fails — but as an
	// error, not a panic. raymond's errRecover only converts a panic carrying an
	// `error` into a returned error and re-panics anything else, so a panic
	// carrying a string unwinds through raymond and prints a Go stack trace at
	// the user. That is never the right way to report a bad YAML value.
	raymond.RegisterHelper("array", func(items interface{}) raymond.SafeString {
		if items == nil {
			return "[]"
		}

		v := reflect.ValueOf(items)
		if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
			panic(fmt.Errorf("%s: expected a list, got %T (%s)", arrayHelperErr, items, formatYAMLValue(items)))
		}

		// A typed nil slice ([]string(nil)) marshals to `null`, which is not a
		// valid HCL list. It means what an absent key means: nothing was set.
		if v.Kind() == reflect.Slice && v.IsNil() {
			return "[]"
		}

		// Convert map[interface{}]interface{} to map[string]interface{} for JSON compatibility
		converted := convertToJSONCompatible(items)

		jsonBytes, err := json.Marshal(converted)
		if err != nil {
			panic(fmt.Errorf("%s: cannot render %s as a list: %w", arrayHelperErr, formatYAMLValue(items), err))
		}
		return raymond.SafeString(jsonBytes)
	})

	// json renders any value as JSON, which doubles as valid HCL: an object
	// constructor accepts `"key": value` as well as `key = value`, so a decoded
	// YAML mapping round-trips through json.Marshal into something Terraform
	// parses.
	//
	// env/main.hbs has called {{{json filter_policy}}} for SNS subscription
	// filters since that block was written, and no such helper was ever
	// registered. raymond resolves an unknown helper name as a path, finds
	// nothing and renders empty, so the call site produced `jsonencode()` —
	// syntactically valid HCL that fails at terraform validate on arity. It only
	// reached anyone who set filter_policy, which is why it survived.
	//
	// Unlike mmap this preserves nesting: a filter policy is arbitrary JSON and
	// mmap flattens every value through %v, turning a list into "[a b]".
	raymond.RegisterHelper("json", func(value interface{}) raymond.SafeString {
		// Guarded by {{#if filter_policy}} at the only call site, but a helper
		// that renders nothing for nil is how this bug looked in the first place.
		if value == nil {
			return "null"
		}

		encoded, err := json.Marshal(convertToJSONCompatible(value))
		if err != nil {
			panic(fmt.Errorf("%s: cannot render %s as JSON: %w", jsonHelperErr, formatYAMLValue(value), err))
		}
		return raymond.SafeString(encoded)
	})

	//   [{ "name" : "PG_DATABASE_HOST", "value" : var.db_endpoint }, ...]
	raymond.RegisterHelper("envToEnvArray", func(t any) raymond.SafeString {
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
		// Sorted for reproducible output — see sortedKeys.
		keys := make([]string, 0, len(result))
		for key := range result {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			tfMap.WriteString(fmt.Sprintf("    { \"name\" : \"%s\", \"value\" : \"%s\" },\n", key, result[key]))
		}
		tfMap.WriteString(" ]")

		return raymond.SafeString(tfMap.String())
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

	// isSubdomainOf - checks if a domain is a subdomain of or equals the parent domain
	// Usage: {{#if (isSubdomainOf "mail.example.com" "example.com")}}...{{/if}}
	// Returns true for:
	//   - "example.com" is subdomain of "example.com" (exact match)
	//   - "mail.example.com" is subdomain of "example.com"
	//   - "dev.mail.example.com" is subdomain of "example.com"
	// Returns false for:
	//   - "otherexample.com" is NOT subdomain of "example.com"
	//   - "mail.other.com" is NOT subdomain of "example.com"
	// gt is a numeric greater-than, for {{#if (gt (len xs) 1)}}.
	//
	// env/main.hbs has called this since ACM subject alternative names were
	// added, and it was never registered, so the subexpression rendered nothing,
	// the {{#if}} saw a falsy value, and subject_alternative_names was never
	// emitted at all. A certificate for a distribution with several aliases
	// covered only the first one, and the generated Terraform was valid, so
	// nothing reported a problem until a browser did.
	//
	// Deliberately not routed through the existing compare helper: that one takes
	// its operands as strings and compares them lexicographically, which is right
	// for the `> 0` emptiness tests it is used for and wrong the moment a length
	// reaches double digits ("10" < "9").
	raymond.RegisterHelper("gt", func(a, b interface{}) bool {
		left, leftOK := numericValue(a)
		right, rightOK := numericValue(b)
		if !leftOK || !rightOK {
			return false
		}
		return left > right
	})

	raymond.RegisterHelper("isSubdomainOf", func(domain, parentDomain interface{}) bool {
		domainStr, ok1 := domain.(string)
		parentStr, ok2 := parentDomain.(string)
		if !ok1 || !ok2 || domainStr == "" || parentStr == "" {
			return false
		}

		// Normalize: remove trailing dots
		domainStr = strings.TrimSuffix(domainStr, ".")
		parentStr = strings.TrimSuffix(parentStr, ".")

		// Exact match
		if domainStr == parentStr {
			return true
		}

		// Check if domain ends with ".parentDomain"
		return strings.HasSuffix(domainStr, "."+parentStr)
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

	raymond.RegisterHelper("mmap", func(value interface{}) raymond.SafeString {
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
			return raymond.SafeString(builder.String())
		}

		// If it's already a map, format it as Terraform map
		if m, ok := value.(map[string]interface{}); ok {
			var builder strings.Builder
			builder.WriteString("{\n")

			// Sort keys: Go map iteration order is randomised, so ranging directly
			// makes generation non-reproducible and every `generate` produces a
			// spurious diff.
			for _, k := range sortedKeys(m) {
				strValue := fmt.Sprintf("%v", m[k])

				// Handle boolean values properly
				if strValue == "true" || strValue == "false" {
					builder.WriteString(fmt.Sprintf("  %s = %s\n", k, strValue))
				} else {
					// Quote all other values
					builder.WriteString(fmt.Sprintf("  %s = \"%s\"\n", k, strValue))
				}
			}

			builder.WriteString("}")
			return raymond.SafeString(builder.String())
		}

		return "{}"
	})

	// stringListMap renders a YAML mapping of name -> list of strings as an HCL
	// map(list(string)), for inputs such as pubsub_appsync.required_claims:
	//
	//   {
	//     "role"      = ["admin", "ops"]
	//     "tenant_id" = []
	//   }
	//
	// A scalar value is promoted to a one-element list so `role: admin` renders
	// as ["admin"] rather than as invalid HCL. Keys are sorted: Go map iteration
	// order is randomised, and generating a different file each run would make
	// every `generate` produce a spurious diff.
	raymond.RegisterHelper("stringListMap", func(value interface{}) raymond.SafeString {
		entries := map[string]interface{}{}
		switch m := value.(type) {
		case nil:
			return "{}"
		case map[string]interface{}:
			entries = m
		case map[interface{}]interface{}:
			for k, v := range m {
				entries[fmt.Sprintf("%v", k)] = v
			}
		default:
			return "{}"
		}

		if len(entries) == 0 {
			return "{}"
		}

		var builder strings.Builder
		builder.WriteString("{\n")
		for _, key := range sortedKeys(entries) {
			var values []string
			switch v := entries[key].(type) {
			case nil:
			case []interface{}:
				for _, item := range v {
					values = append(values, fmt.Sprintf("%q", fmt.Sprintf("%v", item)))
				}
			case []string:
				for _, item := range v {
					values = append(values, fmt.Sprintf("%q", item))
				}
			default:
				values = append(values, fmt.Sprintf("%q", fmt.Sprintf("%v", v)))
			}
			builder.WriteString(fmt.Sprintf("    %q = [%s]\n", key, strings.Join(values, ", ")))
		}
		builder.WriteString("  }")
		return raymond.SafeString(builder.String())
	})

	raymond.RegisterHelper("envArray", func(value interface{}) raymond.SafeString {
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
			// If it's a regular map, convert to name/value format.
			// Sorted: Go randomises map iteration order, which made generation
			// non-reproducible (the REACT_APP_* variables reshuffled every run).
			for _, k := range sortedKeys(m) {
				builder.WriteString(fmt.Sprintf("    { \"name\" : \"%s\", \"value\" : \"%v\" },\n", k, m[k]))
			}
		} else if m, ok := value.(map[interface{}]interface{}); ok {
			// Handle map[interface{}]interface{} from YAML unmarshaling
			normalized := make(map[string]interface{}, len(m))
			for k, v := range m {
				normalized[fmt.Sprintf("%v", k)] = v
			}
			for _, k := range sortedKeys(normalized) {
				builder.WriteString(fmt.Sprintf("    { \"name\" : \"%v\", \"value\" : \"%v\" },\n", k, normalized[k]))
			}
		}

		builder.WriteString("  ]")
		return raymond.SafeString(builder.String())
	})

	// sortedPairs turns a map into a key-ordered slice of {key, value} entries.
	//
	// {{#each someMap}} walks a Go map, and Go randomises map iteration order, so
	// templates that iterate maps directly emit their entries in a different order
	// on every run. That is what made `meroku generate` non-reproducible.
	// Use {{#each (sortedPairs someMap)}} with {{key}} / {{value}} instead.
	raymond.RegisterHelper("sortedPairs", func(value interface{}) []map[string]interface{} {
		normalized := map[string]interface{}{}

		switch m := value.(type) {
		case map[string]interface{}:
			for k, v := range m {
				normalized[k] = v
			}
		case map[interface{}]interface{}:
			// YAML unmarshalling produces this shape.
			for k, v := range m {
				normalized[fmt.Sprintf("%v", k)] = v
			}
		default:
			return nil
		}

		pairs := make([]map[string]interface{}, 0, len(normalized))
		for _, k := range sortedKeys(normalized) {
			pairs = append(pairs, map[string]interface{}{"key": k, "value": normalized[k]})
		}
		return pairs
	})
}

// numericValue coerces a decoded YAML scalar to a float64.
//
// YAML gives back int, float64 or — for a value that was quoted — a string, and
// a helper argument may also be a template literal, which raymond passes through
// as whatever the parser produced. Reporting failure rather than guessing keeps
// a comparison against a non-number from silently reading as "less than".
func numericValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case nil:
		return 0, false
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	}

	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	}
	return 0, false
}

// formatYAMLValue renders a config value the way a user would recognise it in
// their YAML, so an error message can quote the thing they actually typed.
func formatYAMLValue(value interface{}) string {
	if value == nil {
		return "nothing"
	}
	s := fmt.Sprintf("%v", value)
	if _, isString := value.(string); isString {
		s = fmt.Sprintf("%q", value)
	}
	const max = 60
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// arrayHelperCall matches an {{array x.y}} or {{{array x.y}}} call site.
var arrayHelperCall = regexp.MustCompile(`\{\{\{?\s*array\s+([A-Za-z0-9_.@]+)`)

// describeArrayHelperError re-writes the array helper's type error so that it
// names the offending key, not just the value.
//
// raymond gives a helper no access to the expression it was called with, and
// env/main.hbs has twenty {{array ...}} sites, so the helper alone can only
// report "expected a list, got string". This fills in the rest.
//
// It runs only once a render has already failed, which is what makes it safe to
// search rather than resolve: it can add detail to a failure but can never cause
// one. Matching on leaf key name rather than the dotted path is what reaches the
// call sites inside {{#each}} blocks, whose paths are relative to the loop and
// cannot be resolved against the root config at all.
//
// A candidate must also hold the value the helper actually choked on. Plenty of
// {{array ...}} sites sit inside an {{#if}} that a scalar never gets past —
// backend_container_command: "" is one — so name-matching alone would point at
// keys that are working exactly as intended.
func describeArrayHelperError(err error, template string, envMap map[string]interface{}) error {
	if err == nil || !strings.Contains(err.Error(), arrayHelperErr) {
		return err
	}

	keys := map[string]bool{}
	for _, match := range arrayHelperCall.FindAllStringSubmatch(template, -1) {
		path := match[1]
		keys[path[strings.LastIndex(path, ".")+1:]] = true
	}

	var offenders []string
	walkConfigValues(envMap, "", func(path, key string, value interface{}) {
		if !keys[key] || value == nil {
			return
		}
		switch reflect.ValueOf(value).Kind() {
		case reflect.Slice, reflect.Array:
			return
		}
		formatted := formatYAMLValue(value)
		if !strings.Contains(err.Error(), formatted) {
			return
		}
		offenders = append(offenders, fmt.Sprintf("  %s: %s", path, formatted))
	})
	if len(offenders) == 0 {
		return err
	}
	sort.Strings(offenders)

	return fmt.Errorf("%w\n\nSet here:\n\n%s\n\nWrite the value as a YAML list instead:\n\n  %s:\n    - <value>",
		err, strings.Join(offenders, "\n"), lastPathSegment(offenders[0]))
}

// lastPathSegment turns "  cognito.dashboard_callback_urls: ..." into
// "dashboard_callback_urls", so the worked example uses the user's own key.
func lastPathSegment(offender string) string {
	path := strings.TrimSpace(offender)
	if colon := strings.Index(path, ":"); colon >= 0 {
		path = path[:colon]
	}
	return path[strings.LastIndex(path, ".")+1:]
}

// walkConfigValues visits every key in a decoded YAML tree, passing the dotted
// path to it, its own name, and its value. Sequence entries are indexed so the
// path points at exactly one place in the file.
func walkConfigValues(node interface{}, path string, visit func(path, key string, value interface{})) {
	child := func(key string) string {
		if path == "" {
			return key
		}
		return path + "." + key
	}

	switch n := node.(type) {
	case map[string]interface{}:
		for key, value := range n {
			visit(child(key), key, value)
			walkConfigValues(value, child(key), visit)
		}
	case map[interface{}]interface{}:
		// The shape gopkg.in/yaml.v2 produces for a nested mapping.
		for rawKey, value := range n {
			key := fmt.Sprintf("%v", rawKey)
			visit(child(key), key, value)
			walkConfigValues(value, child(key), visit)
		}
	case []interface{}:
		for i, item := range n {
			walkConfigValues(item, fmt.Sprintf("%s[%d]", path, i), visit)
		}
	}
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
	case map[string]interface{}:
		for key, value := range v {
			v[key] = convertToJSONCompatible(value)
		}
		return v
	case []interface{}:
		for i, item := range v {
			v[i] = convertToJSONCompatible(item)
		}
		return v
	default:
		return v
	}
}
