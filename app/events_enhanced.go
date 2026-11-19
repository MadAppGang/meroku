package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// The enhanced project bus (event_bus.type: enhanced).
//
//	event_bus:
//	  enabled: true
//	  type: enhanced                 # classic (the default) | enhanced
//	  share:                         # AWS RAM, enhanced only
//	    - { account: "000000000000", access: subscribe }
//	services:
//	  - name: orders
//	    events:
//	      subscribes:
//	        - { name: ledger, detail_type_prefix: "ledger.", ordered: true }
//	      queue: { fifo_retired: true }   # keeps a deployed FIFO set with nothing ordered
//
// On an enhanced env, project-bus subscriptions are delivered by EventBridge
// subscribers (modules/event_subscriber), one per unordered subscription and
// one FIFO subscriber per consumer for its ordered ones. Those resources exist
// only in the awscc provider, so the env renders awscc whenever it is enhanced
// (mode live) or its state may still hold awscc resources (mode cleanup).
//
// What was deployed is read from the Terraform state on every render that
// writes main.tf (applyTemplate), and from the `# meroku:events` marker lines
// of the previous main.tf where the state cannot answer.
//
// See ai_docs/EVENTS.md.

const (
	eventBusTypeClassic  = "classic"
	eventBusTypeEnhanced = "enhanced"
)

// awscc modes.
const (
	awsccModeOff     = "off"
	awsccModeLive    = "live"
	awsccModeCleanup = "cleanup"
)

// envMap keys written by normalizeEvents for the template.
const (
	awsccModeKey            = "_awscc_mode"
	eventBusEnhancedKey     = "_event_bus_enhanced"
	awsccRenderKey          = "_awscc_render"
	eventBusShareKey        = "_event_bus_share"
	eventBusRevokeKey       = "_event_bus_revoke"
	eventBusGuardKey        = "_event_bus_guard"
	eventSubscriberChainKey = "_event_subscriber_chain"
	eventMarkersKey         = "_event_markers"
	eventBridgeFifoKey      = "_event_bridge_fifo"
)

// envMap keys the callers of preprocessEnvMap set before it runs.
const (
	// eventBusPreviousTypeKey is written by normalizeEvents: enhanced, classic,
	// cleanup or none, the bus kind of the last deploy as far as it is known.
	eventBusPreviousTypeKey = "_event_bus_previous_type"
	// eventPreviousKey holds the parsed markers of the previous main.tf
	// (eventPrevious), when there is one.
	eventPreviousKey = "_event_previous"
	// eventsAwsccStateKey holds the state probe (eventsAwsccState). Only
	// applyTemplate sets it; its absence means the offline warnings path.
	eventsAwsccStateKey = "_events_awscc_state"
	// eventBusSiblingAccountsKey holds the share targets ([]shareTarget) of
	// this env's directory, for share entries that name an env.
	eventBusSiblingAccountsKey = "_event_bus_sibling_accounts"
	// eventBusSiblingScanErrorKey holds the error text of a failed sibling scan.
	eventBusSiblingScanErrorKey = "_event_bus_sibling_scan_error"
)

// eventBusTypeFlipNeedsTwoDeploys turns on the refusal of a classic/enhanced
// switch in one deploy. It is needed only if AWS cannot hold a classic and an
// enhanced bus of the same name at once; in ap-southeast-2 both existed side
// by side, so the switch is a replacement in one deploy.
const eventBusTypeFlipNeedsTwoDeploys = false

// eventFifoDeliveryDLQIsFifo says whether a FIFO subscriber's delivery DLQ is
// a .fifo queue. A FIFO subscriber dead-lettered into a .fifo queue in
// ap-southeast-2. Must equal the fifo_delivery_dlq request of
// modules/events/naming.tf.
const eventFifoDeliveryDLQIsFifo = true

// eventGroupsCarryPublisherAccount says whether a FIFO subscriber's
// MessageGroupId includes the publishing account. When false, two accounts
// that publish the same EventGroupId share one SQS group, and generate warns
// an ordered consumer of a bus shared with publishers.
const eventGroupsCarryPublisherAccount = true

// enhancedEventBusRegions are the Regions where EventBridge offers the enhanced
// custom event bus. Source:
// https://docs.aws.amazon.com/eventbridge/latest/userguide/feature-availability.html
// Add a Region here when that page lists it; web/src/utils/workloadEvents.ts
// mirrors this list.
var enhancedEventBusRegions = []string{
	"ap-east-1", "ap-northeast-1", "ap-south-1", "ap-southeast-1", "ap-southeast-2",
	"ap-southeast-5", "ap-southeast-7", "eu-central-1", "eu-north-1", "eu-south-2",
	"eu-west-1", "us-east-1", "us-east-2", "us-west-2",
}

// Pattern limits of an enhanced-bus subscriber: the JSON of all its filters
// together (enhancedPatternBytes), and the $or combinations of its pattern.
const (
	enhancedPatternMaxBytes        = 4096
	enhancedPatternMaxCombinations = 1000
)

// enhancedPutEventsOnlyPattern is the SYSTEM_METADATA filter every meroku
// subscriber carries, so a PutRawEvents payload never reaches a queue. It
// counts against enhancedPatternMaxBytes. Must equal
// modules/event_subscriber's put_events_only_pattern.
const enhancedPutEventsOnlyPattern = `{"ContentType":["application/eventbridge+json"]}`

// Subscriber module name prefixes in the rendered main.tf.
const (
	eventSubscriberModulePrefix     = "event_subscriber_"
	eventFifoSubscriberModulePrefix = "event_fifo_subscriber_"
)

// eventSubscriberConsumerPattern is a consumer name a subscriber module and
// name can carry (E21).
var eventSubscriberConsumerPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var (
	eventShareOUPattern      = regexp.MustCompile(`^ou-[a-z0-9]{4,32}-[a-z0-9]{8,32}$`)
	eventShareAccountPattern = regexp.MustCompile(`^[0-9]{12}$`)
)

// Share access levels.
const (
	eventShareAccessPublish   = "publish"
	eventShareAccessSubscribe = "subscribe"
	eventShareAccessFull      = "full"
)

// Share principal kinds.
const (
	eventShareKindOrganization = "organization"
	eventShareKindOU           = "ou"
	eventShareKindAccount      = "account"
)

// eventBusSettings is the event_bus block as normalizeEvents reads it.
type eventBusSettings struct {
	Enabled  bool
	Enhanced bool
}

// readEventBus reads event_bus.enabled and event_bus.type. A malformed
// block or enabled value is an error; the type rules (E1-E3) are problems,
// reported with every other events problem.
func readEventBus(envMap map[string]interface{}) (eventBusSettings, []string, error) {
	enabled, err := eventBusEnabled(envMap)
	if err != nil {
		return eventBusSettings{}, nil, err
	}
	settings := eventBusSettings{Enabled: enabled}
	block, _ := yamlSubMap(envMap, "event_bus")
	raw, present := block["type"]
	if !present || raw == nil {
		return settings, nil, nil
	}
	t, _ := raw.(string)
	switch t {
	case eventBusTypeClassic:
		return settings, nil, nil
	case eventBusTypeEnhanced:
	default:
		return settings, []string{fmt.Sprintf("event_bus.type must be classic or enhanced, got %s", formatYAMLValue(raw))}, nil
	}
	var problems []string
	if !enabled {
		problems = append(problems, "event_bus.type is enhanced, but event_bus.enabled is not true; the enhanced bus is the project bus, so set enabled: true or remove type")
	}
	region := stringFromMap(envMap, "region")
	if !enhancedEventBusRegion(region) {
		problems = append(problems, fmt.Sprintf(
			"event_bus.type enhanced is not available in %s; it is offered in %s. Use type: classic, or deploy this environment to one of those regions",
			region, strings.Join(enhancedEventBusRegions, ", ")))
	}
	settings.Enhanced = enabled
	return settings, problems, nil
}

// eventBusType is the bus kind the YAML asks for: enhanced or classic.
func eventBusType(bus eventBusSettings) string {
	if bus.Enhanced {
		return eventBusTypeEnhanced
	}
	return eventBusTypeClassic
}

