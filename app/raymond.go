package main

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
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

// hclArrayHelperErr is deliberately NOT a superset of arrayHelperErr: the
// capital A keeps describeArrayHelperError from claiming an hclArray failure
// and then hunting for {{array ...}} call sites that had nothing to do with it.
const hclArrayHelperErr = "hclArray helper"

// hclEscapeString escapes the two HCL2 template openers, and nothing else.
//
//	"${" -> "$${"      "%{" -> "%%{"
//
// Why this exists at all: every helper that emits into env/main.hbs writes its
// output straight into generated HCL, and json.Marshal — which is all `array`
// does — escapes <, > and & but not $ or %. In HCL2 `${` opens an
// interpolation and `%{` opens a directive, inside quoted strings and inside
// heredocs alike. So a user who types
//
//	user_data_extra: 'echo "${HOSTNAME} booted" >> /var/log/pool'
//
// into a textarea gets `Error: Unknown variable; There is no variable named
// "HOSTNAME"` out of terraform plan — and essentially every non-trivial
// user-data script contains ${. The same hole reaches instance_policies[].
// resources, where IAM documents routinely carry ${aws:PrincipalTag/...}.
// Config content becoming Terraform syntax is the bug; a failed plan is only
// the loudest symptom of it.
//
// Only the two-character sequences are doubled. Doubling every $ would corrupt
// correct scripts: HCL's escape is $${, and a $$ not followed by { is two
// literal dollar signs, so `echo $HOME` would render as `echo $$HOME`.
func hclEscapeString(s string) string {
	s = strings.ReplaceAll(s, "${", "$${")
	s = strings.ReplaceAll(s, "%{", "%%{")
	return s
}

// hclHeredocBaseMarker is the delimiter every heredoc uses when the content
// allows it, so the generated main.tf reads the way it always has.
const hclHeredocBaseMarker = "EOT"

// hclHeredocMarker picks a delimiter that the body cannot terminate.
//
// The other half of the hazard hclEscapeString exists to close. A heredoc ends
// at the first line that is nothing but the marker, so a user-data script
// containing a bare
//
//	EOT
//
// on a line of its own — a here-document of the user's own, a shell function
// that echoes a banner, a base64 blob that happens to wrap that way — closes
// the heredoc early, and every remaining line of the script is handed to the
// HCL parser as configuration. That is the same defect as an unescaped `${`:
// config content becoming syntax. Doubling openers does not help, because the
// terminator is not an opener.
//
// So the marker is derived from the content instead of fixed: EOT, then EOT_1,
// EOT_2, and so on until one is found that no line of the body equals. The loop
// always terminates — the body is finite, so it can collide with only finitely
// many markers.
//
// A line is treated as a terminator if it equals the marker after trimming
// whitespace. HCL only allows leading whitespace before the marker (and only
// for the indented `<<-` form), so this is stricter than HCL is; being stricter
// costs a suffix on the delimiter and never a mis-parse.
func hclHeredocMarker(body string) string {
	conflicts := func(marker string) bool {
		for _, line := range strings.Split(body, "\n") {
			if strings.TrimSpace(line) == marker {
				return true
			}
		}
		return false
	}

	if !conflicts(hclHeredocBaseMarker) {
		return hclHeredocBaseMarker
	}
	for i := 1; ; i++ {
		marker := fmt.Sprintf("%s_%d", hclHeredocBaseMarker, i)
		if !conflicts(marker) {
			return marker
		}
	}
}

// hclHeredocString renders a value as a COMPLETE indented heredoc — the `<<-`
// opener, the escaped body, and the closing marker, all three, so that the
// delimiter cannot desynchronise from the content it delimits. That is the
// whole reason this is one helper rather than the template writing `<<-EOT`
// itself and asking a helper for the body.
//
// The closing marker is emitted at column 0, which is what the template did
// before and what keeps `<<-`'s indent stripping a no-op.
func hclHeredocString(s string) string {
	body := hclEscapeString(s)
	marker := hclHeredocMarker(body)
	return fmt.Sprintf("<<-%s\n%s\n%s", marker, body, marker)
}

// escapeHCLTemplateOpeners applies hclEscapeString to every string in a decoded
// YAML tree — values and map keys alike, since a JSON object key lands inside a
// quoted HCL string too.
//
// It runs after convertToJSONCompatible, so map[interface{}]interface{} has
// already collapsed to map[string]interface{}; the extra cases below are for
// values that reached a helper from a Go struct rather than from YAML.
func escapeHCLTemplateOpeners(data interface{}) interface{} {
	switch v := data.(type) {
	case string:
		return hclEscapeString(v)
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, item := range v {
			out[i] = escapeHCLTemplateOpeners(item)
		}
		return out
	case []string:
		out := make([]string, len(v))
		for i, item := range v {
			out[i] = hclEscapeString(item)
		}
		return out
	case map[string]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[hclEscapeString(key)] = escapeHCLTemplateOpeners(item)
		}
		return out
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(v))
		for key, item := range v {
			out[hclEscapeString(fmt.Sprintf("%v", key))] = escapeHCLTemplateOpeners(item)
		}
		return out
	default:
		return data
	}
}

