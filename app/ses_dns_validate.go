package main

import (
	"fmt"
	"math"
	"regexp"
	"strings"
)

// Rules for the TXT values meroku writes for an SES domain: spf_record,
// spf_ttl, apex_txt_values and txt_records. modules/ses/variables.tf holds the
// same rules as variable validations, so a hand-written main.tf is refused
// too; these exist so `meroku generate` and the web app refuse a value with a
// plain explanation before Terraform runs. web/src/lib/sesDnsValidation.ts
// mirrors them for instant feedback while typing.

// sesDNSIssue is one refused value.
type sesDNSIssue struct {
	// Field is the path inside the ses.domains[] entry, e.g.
	// "txt_records[0].name".
	Field string `json:"field"`
	// Code is a stable identifier for the rule.
	Code string `json:"code"`
	// Message states what is wrong, naming the value.
	Message string `json:"message"`
	// Hint says, in plain words, what to do instead.
	Hint string `json:"hint"`
}

const (
	sesTXTDelimiter      = `" "`
	sesTXTMaxNameLength  = 253
	sesDefaultMailFromSD = "bounce"
)

var (
	sesSPFPrefix          = regexp.MustCompile(`^v=spf1( |$)`)
	sesSPFPrefixAnyCase   = regexp.MustCompile(`(?i)^v=spf1( |$)`)
	sesTXTPrintable       = regexp.MustCompile(`^[\x20-\x7E]*$`)
	sesTXTEscapeAtPartEnd = regexp.MustCompile(`\\[0-9]{0,2}$`)
	sesTXTRecordName      = regexp.MustCompile(`^[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?(\.[a-z0-9_]([a-z0-9_-]{0,61}[a-z0-9_])?)*$`)
)

// sesReservedTXTNames are written by meroku itself at every SES domain.
var sesReservedTXTNames = map[string]string{
	"_amazonses": "Amazon SES uses this name to check that you own the domain.",
	"_dmarc":     "meroku writes the domain's DMARC policy here; set dmarc_policy instead.",
}

// validateSESTXTValue checks one apex or named TXT value against the rules
// every such value shares. It does not check SPF content.
func validateSESTXTValue(field, value string) []sesDNSIssue {
	var issues []sesDNSIssue
	if strings.ReplaceAll(value, sesTXTDelimiter, "") == "" {
		return []sesDNSIssue{{field, "empty_value", "The value is empty.",
			"Type the text the record should hold, or remove the entry."}}
	}
	if !sesTXTPrintable.MatchString(value) {
		issues = append(issues, sesDNSIssue{field, "non_printable",
			"The value holds a character other than plain letters, digits, spaces and punctuation.",
			`Use only plain letters, digits, spaces and punctuation. Write any other character as a DNS escape such as \344.`})
	}
	if strings.Contains(strings.ReplaceAll(value, sesTXTDelimiter, ""), `"`) ||
		strings.HasPrefix(value, `"`) || strings.HasSuffix(value, `"`) {
		issues = append(issues, sesDNSIssue{field, "stray_quote",
			"The value holds a quote character.",
			`Remove the quote characters. meroku adds the quotes DNS needs. The only quotes allowed are the three characters " " that join two parts of one long value.`})
	}
	for _, part := range strings.Split(value, sesTXTDelimiter) {
		for i := 0; i+255 < len(part); i += 255 {
			if sesTXTEscapeAtPartEnd.MatchString(part[i : i+255]) {
				issues = append(issues, sesDNSIssue{field, "split_inside_escape",
					"The value is longer than 255 characters and would be cut in the middle of a backslash escape.",
					`Split it yourself: put the three characters " " at a point outside any backslash escape.`})
				break
			}
		}
	}
	return issues
}

func sesWholeNonNegative(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, n >= 0
	case int64:
		return int(n), n >= 0
	case float64:
		if n >= 0 && math.Floor(n) == n {
			return int(n), true
		}
	}
	return 0, false
}