func enhancedEventBusRegion(region string) bool {
	for _, r := range enhancedEventBusRegions {
		if r == region {
			return true
		}
	}
	return false
}

// eventShareEntry is one resolved share entry: an env entry is already an
// account.
type eventShareEntry struct {
	Kind   string // organization, ou or account
	ID     string // the OU id or account id; "" for the organization
	Access string
}

// principal is the entry's revoke-input form: account:<id>, ou:<id> or
// organization.
func (e eventShareEntry) principal() string {
	if e.Kind == eventShareKindOrganization {
		return eventShareKindOrganization
	}
	return e.Kind + ":" + e.ID
}

// subscribeCapable reports whether the access lets the other account create
// subscribers.
func subscribeCapable(access string) bool {
	return access == eventShareAccessSubscribe || access == eventShareAccessFull
}

// marker is the entry's share= marker value.
func (e eventShareEntry) marker() string {
	id := e.ID
	if id == "" {
		id = "-"
	}
	return e.Kind + ":" + id + ":" + e.Access
}

// normalizeEventBusShare validates event_bus.share (E4-E13) and resolves env
// entries against the share targets the caller recorded. The result is sorted
// by access, then id.
func normalizeEventBusShare(envMap map[string]interface{}, bus eventBusSettings) ([]eventShareEntry, []string) {
	block, _ := yamlSubMap(envMap, "event_bus")
	raw, present := block["share"]
	if !present || raw == nil {
		return nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, []string{"event_bus.share must be a list of { organization, ou, account or env, access }"}
	}
	if len(items) == 0 {
		return nil, nil
	}
	if !bus.Enhanced {
		return nil, []string{"event_bus.share needs event_bus.type: enhanced; a classic bus cannot be shared"}
	}

	project := stringFromMap(envMap, "project")
	selfEnv := stringFromMap(envMap, "env")
	ownAccount := stringFromMap(envMap, "account_id")
	targets, _ := envMap[eventBusSiblingAccountsKey].([]shareTarget)
	scanErr, _ := envMap[eventBusSiblingScanErrorKey].(string)

	var problems []string
	var out []eventShareEntry
	seen := map[string]int{}
	for i, itemRaw := range items {
		at := fmt.Sprintf("event_bus.share[%d]", i)
		item, ok := computeNodeMap(itemRaw)
		set := 0
		if ok {
			for _, k := range []string{"organization", "ou", "account", "env"} {
				if v, has := item[k]; has && v != nil {
					set++
				}
			}
		}
		if !ok || set != 1 {
			problems = append(problems, fmt.Sprintf("%s needs exactly one of organization, ou, account or env", at))
			continue
		}
		accessRaw := item["access"]
		access, _ := accessRaw.(string)
		accessOK := access == eventShareAccessPublish || access == eventShareAccessSubscribe || access == eventShareAccessFull
		if !accessOK {
			// Reported, and the principal is still checked so both show.
			problems = append(problems, fmt.Sprintf("%s.access must be publish, subscribe or full, got %s", at, formatYAMLValue(accessRaw)))
		}

		var entry eventShareEntry
		switch {
		case item["organization"] != nil:
			if b, _ := item["organization"].(bool); !b {
				problems = append(problems, fmt.Sprintf("%s.organization must be true", at))
				continue
			}
			entry = eventShareEntry{Kind: eventShareKindOrganization, Access: access}
		case item["ou"] != nil:
			ou, _ := item["ou"].(string)
			if !eventShareOUPattern.MatchString(ou) {
				problems = append(problems, fmt.Sprintf("%s.ou %s: an organizational unit id looks like ou-ab12-cd34ef56", at, formatYAMLValue(item["ou"])))
				continue
			}
			entry = eventShareEntry{Kind: eventShareKindOU, ID: ou, Access: access}
		case item["account"] != nil:
			account, isString := item["account"].(string)
			if !isString || !eventShareAccountPattern.MatchString(account) {
				problems = append(problems, fmt.Sprintf("%s.account %s: an AWS account id is 12 digits, written in quotes", at, formatYAMLValue(item["account"])))
				continue
			}
			if account == ownAccount {
				problems = append(problems, fmt.Sprintf("%s names this environment's own account; the bus needs no share there", at))
				continue
			}
			entry = eventShareEntry{Kind: eventShareKindAccount, ID: account, Access: access}
		default:
			name, _ := item["env"].(string)
			account, problem := resolveShareEnv(at, name, item["env"], project, selfEnv, targets, scanErr)
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			entry = eventShareEntry{Kind: eventShareKindAccount, ID: account, Access: access}
		}

		if !accessOK {
			continue
		}
		id := entry.principal()
		if first, dup := seen[id]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s shares with %s again, as share[%d] does; give it one entry (access: full covers both)", at, eventSharePrincipalLabel(id), first))
			continue
		}
		seen[id] = i
		out = append(out, entry)
	}
	if len(problems) > 0 {
		return nil, problems
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Access != out[j].Access {
			return out[i].Access < out[j].Access
		}
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Kind < out[j].Kind
	})
	return out, nil
}

// resolveShareEnv turns a share entry's env into the sibling's account
// (E10, E11, E11a, E11b).
func resolveShareEnv(at, name string, raw interface{}, project, selfEnv string, targets []shareTarget, scanErr string) (string, string) {
	shown := formatYAMLValue(raw)
	if scanErr != "" {
		return "", fmt.Sprintf("%s.env %s: the other environment files could not be read (%s), so the account cannot be resolved", at, shown, scanErr)
	}
	noSibling := fmt.Sprintf("%s.env %s: no other environment of project %s has that name and an account_id", at, shown, project)
	if name == "" || name == selfEnv {
		return "", noSibling
	}
	for _, t := range targets {
		if t.Env != name {
			continue
		}
		switch t.Status {
		case shareTargetAmbiguous:
			return "", fmt.Sprintf("%s.env %s: %s both declare environment %s of project %s; keep one",
				at, shown, strings.Join(t.Files, " and "), name, project)
		case shareTargetNoAccount:
			return "", noSibling
		case shareTargetOwnAccount:
			return "", fmt.Sprintf("%s names this environment's own account; the bus needs no share there", at)
		}
		return t.AccountID, ""
	}
	return "", noSibling
}

// eventSharePrincipalLabel names a principal in a message.
func eventSharePrincipalLabel(principal string) string {
	kind, id, _ := strings.Cut(principal, ":")
	switch kind {
	case eventShareKindOrganization:
		return "the organization"
	case eventShareKindOU:
		return "OU " + id
	}
	return "account " + id
}

// Share target statuses (GET /api/event-bus/share-targets and E10/E11/E11a).
const (
	shareTargetOK         = "ok"
	shareTargetAmbiguous  = "ambiguous"
	shareTargetNoAccount  = "no_account"
	shareTargetOwnAccount = "own_account"
)

// shareTarget is one sibling environment of the same project, as a share
// entry's env would resolve it.
type shareTarget struct {
	Env       string   `json:"env"`
	Status    string   `json:"status"`
	AccountID string   `json:"accountId,omitempty"`
	Region    string   `json:"region,omitempty"`
	Files     []string `json:"files"`
}

// eventBusShareTargets lists every sibling environment of project in dir,
// grouped by the env each file declares, then classified: two or more files
// ambiguous, no account_id no_account, this env's account own_account,
// otherwise ok. self is this env's file stem; the env it declares is skipped
// too. Sorted by env. Generate (E10, E11, E11a) and the share-targets endpoint
// both use it.
func eventBusShareTargets(dir, self, project, ownAccount string) ([]shareTarget, error) {
	probes, err := scanSiblingProbes(dir, self)
	if err != nil {
		return nil, err
	}
	return classifyShareTargets(probes, self, project, ownAccount), nil
}