// renderHCLList is the shared body of the array and hclArray helpers.
//
// An absent key is an empty list. All twenty {{array ...}} sites in
// env/main.hbs read an optional field, so a config that simply omits one is
// well-formed and must still generate. Panicking on nil meant every
// hand-written or partial YAML crashed generation instead of producing output.
//
// A wrong *type* is a genuine configuration error and still fails — but as an
// error, not a panic. raymond's errRecover only converts a panic carrying an
// `error` into a returned error and re-panics anything else, so a panic
// carrying a string unwinds through raymond and prints a Go stack trace at the
// user. That is never the right way to report a bad YAML value.
func renderHCLList(items interface{}, label string, escape bool) raymond.SafeString {
	if items == nil {
		return "[]"
	}

	v := reflect.ValueOf(items)
	if v.Kind() != reflect.Slice && v.Kind() != reflect.Array {
		panic(fmt.Errorf("%s: expected a list, got %T (%s)", label, items, formatYAMLValue(items)))
	}

	// A typed nil slice ([]string(nil)) marshals to `null`, which is not a
	// valid HCL list. It means what an absent key means: nothing was set.
	if v.Kind() == reflect.Slice && v.IsNil() {
		return "[]"
	}

	// Convert map[interface{}]interface{} to map[string]interface{} for JSON compatibility
	converted := convertToJSONCompatible(items)
	if escape {
		converted = escapeHCLTemplateOpeners(converted)
	}

	jsonBytes, err := json.Marshal(converted)
	if err != nil {
		panic(fmt.Errorf("%s: cannot render %s as a list: %w", label, formatYAMLValue(items), err))
	}
	return raymond.SafeString(jsonBytes)
}

func registerCustomHelpersOnce() {
	// array renders a YAML list as a JSON array, which is valid HCL for a
	// list(...) variable. See renderHCLList for the nil and wrong-type rules.
	//
	// It deliberately does NOT escape HCL template openers. Changing that would
	// alter the rendering of every existing environment — {{{array services}}}
	// at env/main.hbs alone travels through every service field — and break the
	// zero-diff contract for a hazard that predates the compute pools. New call
	// sites that carry user-controlled strings use hclArray instead.
	raymond.RegisterHelper("array", func(items interface{}) raymond.SafeString {
		return renderHCLList(items, arrayHelperErr, false)
	})

	// hclArray is `array` with hclEscapeString applied to every string in the
	// tree, recursively, before json.Marshal. Use it for any list whose
	// contents came out of a YAML file a user edits: instance_policies[].
	// resources carries ${aws:PrincipalTag/...} routinely, and an unescaped one
	// is an interpolation Terraform tries to resolve.
	raymond.RegisterHelper("hclArray", func(items interface{}) raymond.SafeString {
		return renderHCLList(items, hclArrayHelperErr, true)
	})

	// hclString renders a scalar as a COMPLETE quoted HCL string literal —
	// quotes included, so the call site is `ami_id = {{{hclString ami_id}}}`
	// and not `ami_id = "{{...}}"`.
	//
	// Two hazards, and they need different treatment, which is why this is not
	// the same helper as hclEscape:
	//
	//  1. `"` and `\` must be backslash-escaped or the value simply ends the
	//     HCL string early and whatever follows is parsed as HCL. Rendering the
	//     value through a double-stache instead would have raymond HTML-escape
	//     it — no breakout, but `&quot;` written into main.tf as data.
	//  2. `${` and `%{` must be doubled, which no JSON encoder does.
	//
	// json.Marshal supplies (1) — HCL's quoted-template escapes are JSON's —
	// and hclEscapeString supplies (2), applied first so the encoder cannot
	// re-escape the doubling.
	raymond.RegisterHelper("hclString", func(value interface{}) raymond.SafeString {
		if value == nil {
			return `""`
		}
		encoded, err := json.Marshal(hclEscapeString(fmt.Sprintf("%v", value)))
		if err != nil {
			panic(fmt.Errorf("hclString helper: cannot render %s as a string: %w", formatYAMLValue(value), err))
		}
		return raymond.SafeString(encoded)
	})

	// hclEscape doubles the HCL template openers and does nothing else. It is
	// for a HEREDOC body, where hclString would be wrong twice over: the
	// surrounding quotes do not belong there, and HCL processes backslash
	// escapes only inside quoted templates, so a `\"` emitted into a heredoc
	// stays a literal backslash and corrupts the user's script.
	//
	// The call site must use the triple-stache. {{hclEscape x}} runs raymond's
	// HTML escaper over the result, turning `echo "hi"` into
	// `echo &quot;hi&quot;` inside the generated main.tf.
	raymond.RegisterHelper("hclEscape", func(value interface{}) raymond.SafeString {
		if value == nil {
			return ""
		}
		return raymond.SafeString(hclEscapeString(fmt.Sprintf("%v", value)))
	})

	// hclHeredoc is hclEscape plus the delimiter, so the call site is
	// `user_data_extra = {{{hclHeredoc user_data_extra}}}` and nothing in the
	// template names EOT. See hclHeredocString: a template that writes its own
	// `<<-EOT` fences cannot react to a body that contains one.
	//
	// The triple-stache matters here for the same reason it does for hclEscape:
	// {{hclHeredoc x}} would HTML-escape the script.
	raymond.RegisterHelper("hclHeredoc", func(value interface{}) raymond.SafeString {
		if value == nil {
			return `""`
		}
		return raymond.SafeString(hclHeredocString(fmt.Sprintf("%v", value)))
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