func sesStringList(v interface{}) ([]string, bool) {
	switch list := v.(type) {
	case nil:
		return nil, true
	case []string:
		return list, true
	case []interface{}:
		out := make([]string, 0, len(list))
		for _, x := range list {
			s, ok := x.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	}
	return nil, false
}

// sesEffectiveMailFrom returns whether MAIL FROM is enabled for the domain
// and its subdomain, with the module's fallbacks.
func sesEffectiveMailFrom(ses, d map[string]interface{}) (bool, string) {
	enabled := true
	if v, ok := ses["enable_mail_from"].(bool); ok {
		enabled = v
	}
	if v, ok := d["enable_mail_from"].(bool); ok {
		enabled = v
	}
	sub := computeStringField(d, "mail_from_subdomain")
	if sub == "" {
		sub = computeStringField(ses, "mail_from_subdomain")
	}
	if sub == "" {
		sub = sesDefaultMailFromSD
	}
	return enabled, sub
}

// validateSESDomainDNS checks one ses.domains[] entry. ses is the parent ses
// block, for the MAIL FROM defaults.
func validateSESDomainDNS(ses, d map[string]interface{}) []sesDNSIssue {
	var issues []sesDNSIssue
	domain := strings.ToLower(computeStringField(d, "domain"))

	// An empty spf_record is not rendered, so it means the default.
	if raw, present := d["spf_record"]; present && raw != nil && raw != "" {
		spf := fmt.Sprintf("%v", raw)
		if !sesSPFPrefix.MatchString(spf) {
			issues = append(issues, sesDNSIssue{"spf_record", "not_spf",
				fmt.Sprintf("spf_record %q is not a sender (SPF) record.", spf),
				`A sender record starts with v=spf1 in lower case, for example: v=spf1 include:amazonses.com ~all`})
		} else {
			issues = append(issues, validateSESTXTValue("spf_record", spf)...)
		}
	}
	if raw, present := d["spf_ttl"]; present && raw != nil {
		if _, ok := sesWholeNonNegative(raw); !ok {
			issues = append(issues, sesDNSIssue{"spf_ttl", "bad_ttl",
				fmt.Sprintf("spf_ttl %v is not a whole number of seconds.", raw),
				"Use a whole number of seconds, 0 or more. 600 is ten minutes."})
		}
	}

	apex, ok := sesStringList(d["apex_txt_values"])
	if !ok {
		issues = append(issues, sesDNSIssue{"apex_txt_values", "not_a_list",
			"apex_txt_values is not a list of text values.", "Write one text value per list entry."})
	}
	for i, v := range apex {
		field := fmt.Sprintf("apex_txt_values[%d]", i)
		if sesSPFPrefixAnyCase.MatchString(strings.ReplaceAll(v, sesTXTDelimiter, "")) {
			issues = append(issues, sesDNSIssue{field, "spf_in_list",
				"This is a sender (SPF) record.",
				"Set it as spf_record instead (in the web app: Who sends email from this domain). A domain can have only one."})
			continue
		}
		issues = append(issues, validateSESTXTValue(field, v)...)
	}

	recordsRaw, present := d["txt_records"]
	if !present || recordsRaw == nil {
		return issues
	}
	records, ok := recordsRaw.([]interface{})
	if !ok {
		return append(issues, sesDNSIssue{"txt_records", "not_a_list",
			"txt_records is not a list.", "Write each extra record as a list entry with a name and values."})
	}
	mailFromOn, mailFromSub := sesEffectiveMailFrom(ses, d)
	seen := map[string]int{}
	for i, raw := range records {
		base := fmt.Sprintf("txt_records[%d]", i)
		r, ok := computeNodeMap(raw)
		if !ok {
			issues = append(issues, sesDNSIssue{base, "not_a_record",
				"This entry is not a record.", "Give each extra record a name and a list of values."})
			continue
		}
		name := computeStringField(r, "name")
		nameField := base + ".name"
		switch {
		case name == "" || name == "@":
			issues = append(issues, sesDNSIssue{nameField, "apex_name",
				"The name is empty or @, which is the domain itself.",
				"Values at the domain itself go under the other text records at the domain (apex_txt_values)."})
		case domain != "" && (name == domain || strings.HasSuffix(name, "."+domain)):
			issues = append(issues, sesDNSIssue{nameField, "absolute_name",
				fmt.Sprintf("The name %q already ends with the domain %s.", name, domain),
				fmt.Sprintf("Write only the part before .%s, for example google._domainkey.", domain)})
		case !sesTXTRecordName.MatchString(name):
			issues = append(issues, sesDNSIssue{nameField, "bad_name",
				fmt.Sprintf("The name %q is not a valid DNS name.", name),
				"Use lower-case letters, digits, hyphens and underscores, with dots between the parts, for example google._domainkey. No dot at the end."})
		default:
			if why, reserved := sesReservedTXTNames[name]; reserved {
				issues = append(issues, sesDNSIssue{nameField, "reserved_name",
					fmt.Sprintf("meroku writes the record %s.%s itself.", name, domain), why + " Pick another name."})
			} else if mailFromOn && name == mailFromSub {
				issues = append(issues, sesDNSIssue{nameField, "reserved_name",
					fmt.Sprintf("meroku writes the record %s.%s itself.", name, domain),
					"This is the bounce address Amazon SES sends from. Pick another name."})
			} else if len(name)+1+len(domain) > sesTXTMaxNameLength {
				issues = append(issues, sesDNSIssue{nameField, "name_too_long",
					fmt.Sprintf("%s.%s is longer than 253 characters.", name, domain), "Use a shorter name."})
			} else if j, dup := seen[name]; dup {
				issues = append(issues, sesDNSIssue{nameField, "duplicate_name",
					fmt.Sprintf("The name %q is also used by extra record %d.", name, j+1),
					"Put all values for one name in one entry."})
			}
			seen[name] = i
		}

		values, ok := sesStringList(r["values"])
		if !ok || len(values) == 0 {
			issues = append(issues, sesDNSIssue{base + ".values", "no_values",
				"The record has no values.", "Add the text the record should hold."})
		}
		for j, v := range values {
			field := fmt.Sprintf("%s.values[%d]", base, j)
			if sesSPFPrefixAnyCase.MatchString(strings.ReplaceAll(v, sesTXTDelimiter, "")) {
				issues = append(issues, sesDNSIssue{field, "spf_in_list",
					"This is a sender (SPF) record.",
					"meroku writes sender records only at the domain itself, through spf_record."})
				continue
			}
			issues = append(issues, validateSESTXTValue(field, v)...)
		}
		if ttl, present := r["ttl"]; present && ttl != nil {
			if _, ok := sesWholeNonNegative(ttl); !ok {
				issues = append(issues, sesDNSIssue{base + ".ttl", "bad_ttl",
					fmt.Sprintf("ttl %v is not a whole number of seconds.", ttl),
					"Use a whole number of seconds, 0 or more. 3600 is one hour."})
			}
		}
	}
	return issues
}

// validateSESDNSConfigMap refuses, at generate, every SES domain value the
// module would refuse at plan. Disabled SES is not checked: nothing renders.
func validateSESDNSConfigMap(envMap map[string]interface{}) error {
	ses, ok := computeNodeMap(envMap["ses"])
	if !ok || !isTruthy(ses["enabled"]) {
		return nil
	}
	domains, _ := ses["domains"].([]interface{})
	var problems []string
	for i, raw := range domains {
		d, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		name := computeStringField(d, "domain")
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		for _, issue := range validateSESDomainDNS(ses, d) {
			problems = append(problems, fmt.Sprintf("ses.domains %s: %s: %s %s", name, issue.Field, issue.Message, issue.Hint))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("invalid SES DNS records:\n  %s", strings.Join(problems, "\n  "))
	}
	return nil
}