func classifyShareTargets(probes []siblingProbe, self, project, ownAccount string) []shareTarget {
	byEnv := map[string][]siblingProbe{}
	for _, p := range probes {
		if p.Probe.Project != project || p.Probe.Env == self {
			continue
		}
		byEnv[p.Probe.Env] = append(byEnv[p.Probe.Env], p)
	}
	out := make([]shareTarget, 0, len(byEnv))
	for env, group := range byEnv {
		t := shareTarget{Env: env}
		for _, p := range group {
			t.Files = append(t.Files, p.File)
		}
		sort.Strings(t.Files)
		switch {
		case len(group) > 1:
			t.Status = shareTargetAmbiguous
		case group[0].Probe.AccountID == "":
			t.Status = shareTargetNoAccount
		case group[0].Probe.AccountID == ownAccount:
			t.Status = shareTargetOwnAccount
		default:
			t.Status = shareTargetOK
			t.AccountID = group[0].Probe.AccountID
			t.Region = group[0].Probe.Region
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Env < out[j].Env })
	return out
}

// --- Patterns -------------------------------------------------------------

// enhancedDetailTypeMatcher is the detail-type value of an enhanced DATA
// pattern: exact, prefix, or one leading/trailing * mapped to suffix/prefix.
func enhancedDetailTypeMatcher(detailType, prefix string) []interface{} {
	switch {
	case detailType == "":
		return []interface{}{map[string]interface{}{"prefix": prefix}}
	case !strings.Contains(detailType, "*"):
		return []interface{}{detailType}
	case strings.HasSuffix(detailType, "*"):
		return []interface{}{map[string]interface{}{"prefix": strings.TrimSuffix(detailType, "*")}}
	}
	return []interface{}{map[string]interface{}{"suffix": strings.TrimPrefix(detailType, "*")}}
}

// enhancedWildcardMappable reports whether an enhanced-bus detail_type with a
// * can be matched: exactly one *, at the start or the end, beside at least
// one other character.
func enhancedWildcardMappable(detailType string) bool {
	if strings.Count(detailType, "*") != 1 || len(detailType) < 2 {
		return false
	}
	return strings.HasPrefix(detailType, "*") || strings.HasSuffix(detailType, "*")
}

// enhancedSubscriptionBranch is one subscription's DATA pattern as a value
// for json.Marshal: detail-type, the optional source and the detail filters.
// An enhanced bus has no archive, so there is no replay-name.
func enhancedSubscriptionBranch(sub map[string]interface{}) map[string]interface{} {
	branch := map[string]interface{}{
		"detail-type": enhancedDetailTypeMatcher(stringFromMap(sub, "detail_type"), stringFromMap(sub, "detail_type_prefix")),
	}
	if source := stringFromMap(sub, "source"); source != "" {
		branch["source"] = []interface{}{source}
	}
	filters, _ := sub["filters"].([]interface{})
	if len(filters) > 0 {
		detail := map[string]interface{}{}
		for _, raw := range filters {
			f, _ := raw.(map[string]interface{})
			path := strings.Split(strings.TrimPrefix(stringFromMap(f, "field"), "detail."), ".")
			node := detail
			for _, seg := range path[:len(path)-1] {
				next, ok := node[seg].(map[string]interface{})
				if !ok {
					next = map[string]interface{}{}
					node[seg] = next
				}
				node = next
			}
			node[path[len(path)-1]] = f["values"]
		}
		branch["detail"] = detail
	}
	return branch
}

// enhancedSubscriptionPattern is the DATA pattern of one standard
// subscriber, the same text modules/events' jsonencode produces: sorted keys,
// compact, <, > and & escaped.
func enhancedSubscriptionPattern(sub map[string]interface{}) string {
	data, _ := json.Marshal(enhancedSubscriptionBranch(sub))
	return string(data)
}

// enhancedFifoPattern is the DATA pattern of a consumer's FIFO subscriber: the
// one branch, or $or of every ordered subscription's branch sorted by key.
func enhancedFifoPattern(ordered []map[string]interface{}) string {
	sorted := append([]map[string]interface{}(nil), ordered...)
	sort.Slice(sorted, func(i, j int) bool { return stringFromMap(sorted[i], "key") < stringFromMap(sorted[j], "key") })
	if len(sorted) == 1 {
		return enhancedSubscriptionPattern(sorted[0])
	}
	branches := make([]interface{}, 0, len(sorted))
	for _, s := range sorted {
		branches = append(branches, enhancedSubscriptionBranch(s))
	}
	data, _ := json.Marshal(map[string]interface{}{"$or": branches})
	return string(data)
}

// enhancedFilter is one filter of a subscriber's FilterConfiguration, in the
// shape AWS measures. The field order is the sorted order jsonencode writes.
type enhancedFilter struct {
	Pattern string `json:"Pattern"`
	Scope   string `json:"Scope"`
}

// enhancedPatternBytes is the size AWS gives a subscriber's filters with this
// DATA pattern: the JSON text of the whole FilterConfiguration, each pattern
// escaped as a JSON string inside it, beside the SYSTEM_METADATA filter. It
// mirrors modules/event_subscriber's filter_configuration_size: json.Marshal
// escapes <, > and & as jsonencode does.
func enhancedPatternBytes(pattern string) int {
	data, _ := json.Marshal(struct {
		Filters []enhancedFilter `json:"Filters"`
	}{Filters: []enhancedFilter{
		{Pattern: pattern, Scope: "DATA"},
		{Pattern: enhancedPutEventsOnlyPattern, Scope: "SYSTEM_METADATA"},
	}})
	return len(data)
}

// eventPatternOrCombinations is AWS's count of a pattern's $or combinations:
// the product, over every $or array at any depth, of its argument count. A
// pattern without $or counts 1; value lists do not count. A pattern that is
// not JSON counts 1.
func eventPatternOrCombinations(pattern string) int {
	var v interface{}
	if err := json.Unmarshal([]byte(pattern), &v); err != nil {
		return 1
	}
	var walk func(interface{}) int
	walk = func(node interface{}) int {
		n := 1
		switch t := node.(type) {
		case map[string]interface{}:
			for k, child := range t {
				if list, ok := child.([]interface{}); ok && k == "$or" {
					n *= len(list)
				}
				n *= walk(child)
			}
		case []interface{}:
			for _, child := range t {
				n *= walk(child)
			}
		}
		return n
	}
	return walk(v)
}

// --- Names ----------------------------------------------------------------

// eventSubscriberModule is the main.tf module of a consumer's standard
// subscriber for key k.
func eventSubscriberModule(consumer, key string) string {
	return eventSubscriberModulePrefix + consumer + "_" + key
}

// eventFifoSubscriberModule is the main.tf module of a consumer's FIFO
// subscriber.
func eventFifoSubscriberModule(consumer string) string {
	return eventFifoSubscriberModulePrefix + consumer
}

// parseEventSubscriberModule splits a subscriber module name back into its
// consumer and key; key is "" for a FIFO subscriber. Neither a consumer name
// nor a key contains "_".
func parseEventSubscriberModule(module string) (consumer, key string, fifo, ok bool) {
	if c, found := strings.CutPrefix(module, eventFifoSubscriberModulePrefix); found {
		return c, "", true, c != ""
	}
	rest, found := strings.CutPrefix(module, eventSubscriberModulePrefix)
	if !found {
		return "", "", false, false
	}
	c, k, found := strings.Cut(rest, "_")
	return c, k, false, found && c != "" && k != ""
}

// eventSubscriberName is the AWS name modules/events gives a standard
// subscriber (module.naming_subscribers, request subscriber/<c>/<k>).
func eventSubscriberName(project, env, consumer, key string) string {
	return awsName(project, env, []string{"events", consumer + "." + key}, awsNameLimits["iam_role"], "-", "", "")
}

// eventFifoSubscriberName is the AWS name of a consumer's FIFO subscriber
// (request fifo_subscriber/<c>).
func eventFifoSubscriberName(project, env, consumer string) string {
	return awsName(project, env, []string{"events", consumer, "fifo"}, awsNameLimits["iam_role"], "-", "", "")
}

// eventDeliveryRoleName is the delivery role of a consumer with a subscriber
// (module.naming_roles, request delivery_role/<c>).
func eventDeliveryRoleName(project, env, consumer string) string {
	return awsName(project, env, []string{"events", consumer, "delivery"}, awsNameLimits["iam_role"], "_", "", "")
}

// eventModuleSubscriberName is the AWS name the subscriber of a module gets.
func eventModuleSubscriberName(project, env, module string) (string, bool) {
	c, k, fifo, ok := parseEventSubscriberModule(module)
	if !ok {
		return "", false
	}
	if fifo {
		return eventFifoSubscriberName(project, env, c), true
	}
	return eventSubscriberName(project, env, c, k), true
}

// eventFifoQueueName is a consumer's FIFO main queue.
func eventFifoQueueName(project, env, consumer string) string {
	return awsName(project, env, []string{"events", consumer}, awsNameLimits["sqs_queue"], "-", ".fifo", "")
}

// --- Markers --------------------------------------------------------------

// eventMarkerPrefix starts every marker line of a rendered main.tf.
const eventMarkerPrefix = "# meroku:events "

// eventMarkerPattern parses one marker line.
var eventMarkerPattern = regexp.MustCompile(`(?m)^# meroku:events (\w+)=(\S+)( retired)?$`)

// eventPrevious is what the markers of the previous main.tf say about the
// last render.
type eventPrevious struct {
	Mode        string          // the awscc= value; "" when absent
	Subscribers []string        // live subscriber modules
	Tombstones  []string        // tombstone modules
	Ordered     []string        // "<c>/<k>"
	Fifo        map[string]bool // consumer -> retired
	Share       []string        // "<kind>:<id|->:<access>"
}

// parseEventMarkers reads the marker lines of a rendered main.tf.
func parseEventMarkers(content []byte) eventPrevious {
	prev := eventPrevious{Fifo: map[string]bool{}}
	for _, m := range eventMarkerPattern.FindAllSubmatch(content, -1) {
		value := string(m[2])
		switch string(m[1]) {
		case "awscc":
			prev.Mode = value
		case "subscriber":
			prev.Subscribers = append(prev.Subscribers, value)
		case "tombstone":
			prev.Tombstones = append(prev.Tombstones, value)
		case "ordered":
			prev.Ordered = append(prev.Ordered, value)
		case "fifo":
			prev.Fifo[value] = len(m[3]) > 0
		case "share":
			prev.Share = append(prev.Share, value)
		}
	}
	return prev
}

// previousEventMarkers parses env/<env>/main.tf as last generated. ok is
// false when there is no main.tf.
func previousEventMarkers(env string) (eventPrevious, bool) {
	content, err := os.ReadFile(filepath.Join("env", env, "main.tf"))
	if err != nil {
		return eventPrevious{}, false
	}
	return parseEventMarkers(content), true
}

// subscribeCapableFromMarkers returns the subscribe-capable principals of
// share= markers.
func subscribeCapableFromMarkers(share []string) []string {
	var out []string
	for _, s := range share {
		parts := strings.Split(s, ":")
		if len(parts) != 3 || !subscribeCapable(parts[2]) {
			continue
		}
		if parts[0] == eventShareKindOrganization {
			out = append(out, eventShareKindOrganization)
		} else {
			out = append(out, parts[0]+":"+parts[1])
		}
	}
	return out
}

// --- State probe ----------------------------------------------------------

// eventsAwsccState is what the env's Terraform state holds of the events
// resources. Nothing else of the state is kept.
type eventsAwsccState struct {
	Readable      bool                // the state could be read (an empty or missing state is readable)
	Pulled        bool                // `terraform state pull` ran and its output was parsed
	LockFile      bool                // the env dir has a .terraform.lock.hcl
	Err           string              // why the state could not be read
	Bus           bool                // module.event_bus_enhanced.awscc_eventsv2_event_bus.project[0]
	ClassicBus    bool                // module.events.aws_cloudwatch_event_bus.project[0]
	Revoke        *eventsRevokeRecord // module.event_bus_enhanced.terraform_data.revoke[0]; nil if absent
	Guard         bool                // module.event_bus_enhanced.terraform_data.bus_guard[0]
	RamPrincipals []string            // keys of aws_ram_principal_association.bus: "<access>/<kind>/<id>"
	Subscribers   map[string]string   // module name -> attributes.name of its awscc_eventsv2_subscriber.this[0]
	FifoQueues    []string            // consumers with module.events.aws_sqs_queue.fifo_main["<c>"]
	Consumers     []string            // consumers with module.events.aws_sqs_queue.main["<c>"]
}

// eventsRevokeRecord is the applied revoke step.
type eventsRevokeRecord struct {
	Granted []string // input.granted: the subscribe-capable principals it was applied with
	Lost    []string // input.lost: what it had to revoke
	Tainted bool     // its script failed
}

// eventsStatePullTimeout bounds the state read.
const eventsStatePullTimeout = 2 * time.Minute

// awsccLockProvider is the lock-file entry of the awscc provider.
const awsccLockProvider = `"registry.terraform.io/hashicorp/awscc"`

// eventsStatePuller runs `terraform state pull` in dir. A variable so tests
// stub Terraform.
var eventsStatePuller = func(ctx context.Context, dir, profile, region string) ([]byte, error) {
	cmd, err := eventsStatePullCmd(ctx, dir, profile, region)
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if tail := strings.TrimSpace(lastLines(stderr.String(), 8)); tail != "" {
			return nil, fmt.Errorf("terraform state pull: %w\n%s", err, redactSecrets(tail))
		}
		return nil, fmt.Errorf("terraform state pull: %w", err)
	}
	return out, nil
}

// eventsStatePullCmd builds the state read with the env YAML's profile and
// region set explicitly: `meroku generate` runs in a shell whose AWS_PROFILE
// may name another account, and the S3 backend reads with it.
func eventsStatePullCmd(ctx context.Context, dir, profile, region string) (*exec.Cmd, error) {
	cmd, err := newTerraformCmd(ctx, tfState, dir, "state", "pull")
	if err != nil {
		return nil, err
	}
	if profile != "" {
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+profile)
	}
	if region != "" {
		cmd.Env = append(cmd.Env, "AWS_REGION="+region)
	}
	return cmd, nil
}

// eventsAwsccStateFor reads envDir's state. The lock file is the cheap
// pre-check: no lock means nothing was initialised, so nothing is readable
// and nothing is deployed; a lock without awscc means no awscc resource and no
// FIFO set can be in state (a FIFO set needs an enhanced bus first), so
// Terraform is not run. A variable so tests stub it.
var eventsAwsccStateFor = func(envDir string, e Env) eventsAwsccState {
	lock, err := os.ReadFile(filepath.Join(envDir, ".terraform.lock.hcl"))
	if err != nil {
		return eventsAwsccState{}
	}
	if !bytes.Contains(lock, []byte(awsccLockProvider)) {
		return eventsAwsccState{Readable: true, LockFile: true}
	}
	ctx, cancel := context.WithTimeout(context.Background(), eventsStatePullTimeout)
	defer cancel()
	out, err := eventsStatePuller(ctx, envDir, e.AWSProfile, e.Region)
	if isNoStateFileError(err) {
		return eventsAwsccState{Readable: true, LockFile: true, Pulled: true}
	}
	if err != nil {
		first, _, _ := strings.Cut(err.Error(), "\n")
		return eventsAwsccState{LockFile: true, Err: first}
	}
	st, err := parseEventsAwsccState(out)
	if err != nil {
		return eventsAwsccState{LockFile: true, Err: err.Error()}
	}
	st.LockFile = true
	return st
}

// tfStateDocument is the part of a version-4 state document the probe reads.
type tfStateDocument struct {
	Version   int `json:"version"`
	Resources []struct {
		Module    string `json:"module"`
		Mode      string `json:"mode"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		Instances []struct {
			IndexKey   interface{}            `json:"index_key"`
			Status     string                 `json:"status"`
			Attributes map[string]interface{} `json:"attributes"`
		} `json:"instances"`
	} `json:"resources"`
}

// parseEventsAwsccState reads the events addresses from `terraform state
// pull` output. Empty output is an empty state.
func parseEventsAwsccState(out []byte) (eventsAwsccState, error) {
	st := eventsAwsccState{Readable: true, Pulled: true, Subscribers: map[string]string{}}
	if len(bytes.TrimSpace(out)) == 0 {
		return st, nil
	}
	var doc tfStateDocument
	if err := json.Unmarshal(out, &doc); err != nil {
		return eventsAwsccState{}, fmt.Errorf("the state is not a Terraform state document: %w", err)
	}
	if doc.Version != 4 {
		return eventsAwsccState{}, fmt.Errorf("state format version %d is not 4", doc.Version)
	}
	for _, r := range doc.Resources {
		if r.Mode != "managed" || len(r.Instances) == 0 {
			continue
		}
		switch {
		case r.Module == "module.event_bus_enhanced" && r.Type == "awscc_eventsv2_event_bus" && r.Name == "project":
			st.Bus = true
		case r.Module == "module.events" && r.Type == "aws_cloudwatch_event_bus" && r.Name == "project":
			st.ClassicBus = true
		case r.Module == "module.event_bus_enhanced" && r.Type == "terraform_data" && r.Name == "revoke":
			inst := r.Instances[0]
			input := terraformDataInput(inst.Attributes["input"])
			st.Revoke = &eventsRevokeRecord{
				Granted: stringsOf(input["granted"]),
				Lost:    stringsOf(input["lost"]),
				Tainted: inst.Status == "tainted",
			}
		case r.Module == "module.event_bus_enhanced" && r.Type == "terraform_data" && r.Name == "bus_guard":
			st.Guard = true
		case r.Module == "module.event_bus_enhanced" && r.Type == "aws_ram_principal_association" && r.Name == "bus":
			for _, inst := range r.Instances {
				if k, ok := inst.IndexKey.(string); ok {
					st.RamPrincipals = append(st.RamPrincipals, k)
				}
			}
		case r.Type == "awscc_eventsv2_subscriber" && r.Name == "this":
			module, ok := strings.CutPrefix(r.Module, "module.")
			if !ok || strings.Contains(module, ".") {
				continue
			}
			name, _ := r.Instances[0].Attributes["name"].(string)
			st.Subscribers[module] = name
		case r.Module == "module.events" && r.Type == "aws_sqs_queue" && (r.Name == "fifo_main" || r.Name == "main"):
			for _, inst := range r.Instances {
				k, ok := inst.IndexKey.(string)
				if !ok {
					continue
				}
				if r.Name == "fifo_main" {
					st.FifoQueues = append(st.FifoQueues, k)
				} else {
					st.Consumers = append(st.Consumers, k)
				}
			}
		}
	}
	sort.Strings(st.RamPrincipals)
	sort.Strings(st.FifoQueues)
	sort.Strings(st.Consumers)
	return st, nil
}

// terraformDataInput unwraps terraform_data's input attribute, which the
// state stores as a dynamic value {value, type}.
func terraformDataInput(raw interface{}) map[string]interface{} {
	m, _ := raw.(map[string]interface{})
	if v, ok := m["value"].(map[string]interface{}); ok {
		if _, typed := m["type"]; typed {
			return v
		}
	}
	return m
}

func stringsOf(raw interface{}) []string {
	items, _ := raw.([]interface{})
	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// subscribeCapableFromRAM returns the subscribe-capable principals of
// aws_ram_principal_association keys "<access>/<kind>/<id>".
func subscribeCapableFromRAM(keys []string) []string {
	var out []string
	for _, k := range keys {
		parts := strings.SplitN(k, "/", 3)
		if len(parts) != 3 || !subscribeCapable(parts[0]) {
			continue
		}
		if parts[1] == eventShareKindOrganization {
			out = append(out, eventShareKindOrganization)
		} else {
			out = append(out, parts[1]+":"+parts[2])
		}
	}
	return out
}

// --- Inputs ---------------------------------------------------------------

// prepareEventInputs records, before preprocessEnvMap, what normalizeEvents
// needs from outside the YAML: the bus switch and markers of the previous
// main.tf, and the same-project siblings in the env file's directory. Both
// the render and the warnings collector call it.
func prepareEventInputs(envMap map[string]interface{}, env, envFile string) {
	if previous, ok := previousEventBusState(env); ok {
		envMap[eventBusPreviousKey] = previous
	}
	if markers, ok := previousEventMarkers(env); ok {
		envMap[eventPreviousKey] = markers
	}
	targets, err := eventBusShareTargets(filepath.Dir(envFile), env,
		stringFromMap(envMap, "project"), stringFromMap(envMap, "account_id"))
	if err != nil {
		envMap[eventBusSiblingScanErrorKey] = err.Error()
		return
	}
	envMap[eventBusSiblingAccountsKey] = targets
}

// prepareEventStateProbe reads the env's Terraform state for normalizeEvents.
// Only a render that writes main.tf calls it.
func prepareEventStateProbe(envMap map[string]interface{}, env string) {
	e := Env{
		Project:    stringFromMap(envMap, "project"),
		Env:        stringFromMap(envMap, "env"),
		Region:     stringFromMap(envMap, "region"),
		AWSProfile: stringFromMap(envMap, "aws_profile"),
	}
	envMap[eventsAwsccStateKey] = eventsAwsccStateFor(filepath.Join("env", env), e)
}

// eventDeployed is what is known of the last deploy: state where a render
// that writes main.tf could read it, else the previous main.tf's markers.
type eventDeployed struct {
	hasPrevious bool
	previous    eventPrevious
	renderPath  bool
	state       eventsAwsccState
}

func readEventDeployed(envMap map[string]interface{}) eventDeployed {
	d := eventDeployed{}
	d.previous, d.hasPrevious = envMap[eventPreviousKey].(eventPrevious)
	if d.previous.Fifo == nil {
		d.previous.Fifo = map[string]bool{}
	}
	d.state, d.renderPath = envMap[eventsAwsccStateKey].(eventsAwsccState)
	return d
}

// fromState reports whether the state answers.
func (d eventDeployed) fromState() bool { return d.renderPath && d.state.Readable }

// fifoSetDeployed reports whether consumer c has a deployed FIFO queue set.
func (d eventDeployed) fifoSetDeployed(c string) bool {
	if d.fromState() {
		return containsString(d.state.FifoQueues, c)
	}
	_, ok := d.previous.Fifo[c]
	return ok
}

// activeFifoConsumers lists the consumers whose FIFO subscriber is deployed.
func (d eventDeployed) activeFifoConsumers() []string {
	var out []string
	if d.fromState() {
		for module := range d.state.Subscribers {
			if c, _, fifo, ok := parseEventSubscriberModule(module); ok && fifo {
				out = append(out, c)
			}
		}
	} else {
		for c, retired := range d.previous.Fifo {
			if !retired {
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return out
}

// deployedSubscribers is every subscriber module the last deploy holds, with
// its AWS name: state attributes, or offline the name recomputed from the
// marker module names.
func (d eventDeployed) deployedSubscribers(project, env string) map[string]string {
	out := map[string]string{}
	if d.fromState() {
		for m, n := range d.state.Subscribers {
			out[m] = n
		}
		return out
	}
	for _, m := range append(append([]string(nil), d.previous.Subscribers...), d.previous.Tombstones...) {
		if n, ok := eventModuleSubscriberName(project, env, m); ok {
			out[m] = n
		}
	}
	return out
}

// inState reports whether a subscriber module is already deployed.
func (d eventDeployed) inState(module string) bool {
	if d.fromState() {
		_, ok := d.state.Subscribers[module]
		return ok
	}
	return containsString(d.previous.Subscribers, module)
}

// previousType is the bus kind of the last deploy: enhanced, classic or none
// from state; enhanced, cleanup or classic from markers; "" when nothing is
// known.
func (d eventDeployed) previousType() string {
	if d.renderPath && d.state.Pulled {
		switch {
		case d.state.Bus:
			return eventBusTypeEnhanced
		case d.state.ClassicBus:
			return eventBusTypeClassic
		}
		return "none"
	}
	if !d.hasPrevious {
		return ""
	}
	switch d.previous.Mode {
	case awsccModeLive:
		return eventBusTypeEnhanced
	case awsccModeCleanup:
		return awsccModeCleanup
	}
	return eventBusTypeClassic
}

// awsccMode is the mode of this render (D12).
func (d eventDeployed) awsccMode(bus eventBusSettings) string {
	switch {
	case bus.Enhanced:
		return awsccModeLive
	case d.fromState():
		if d.state.Bus || len(d.state.Subscribers) > 0 {
			return awsccModeCleanup
		}
		return awsccModeOff
	case d.previous.Mode != "":
		return awsccModeCleanup
	}
	return awsccModeOff
}

// --- Chain ----------------------------------------------------------------

// eventLiveSubscriber is a subscriber module this render wants.
type eventLiveSubscriber struct {
	module string
	kind   string // standard or fifo
	key    string // "<c>/<k>" or "<c>"
	name   string // its AWS name
	where  string // YAML path, for messages
}

// eventSubscriberChain orders the subscriber modules: tombstones, then live
// modules new to state, then live modules in state, each group sorted by
// module name. Each entry names the module before it in after.
func eventSubscriberChain(live []eventLiveSubscriber, d eventDeployed) (chain []interface{}, tombstones []string) {
	wanted := map[string]bool{}
	for _, l := range live {
		wanted[l.module] = true
	}
	seen := map[string]bool{}
	addTomb := func(m string) {
		if !wanted[m] && !seen[m] {
			if _, _, _, ok := parseEventSubscriberModule(m); ok {
				seen[m] = true
				tombstones = append(tombstones, m)
			}
		}
	}
	if d.fromState() {
		for m := range d.state.Subscribers {
			addTomb(m)
		}
	} else {
		for _, m := range d.previous.Subscribers {
			addTomb(m)
		}
		for _, m := range d.previous.Tombstones {
			addTomb(m)
		}
	}
	sort.Strings(tombstones)

	var fresh, existing []eventLiveSubscriber
	for _, l := range live {
		if d.inState(l.module) {
			existing = append(existing, l)
		} else {
			fresh = append(fresh, l)
		}
	}
	byModule := func(s []eventLiveSubscriber) {
		sort.Slice(s, func(i, j int) bool { return s[i].module < s[j].module })
	}
	byModule(fresh)
	byModule(existing)

	chain = []interface{}{}
	after := ""
	for _, m := range tombstones {
		kind := "standard"
		if _, _, fifo, _ := parseEventSubscriberModule(m); fifo {
			kind = "fifo"
		}
		chain = append(chain, map[string]interface{}{"module": m, "kind": kind, "key": "", "enabled": false, "after": after})
		after = m
	}
	for _, group := range [][]eventLiveSubscriber{fresh, existing} {
		for _, l := range group {
			chain = append(chain, map[string]interface{}{"module": l.module, "kind": l.kind, "key": l.key, "enabled": true, "after": after})
			after = l.module
		}
	}
	return chain, tombstones
}

// eventSubscriberNameClashes refuses two live subscribers, or two delivery
// roles, with one AWS name. It mirrors module.naming_subscribers.collisions
// and module.naming_roles.collisions; the pools are separate because names
// in different AWS namespaces never clash.
func eventSubscriberNameClashes(project, env string, live []eventLiveSubscriber) []string {
	var problems []string
	subscribers := map[string]eventLiveSubscriber{}
	roles := map[string]string{}
	sorted := append([]eventLiveSubscriber(nil), live...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].module < sorted[j].module })
	for _, l := range sorted {
		if other, dup := subscribers[l.name]; dup {
			problems = append(problems, fmt.Sprintf("%s: the subscriber name %s is also the name of %s; give one subscription a distinct name", l.where, l.name, other.where))
		} else {
			subscribers[l.name] = l
		}
		c, _, _, _ := parseEventSubscriberModule(l.module)
		role := eventDeliveryRoleName(project, env, c)
		if owner, dup := roles[role]; dup && owner != c {
			problems = append(problems, fmt.Sprintf("%s: the delivery role name %s is also the name of the delivery role of %s; give one workload a distinct name", l.where, role, owner))
		} else {
			roles[role] = c
		}
	}
	return problems
}

// eventSubscriberTransitionClashes refuses a live subscriber whose name is the
// name of a deployed subscriber of another module (E28): every deployed
// subscriber still exists while the creates run.
func eventSubscriberTransitionClashes(live []eventLiveSubscriber, deployed map[string]string) []string {
	var problems []string
	modules := make([]string, 0, len(deployed))
	for m := range deployed {
		modules = append(modules, m)
	}
	sort.Strings(modules)
	for _, l := range live {
		for _, m := range modules {
			if m != l.module && deployed[m] == l.name {
				problems = append(problems, fmt.Sprintf(
					"%s: the subscriber name %s is still used by the deployed subscriber %s, which exists until this deploy has created the new one. Give the subscription another name (name:), or deploy the removal of the old one first",
					l.where, l.name, m))
			}
		}
	}
	return problems
}

// --- Revoke ---------------------------------------------------------------

// eventBusRevokeInputs computes module.event_bus_enhanced's revoke input
// (D-R). lost = (C ∪ (B − granted)) − granted, where B is what the last
// revoke step was applied with (or the RAM principals in state, or offline
// the share= markers) and C what a failed step still owes.
func eventBusRevokeInputs(share []eventShareEntry, d eventDeployed) (revoke map[string]interface{}, lost []string) {
	var granted, keepAccounts, keepOUs []string
	keepOrg := false
	for _, e := range share {
		if !subscribeCapable(e.Access) {
			continue
		}
		granted = append(granted, e.principal())
		switch e.Kind {
		case eventShareKindAccount:
			keepAccounts = append(keepAccounts, e.ID)
		case eventShareKindOU:
			keepOUs = append(keepOUs, e.ID)
		case eventShareKindOrganization:
			keepOrg = true
		}
	}
	sort.Strings(granted)
	sort.Strings(keepAccounts)
	sort.Strings(keepOUs)

	var baseline, carry []string
	if d.fromState() {
		if d.state.Revoke != nil {
			baseline = d.state.Revoke.Granted
			if d.state.Revoke.Tainted {
				carry = d.state.Revoke.Lost
			}
		} else {
			baseline = subscribeCapableFromRAM(d.state.RamPrincipals)
		}
	} else {
		baseline = subscribeCapableFromMarkers(d.previous.Share)
	}
	lostSet := map[string]bool{}
	for _, p := range append(append([]string(nil), carry...), baseline...) {
		if !containsString(granted, p) {
			lostSet[p] = true
		}
	}
	for p := range lostSet {
		lost = append(lost, p)
	}
	sort.Strings(lost)

	enabled := len(share) > 0 || (d.fromState() && d.state.Revoke != nil) || len(lost) > 0
	return map[string]interface{}{
		"enabled":       enabled,
		"granted":       toInterfaces(granted),
		"keep_accounts": toInterfaces(keepAccounts),
		"keep_org":      keepOrg,
		"keep_ous":      toInterfaces(keepOUs),
		"lost":          toInterfaces(lost),
	}, lost
}

// disabledEventBusRevoke is the revoke input of an env that is not live.
func disabledEventBusRevoke() map[string]interface{} {
	return map[string]interface{}{
		"enabled":       false,
		"granted":       []interface{}{},
		"keep_accounts": []interface{}{},
		"keep_org":      false,
		"keep_ous":      []interface{}{},
		"lost":          []interface{}{},
	}
}

func toInterfaces(items []string) []interface{} {
	out := make([]interface{}, 0, len(items))
	for _, s := range items {
		out = append(out, s)
	}
	return out
}

// eventShareRevokeDetail is the event_share_revoke warning text.
func eventShareRevokeDetail(project, env string, lost, granted []string) string {
	bus := project + "-" + env
	lines := make([]string, 0, len(lost)+1)
	orgLoss := false
	for _, p := range lost {
		lines = append(lines, fmt.Sprintf(
			"%s may no longer subscribe to %s. The next deploy revokes every subscriber that account created on the bus (or, for an organization or OU, every subscriber of an account no remaining entry covers). Revocation is permanent: they must create new subscribers if you share again.",
			eventSharePrincipalLabel(p), bus))
		if !strings.HasPrefix(p, eventShareKindAccount+":") {
			orgLoss = true
		}
	}
	remaining := ""
	for _, g := range granted {
		switch {
		case g == eventShareKindOrganization:
			remaining = "organization"
		case strings.HasPrefix(g, eventShareKindOU+":") && remaining == "":
			remaining = "OU"
		}
	}
	if orgLoss && remaining != "" {
		lines = append(lines, fmt.Sprintf(
			"Deciding which accounts the remaining %s entry still covers needs organizations:ListParents and organizations:DescribeAccount, which only the management account or a delegated administrator can call. If the deploy cannot call them, it stops and lists the subscribers it could not decide.",
			remaining))
	}
	return strings.Join(lines, "\n  ")
}

// --- Main step ------------------------------------------------------------

// eventConsumerResult is one consumer as normalizeEvents built it, with what
// the enhanced checks need.
type eventConsumerResult struct {
	w       eventWorkload
	subs    []interface{}
	wheres  []string // the YAML path of each of subs
	fifo    bool
	retired bool
}

// normalizeEnhancedEvents runs after the per-workload normalisation. It
// computes the awscc mode, the subscriber chain, the revoke inputs and the
// markers, runs the checks that need what was deployed, and writes the
// envMap keys of the template. It returns problems and warnings; the caller
// reports them with the rest.
func normalizeEnhancedEvents(envMap map[string]interface{}, bus eventBusSettings, share []eventShareEntry, consumers []eventConsumerResult, d eventDeployed) ([]string, []ConfigWarning) {
	project := stringFromMap(envMap, "project")
	env := stringFromMap(envMap, "env")
	mode := d.awsccMode(bus)

	var problems []string
	var warnings []ConfigWarning

	// E21: names a subscriber module and AWS name can carry.
	if mode != awsccModeOff {
		for _, c := range consumers {
			if !eventSubscriberConsumerPattern.MatchString(c.w.name) {
				problems = append(problems, fmt.Sprintf(
					"%s: workload name %s cannot name an enhanced-bus subscriber; use lower-case letters, digits and hyphens", c.w.field, c.w.name))
			}
		}
	}

	// The live subscribers.
	var live []eventLiveSubscriber
	var orderedKeys []string
	projectSubs := 0
	if mode == awsccModeLive {
		for _, c := range consumers {
			var ordered []map[string]interface{}
			for i, raw := range c.subs {
				sub, _ := raw.(map[string]interface{})
				if stringFromMap(sub, "bus") != eventBusProject {
					continue
				}
				projectSubs++
				key := stringFromMap(sub, "key")
				if isOrdered(sub) {
					ordered = append(ordered, sub)
					orderedKeys = append(orderedKeys, c.w.name+"/"+key)
					continue
				}
				live = append(live, eventLiveSubscriber{
					module: eventSubscriberModule(c.w.name, key),
					kind:   "standard",
					key:    c.w.name + "/" + key,
					name:   eventSubscriberName(project, env, c.w.name, key),
					where:  c.wheres[i],
				})
			}
			if len(ordered) == 0 {
				continue
			}
			where := c.w.field + ".subscribes"
			pattern := enhancedFifoPattern(ordered)
			n, m := enhancedPatternBytes(pattern), eventPatternOrCombinations(pattern)
			if n > enhancedPatternMaxBytes || m > enhancedPatternMaxCombinations {
				problems = append(problems, fmt.Sprintf(
					"%s: the ordered subscriptions together make filters of %d bytes, as AWS counts them, and a pattern with %d $or combinations; the enhanced bus allows %d bytes and %d combinations. Use fewer ordered subscriptions or shorter filter values",
					where, n, m, enhancedPatternMaxBytes, enhancedPatternMaxCombinations))
			}
			live = append(live, eventLiveSubscriber{
				module: eventFifoSubscriberModule(c.w.name),
				kind:   "fifo",
				key:    c.w.name,
				name:   eventFifoSubscriberName(project, env, c.w.name),
				where:  where,
			})
		}
		problems = append(problems, eventSubscriberNameClashes(project, env, live)...)
		problems = append(problems, eventSubscriberTransitionClashes(live, d.deployedSubscribers(project, env))...)
	}

	// E27: a sharing env whose state cannot be read.
	if d.renderPath && d.state.LockFile && !d.state.Readable && (len(share) > 0 || len(d.previous.Share) > 0) {
		problems = append(problems, fmt.Sprintf(
			"event_bus.share: the Terraform state of %s could not be read (%s), so meroku cannot tell which shared accounts lose access to the bus. Fix access to the state (AWS_PROFILE %s, region %s) and run it again",
			env, d.state.Err, stringFromMap(envMap, "aws_profile"), stringFromMap(envMap, "region")))
	}

	// Type change (E26 behind its switch, and the warning).
	previousType := d.previousType()
	envMap[eventBusPreviousTypeKey] = previousType
	compared := previousType
	if compared == awsccModeCleanup {
		compared = eventBusTypeClassic
	}
	previousEnabled := compared == eventBusTypeEnhanced
	if compared == eventBusTypeClassic {
		if d.renderPath && d.state.Pulled {
			previousEnabled = true
		} else {
			previousEnabled, _ = envMap[eventBusPreviousKey].(bool)
		}
	}
	current := eventBusType(bus)
	if previousEnabled && bus.Enabled && compared != current {
		if eventBusTypeFlipNeedsTwoDeploys {
			problems = append(problems, fmt.Sprintf(
				"event_bus.type changes from %s to %s in one deploy, but AWS cannot hold both buses named %s-%s at once. Turn the project bus off (event_bus.enabled: false) and deploy, then set type: %s and deploy again",
				compared, current, project, env, current))
		}
		client := "the classic EventBridge client and EVENT_BUS_NAME"
		if bus.Enhanced {
			client = "the EventBridge V2 client and EVENT_BUS_ARN"
		}
		if !bus.Enhanced {
			projectSubs = countProjectSubscriptions(consumers)
		}
		warnings = append(warnings, ConfigWarning{
			Code:  warnCodeEventBusTypeChange,
			Item:  workloadItem(WarningKindEventBus, ""),
			Title: "event_bus.type changed: the project bus is replaced",
			Detail: fmt.Sprintf(
				"The project bus %s-%s is deleted and created again as a %s bus. Every project-bus subscription is recreated (%d of them), and events published during the deploy are not delivered. Producers must publish with %s. Event-task rules on the project bus are refused on an enhanced bus.",
				project, env, current, projectSubs, client),
			Docs: docsLink("ai_docs/EVENTS.md"),
		})
	}

	// Ordering changes, from the previous markers.
	if mode == awsccModeLive && d.hasPrevious {
		warnings = append(warnings, eventOrderChangeWarnings(consumers, d)...)
	}

	// Queues of a workload that is no longer a consumer.
	if mode != awsccModeOff && d.fromState() {
		now := map[string]bool{}
		for _, c := range consumers {
			now[c.w.name] = true
		}
		for _, c := range d.state.Consumers {
			if now[c] {
				continue
			}
			names := eventQueueNames(project, env, c, false)
			warnings = append(warnings, ConfigWarning{
				Code:  warnCodeEventQueueDeleted,
				Item:  eventWorkloadItem(c),
				Title: fmt.Sprintf("%s's event queues are deleted", c),
				Detail: fmt.Sprintf(
					"%s has no subscription left, so this deploy deletes its queues %s, %s and %s, with any messages still in them. Keep one subscription until the queue is empty if those messages matter.",
					c, names[0], names[1], names[2]),
				Docs: docsLink("ai_docs/EVENTS.md"),
			})
		}
	}

	// Ordered consumers of a bus shared with publishers, when groups do not
	// carry the publishing account.
	if !eventGroupsCarryPublisherAccount && mode == awsccModeLive {
		publishers := false
		for _, e := range share {
			if e.Access == eventShareAccessPublish || e.Access == eventShareAccessFull {
				publishers = true
			}
		}
		if publishers {
			for _, c := range consumers {
				if c.fifo && !c.retired {
					warnings = append(warnings, eventWarning(c.w, warnCodeEventOrderedSharedPublishers, "Ordered events from shared accounts share groups",
						fmt.Sprintf("%s has ordered subscriptions on a bus other accounts publish to. Two accounts that use the same EventGroupId share one queue group: each account's events keep their order, the two are interleaved, and a stuck event of one holds up the other.", c.w.field)))
				}
			}
		}
	}

	// Share, revoke, guard.
	shareOut := []interface{}{}
	revoke := disabledEventBusRevoke()
	guard := false
	var lost []string
	switch mode {
	case awsccModeLive:
		for _, e := range share {
			shareOut = append(shareOut, map[string]interface{}{"kind": e.Kind, "id": e.ID, "access": e.Access})
		}
		revoke, lost = eventBusRevokeInputs(share, d)
		guard = len(share) > 0
		if d.fromState() {
			guard = guard || d.state.Revoke != nil || d.state.Guard
		} else {
			guard = guard || len(d.previous.Share) > 0
		}
		if len(lost) > 0 {
			granted, _ := revoke["granted"].([]interface{})
			grantedStrings := make([]string, 0, len(granted))
			for _, g := range granted {
				grantedStrings = append(grantedStrings, g.(string))
			}
			warnings = append(warnings, eventShareRevokeWarning(eventShareRevokeDetail(project, env, lost, grantedStrings)))
		}
	case awsccModeCleanup:
		leaving := false
		if d.fromState() {
			leaving = d.state.Guard
		} else {
			leaving = len(subscribeCapableFromMarkers(d.previous.Share)) > 0
		}
		if leaving {
			warnings = append(warnings, eventShareRevokeWarning(fmt.Sprintf(
				"%s-%s leaves the enhanced bus, so this deploy revokes every subscriber and event source other accounts created on it before the bus is deleted. Revocation is permanent: they must create new subscribers if you share again.",
				project, env)))
		}
	}

	// Chain and markers.
	chain := []interface{}{}
	var tombstones []string
	if mode != awsccModeOff {
		chain, tombstones = eventSubscriberChain(live, d)
	}
	var markers []string
	if mode != awsccModeOff {
		markers = append(markers, eventMarkerPrefix+"awscc="+mode)
	}
	for _, l := range live {
		markers = append(markers, eventMarkerPrefix+"subscriber="+l.module)
	}
	for _, t := range tombstones {
		markers = append(markers, eventMarkerPrefix+"tombstone="+t)
	}
	for _, k := range orderedKeys {
		markers = append(markers, eventMarkerPrefix+"ordered="+k)
	}
	anyFifo := false
	for _, c := range consumers {
		if !c.fifo {
			continue
		}
		anyFifo = true
		line := eventMarkerPrefix + "fifo=" + c.w.name
		if c.retired {
			line += " retired"
		}
		markers = append(markers, line)
	}
	if mode == awsccModeLive {
		for _, e := range share {
			markers = append(markers, eventMarkerPrefix+"share="+e.marker())
		}
	}
	sort.Strings(markers)

	envMap[awsccModeKey] = mode
	envMap[eventBusEnhancedKey] = mode == awsccModeLive
	envMap[awsccRenderKey] = mode != awsccModeOff
	envMap[eventBusShareKey] = shareOut
	envMap[eventBusRevokeKey] = revoke
	envMap[eventBusGuardKey] = guard
	envMap[eventSubscriberChainKey] = chain
	envMap[eventMarkersKey] = toInterfaces(markers)
	envMap[eventBridgeFifoKey] = mode != awsccModeOff || anyFifo
	return problems, warnings
}

func isOrdered(sub map[string]interface{}) bool {
	b, _ := sub["ordered"].(bool)
	return b
}

func countProjectSubscriptions(consumers []eventConsumerResult) int {
	n := 0
	for _, c := range consumers {
		for _, raw := range c.subs {
			sub, _ := raw.(map[string]interface{})
			if stringFromMap(sub, "bus") == eventBusProject {
				n++
			}
		}
	}
	return n
}

func eventShareRevokeWarning(detail string) ConfigWarning {
	return ConfigWarning{
		Code:   warnCodeEventShareRevoke,
		Item:   workloadItem(WarningKindEventBus, ""),
		Title:  "Shared subscribers are revoked",
		Detail: detail,
		Docs:   docsLink("ai_docs/EVENTS.md"),
	}
}

// eventOrderChangeWarnings warns each subscription whose ordered setting
// differs from the previous render: an ordered= marker now unordered, or an
// ordered subscription whose standard subscriber was deployed.
func eventOrderChangeWarnings(consumers []eventConsumerResult, d eventDeployed) []ConfigWarning {
	var out []ConfigWarning
	for _, c := range consumers {
		for i, raw := range c.subs {
			sub, _ := raw.(map[string]interface{})
			if stringFromMap(sub, "bus") != eventBusProject {
				continue
			}
			key := stringFromMap(sub, "key")
			ck := c.w.name + "/" + key
			was := containsString(d.previous.Ordered, ck)
			now := isOrdered(sub)
			standardDeployed := d.inState(eventSubscriberModule(c.w.name, key))
			if was == now || (now && !standardDeployed) {
				continue
			}
			from, to, oldVar := "ordered", "standard", "EVENT_FIFO_QUEUE_URL"
			if now {
				from, to, oldVar = "standard", "ordered", "EVENT_QUEUE_URL"
			}
			out = append(out, eventWarning(c.w, warnCodeEventSubscriptionOrderChange, "Subscription ordering changed",
				fmt.Sprintf("%s.ordered changed to %t: its events move from the %s queue to the %s queue. The new subscriber is created before the old one is deleted, so for a short time events arrive on both; dedupe on id. Before you deploy, check that the %s delivery DLQ alarm of %s is OK: events the old subscriber is still retrying when it is deleted are lost. Read %s until it is empty before removing code that reads it.",
					c.wheres[i], now, from, to, from, c.w.name, oldVar)))
		}
	}
	return out
}

// eventWorkloadItem is the warning item of a workload named in state.
func eventWorkloadItem(name string) WarningItem {
	if name == backendWorkloadName {
		return workloadItem(WarningKindBackend, "")
	}
	return workloadItem(WarningKindService, name)
}

// eventWorkloadField finds a backend or service by name in the YAML, for a
// workload that may no longer have an events block. ok is false when the
// workload is not in the YAML.
func eventWorkloadField(envMap map[string]interface{}, name string) (string, eventWorkload, bool) {
	if name == backendWorkloadName {
		if _, ok := yamlSubMap(envMap, "workload"); ok {
			return "workload.events", eventWorkload{name: name, kind: eventKindBackend, field: "workload.events"}, true
		}
		return "", eventWorkload{}, false
	}
	items, _ := envMap["services"].([]interface{})
	for i, raw := range items {
		item, ok := computeNodeMap(raw)
		if ok && stringFromMap(item, "name") == name {
			field := fmt.Sprintf("services[%d].events ('%s')", i, name)
			return field, eventWorkload{name: name, kind: eventKindService, field: field}, true
		}
	}
	return "", eventWorkload{}, false
}

// eventFifoRetirementProblems is E23: a deployed active FIFO subscriber whose
// workload is still in the YAML, has no ordered subscription now, and does
// not set fifo_retired. It walks the deployed FIFO consumers, so a workload
// whose events block was removed is still checked.
func eventFifoRetirementProblems(envMap map[string]interface{}, consumers []eventConsumerResult, retiredBy map[string]bool, d eventDeployed) []string {
	project := stringFromMap(envMap, "project")
	env := stringFromMap(envMap, "env")
	ordered := map[string]bool{}
	for _, c := range consumers {
		for _, raw := range c.subs {
			sub, _ := raw.(map[string]interface{})
			if isOrdered(sub) {
				ordered[c.w.name] = true
			}
		}
	}
	var problems []string
	for _, c := range d.activeFifoConsumers() {
		if ordered[c] || retiredBy[c] {
			continue
		}
		field, _, present := eventWorkloadField(envMap, c)
		if !present {
			continue
		}
		problems = append(problems, fmt.Sprintf(
			"%s.queue: %s no longer has an ordered subscription, but its ordered queue %s is deployed and may still hold events. Set events.queue.fifo_retired: true to keep that queue with no subscription, deploy, read what it holds (EVENT_FIFO_QUEUE_URL), then remove fifo_retired to delete it",
			field, c, eventFifoQueueName(project, env, c)))
	}
	return problems
}
