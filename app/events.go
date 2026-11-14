package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Events on a project bus (modules/events).
//
//	event_bus: { enabled: true }          # creates <project>-<env>
//	services:
//	  - name: orders
//	    events:
//	      subscribes:
//	        - { detail_type: payment.completed, source: example.payments }
//	        - { name: users, detail_type_prefix: "user.", bus: default }
//	      queue: { visibility_timeout: 30, max_receive_count: 5 }
//	      produces:
//	        - { detail_types: [order.created], own_source_only: true }
//	        - { any: true, bus: default }
//	workload: { events: { ... } }         # the backend, keyed "backend"
//	scheduled_tasks: [{ name: nightly, events: { produces: [...] } }]
//	event_processor_tasks: [{ name: mailer, rules: [{ name: r1, ..., bus: default }] }]
//
// A workload that subscribes to anything is a consumer: it gets its own queue,
// an application dead-letter queue, a delivery dead-letter queue, an alarm on
// each dead-letter queue and EVENT_QUEUE_URL. A workload with produces entries
// gets events:PutEvents on the listed buses and nothing else; without one it
// cannot publish at all.
//
// An entry or event-task rule without `bus` means the project bus when
// event_bus.enabled is true and the default bus when it is false. The resolved
// value is written as `_bus` ("project" or "default").
//
// See ai_docs/EVENTS.md.

// eventConsumersKey is the envMap key holding the consumers modules/events
// renders: [{name, visibility_timeout, max_receive_count, subscriptions}].
// app/workload_env_reserved.go reads it to reserve EVENT_QUEUE_URL.
const eventConsumersKey = "_event_consumers"

// eventProducersKey is the envMap key holding the producers modules/events
// grants PutEvents to: [{name, source, entries}].
const eventProducersKey = "_event_producers"

// eventBusPreviousKey is the envMap key applyTemplate sets to the bus state the
// environment's current main.tf was generated with (a bool), when one exists.
// normalizeEvents compares it with event_bus.enabled to warn about what moves.
const eventBusPreviousKey = "_event_bus_previous"

// Resolved bus values.
const (
	eventBusProject = "project"
	eventBusDefault = "default"
)

// Queue settings: the only two a consumer can change, with their defaults and
// the ranges SQS accepts.
const (
	eventQueueVisibilityDefault = 30
	eventQueueVisibilityMax     = 43200
	eventQueueMaxReceiveDefault = 5
	eventQueueMaxReceiveMax     = 1000
)

// eventSubscriptionNamePattern is an explicit subscription name. The key is a
// part of the EventBridge rule name, which accepts [.\-_A-Za-z0-9].
var eventSubscriptionNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// eventKeyMaxLen is how long a derived key may be before "-default".
const eventKeyMaxLen = 32

var eventKeyUnsafeRun = regexp.MustCompile(`[^a-z0-9]+`)

// Workload kinds, as the event source formulas tell them apart.
const (
	eventKindBackend       = "backend"
	eventKindService       = "service"
	eventKindScheduledTask = "scheduled_task"
	eventKindEventTask     = "event_task"
)

// eventSource is the EVENT_SOURCE each owning module sets on a workload:
// modules/workloads/env.tf (backend), env_services.tf (services),
// modules/ecs_task/env.tf and modules/event_bridge_task/env.tf. The four
// formulas are pinned by app/testdata/event_source_vectors.json.
func eventSource(kind, project, name string) string {
	switch kind {
	case eventKindBackend:
		return project + ".backend"
	case eventKindService:
		return project + "." + name
	case eventKindScheduledTask:
		return project + ".task." + name
	case eventKindEventTask:
		return project + ".event." + name
	}
	return ""
}

// deriveEventSubscriptionKey builds the key of a subscription that has no name:
// the detail-type, or the prefix followed by "-prefix", lower-cased, every run of
// characters outside [a-z0-9] turned into "-", trimmed and cut to 32, with
// "-default" appended when the subscription is on the default bus. Returns "" when
// nothing usable is left.
func deriveEventSubscriptionKey(detailType, detailTypePrefix, bus string) string {
	base := detailType
	if base == "" {
		base = detailTypePrefix + "-prefix"
	}
	key := eventKeyUnsafeRun.ReplaceAllString(strings.ToLower(base), "-")
	key = strings.Trim(key, "-")
	if len(key) > eventKeyMaxLen {
		key = strings.TrimRight(key[:eventKeyMaxLen], "-")
	}
	if key == "" {
		return ""
	}
	if bus == eventBusDefault {
		key += "-default"
	}
	return key
}

// eventWorkload is one workload's events block and where it lives.
type eventWorkload struct {
	name   string
	kind   string
	field  string // YAML path of the events block, for messages
	events map[string]interface{}
}

// eventWorkloads lists every workload with an events block, backend first.
func eventWorkloads(envMap map[string]interface{}) ([]eventWorkload, []string) {
	var out []eventWorkload
	var problems []string

	add := func(name, kind, field string, raw interface{}, present bool) {
		if !present || raw == nil {
			return
		}
		events, ok := computeNodeMap(raw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with subscribes, queue and produces", field))
			return
		}
		out = append(out, eventWorkload{name: name, kind: kind, field: field, events: events})
	}

	if workload, ok := yamlSubMap(envMap, "workload"); ok {
		raw, present := workload["events"]
		add(backendWorkloadName, eventKindBackend, "workload.events", raw, present)
	}
	lists := []struct{ key, kind string }{
		{"services", eventKindService},
		{"scheduled_tasks", eventKindScheduledTask},
		{"event_processor_tasks", eventKindEventTask},
	}
	for _, list := range lists {
		items, _ := envMap[list.key].([]interface{})
		for i, rawItem := range items {
			item, ok := computeNodeMap(rawItem)
			if !ok {
				continue
			}
			name := stringFromMap(item, "name")
			if name == "" {
				continue
			}
			raw, present := item["events"]
			add(name, list.kind, fmt.Sprintf("%s[%d].events ('%s')", list.key, i, name), raw, present)
		}
	}
	return out, problems
}

// eventBusEnabled reads event_bus.enabled.
func eventBusEnabled(envMap map[string]interface{}) (bool, error) {
	raw, present := envMap["event_bus"]
	if !present || raw == nil {
		return false, nil
	}
	block, ok := computeNodeMap(raw)
	if !ok {
		return false, fmt.Errorf("event_bus must be a mapping with `enabled: true` or `enabled: false`")
	}
	enabledRaw, present := block["enabled"]
	if !present || enabledRaw == nil {
		return false, nil
	}
	enabled, ok := enabledRaw.(bool)
	if !ok {
		return false, fmt.Errorf("event_bus.enabled must be true or false, got %s", formatYAMLValue(enabledRaw))
	}
	return enabled, nil
}

// resolveEventBus turns an entry's optional `bus` into "project" or "default".
// explicit reports whether the YAML set it, so the toggle warning can list the
// entries that follow the bus switch.
func resolveEventBus(raw interface{}, busEnabled bool, where string) (bus string, explicit bool, problem string) {
	if raw == nil {
		if busEnabled {
			return eventBusProject, false, ""
		}
		return eventBusDefault, false, ""
	}
	s, _ := raw.(string)
	switch s {
	case eventBusDefault:
		return eventBusDefault, true, ""
	case eventBusProject:
		if !busEnabled {
			return "", true, fmt.Sprintf("%s.bus is project, but event_bus.enabled is not true; turn the project bus on or use bus: default", where)
		}
		return eventBusProject, true, ""
	}
	return "", true, fmt.Sprintf("%s.bus must be project or default, got %s", where, formatYAMLValue(raw))
}

// eventInt reads a whole number from a YAML value.
func eventInt(raw interface{}) (int, bool) {
	switch v := raw.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case uint64:
		return int(v), true
	case float64:
		if v == math.Trunc(v) {
			return int(v), true
		}
	}
	return 0, false
}

// eventStringList reads a list of non-empty strings.
func eventStringList(raw interface{}) ([]string, bool) {
	items, ok := raw.([]interface{})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s, ok := item.(string)
		if !ok || strings.TrimSpace(s) == "" {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

func checkEventKeys(m map[string]interface{}, allowed []string, where string) []string {
	var problems []string
	for k := range m {
		known := false
		for _, a := range allowed {
			if k == a {
				known = true
				break
			}
		}
		if !known {
			problems = append(problems, fmt.Sprintf("%s has an unknown field %q; allowed: %s", where, k, strings.Join(allowed, ", ")))
		}
	}
	return problems
}

// eventMove is an entry whose bus follows event_bus.enabled, for the toggle
// warning.
type eventMove struct{ where string }

// normalizeEvents validates every events block and event-task rule, resolves
// buses and subscription keys, and writes eventConsumersKey and
// eventProducersKey. It deletes `events` from the service maps afterwards, so the
// block never travels through {{{array services}}}.
func normalizeEvents(envMap map[string]interface{}) error {
	project := stringFromMap(envMap, "project")
	env := stringFromMap(envMap, "env")

	envMap[eventConsumersKey] = []interface{}{}
	envMap[eventProducersKey] = []interface{}{}

	busEnabled, err := eventBusEnabled(envMap)
	if err != nil {
		return err
	}

	workloads, problems := eventWorkloads(envMap)
	var warnings []string
	var moves []eventMove
	var consumers, producers []interface{}

	for _, w := range workloads {
		problems = append(problems, checkEventKeys(w.events, []string{"subscribes", "queue", "produces"}, w.field)...)

		subs, subProblems, subMoves := normalizeEventSubscriptions(w, busEnabled)
		problems = append(problems, subProblems...)
		moves = append(moves, subMoves...)

		queue, queueProblems := normalizeEventQueue(w)
		problems = append(problems, queueProblems...)
		if q, has := w.events["queue"]; has && q != nil {
			switch {
			case w.kind == eventKindScheduledTask || w.kind == eventKindEventTask:
				problems = append(problems, fmt.Sprintf("%s.queue: tasks have no event queue; remove it", w.field))
			case len(subs) == 0:
				warnings = append(warnings, fmt.Sprintf("%s.queue has no effect: the workload subscribes to nothing, so it has no queue", w.field))
			}
		}
		if len(subs) > 0 {
			consumers = append(consumers, map[string]interface{}{
				"name":               w.name,
				"visibility_timeout": queue.visibilityTimeout,
				"max_receive_count":  queue.maxReceiveCount,
				"subscriptions":      subs,
			})
		}

		entries, produceProblems, produceWarnings, produceMoves := normalizeEventProduces(w, busEnabled)
		problems = append(problems, produceProblems...)
		warnings = append(warnings, produceWarnings...)
		moves = append(moves, produceMoves...)
		if len(entries) > 0 {
			producers = append(producers, map[string]interface{}{
				"name":    w.name,
				"source":  eventSource(w.kind, project, w.name),
				"entries": entries,
			})
		}
	}

	ruleProblems, ruleMoves := normalizeEventTaskRules(envMap, project, env, busEnabled)
	problems = append(problems, ruleProblems...)
	moves = append(moves, ruleMoves...)

	problems = append(problems, eventQueueNameClashes(envMap, project, env, consumers)...)
	if len(consumers) > 0 {
		problems = append(problems, eventAlertsTopicClashes(envMap, project, env)...)
	}

	if len(problems) > 0 {
		sort.Strings(problems)
		return fmt.Errorf("events:\n  %s", strings.Join(problems, "\n  "))
	}

	for _, warning := range warnings {
		addGenerateWarning(envMap, warning)
	}
	if previous, ok := envMap[eventBusPreviousKey].(bool); ok && previous != busEnabled {
		addGenerateWarning(envMap, eventBusToggleWarning(project, env, busEnabled, moves))
	}

	envMap[eventConsumersKey] = consumers
	envMap[eventProducersKey] = producers
	if consumers == nil {
		envMap[eventConsumersKey] = []interface{}{}
	}
	if producers == nil {
		envMap[eventProducersKey] = []interface{}{}
	}
	moveServiceEventsOut(envMap)
	return nil
}

// normalizeEventSubscriptions validates a workload's subscribes list and returns
// the subscriptions modules/events renders.
func normalizeEventSubscriptions(w eventWorkload, busEnabled bool) ([]interface{}, []string, []eventMove) {
	raw, present := w.events["subscribes"]
	if !present || raw == nil {
		return nil, nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, []string{fmt.Sprintf("%s.subscribes must be a list", w.field)}, nil
	}
	if len(items) == 0 {
		return nil, nil, nil
	}
	if w.kind == eventKindScheduledTask || w.kind == eventKindEventTask {
		return nil, []string{fmt.Sprintf(
			"%s.subscribes: tasks cannot subscribe; an event task runs from its rules, and a service or the backend subscribes with its own queue", w.field)}, nil
	}

	var problems []string
	var moves []eventMove
	var out []interface{}
	keys := map[string]int{}

	for i, itemRaw := range items {
		where := fmt.Sprintf("%s.subscribes[%d]", w.field, i)
		item, ok := computeNodeMap(itemRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with detail_type or detail_type_prefix", where))
			continue
		}
		problems = append(problems, checkEventKeys(item, []string{"name", "detail_type", "detail_type_prefix", "source", "bus"}, where)...)

		detailType, _ := item["detail_type"].(string)
		prefix, _ := item["detail_type_prefix"].(string)
		if (detailType == "") == (prefix == "") {
			problems = append(problems, fmt.Sprintf("%s needs exactly one of detail_type or detail_type_prefix", where))
			continue
		}
		source := ""
		if s, has := item["source"]; has && s != nil {
			str, ok := s.(string)
			if !ok || str == "" {
				problems = append(problems, fmt.Sprintf("%s.source must be a non-empty string", where))
				continue
			}
			source = str
		}
		bus, explicit, problem := resolveEventBus(item["bus"], busEnabled, where)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		if !explicit {
			moves = append(moves, eventMove{where: where})
		}

		key := ""
		if n, has := item["name"]; has && n != nil {
			name, _ := n.(string)
			if !eventSubscriptionNamePattern.MatchString(name) {
				problems = append(problems, fmt.Sprintf(
					"%s.name %s: use 1 to 32 lower-case letters, digits and hyphens, starting with a letter or a digit", where, formatYAMLValue(n)))
				continue
			}
			key = name
		} else {
			key = deriveEventSubscriptionKey(detailType, prefix, bus)
			if key == "" {
				problems = append(problems, fmt.Sprintf("%s: no key can be derived from the detail-type; give the entry a name", where))
				continue
			}
		}
		if first, dup := keys[key]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s has the key %q, as subscribes[%d] does; give one of them a distinct name", where, key, first))
			continue
		}
		keys[key] = i

		out = append(out, map[string]interface{}{
			"key":                key,
			"detail_type":        detailType,
			"detail_type_prefix": prefix,
			"source":             source,
			"bus":                bus,
		})
	}
	return out, problems, moves
}

type eventQueueSettings struct {
	visibilityTimeout int
	maxReceiveCount   int
}

func normalizeEventQueue(w eventWorkload) (eventQueueSettings, []string) {
	settings := eventQueueSettings{visibilityTimeout: eventQueueVisibilityDefault, maxReceiveCount: eventQueueMaxReceiveDefault}
	raw, present := w.events["queue"]
	if !present || raw == nil {
		return settings, nil
	}
	where := w.field + ".queue"
	queue, ok := computeNodeMap(raw)
	if !ok {
		return settings, []string{fmt.Sprintf("%s must be a mapping with visibility_timeout and max_receive_count", where)}
	}
	problems := checkEventKeys(queue, []string{"visibility_timeout", "max_receive_count"}, where)
	if v, has := queue["visibility_timeout"]; has && v != nil {
		n, ok := eventInt(v)
		if !ok || n < 0 || n > eventQueueVisibilityMax {
			problems = append(problems, fmt.Sprintf("%s.visibility_timeout must be a whole number of seconds from 0 to %d, got %s", where, eventQueueVisibilityMax, formatYAMLValue(v)))
		} else {
			settings.visibilityTimeout = n
		}
	}
	if v, has := queue["max_receive_count"]; has && v != nil {
		n, ok := eventInt(v)
		if !ok || n < 1 || n > eventQueueMaxReceiveMax {
			problems = append(problems, fmt.Sprintf("%s.max_receive_count must be a whole number from 1 to %d, got %s", where, eventQueueMaxReceiveMax, formatYAMLValue(v)))
		} else {
			settings.maxReceiveCount = n
		}
	}
	return settings, problems
}

// normalizeEventProduces validates a workload's produces list.
func normalizeEventProduces(w eventWorkload, busEnabled bool) ([]interface{}, []string, []string, []eventMove) {
	raw, present := w.events["produces"]
	if !present || raw == nil {
		return nil, nil, nil, nil
	}
	items, ok := raw.([]interface{})
	if !ok {
		return nil, []string{fmt.Sprintf("%s.produces must be a list", w.field)}, nil, nil
	}

	var problems, warnings []string
	var moves []eventMove
	var out []interface{}

	for i, itemRaw := range items {
		where := fmt.Sprintf("%s.produces[%d]", w.field, i)
		item, ok := computeNodeMap(itemRaw)
		if !ok {
			problems = append(problems, fmt.Sprintf("%s must be a mapping with detail_types or any: true", where))
			continue
		}
		problems = append(problems, checkEventKeys(item, []string{"detail_types", "any", "own_source_only", "bus"}, where)...)

		anyType := false
		if a, has := item["any"]; has && a != nil {
			b, ok := a.(bool)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.any must be true or false", where))
				continue
			}
			anyType = b
		}
		var detailTypes []string
		if d, has := item["detail_types"]; has && d != nil {
			list, ok := eventStringList(d)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.detail_types must be a list of detail-type names", where))
				continue
			}
			detailTypes = list
		}
		if anyType == (len(detailTypes) > 0) {
			problems = append(problems, fmt.Sprintf("%s needs exactly one of a non-empty detail_types list or any: true", where))
			continue
		}
		ownSourceOnly := true
		if o, has := item["own_source_only"]; has && o != nil {
			b, ok := o.(bool)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s.own_source_only must be true or false", where))
				continue
			}
			ownSourceOnly = b
		}
		bus, explicit, problem := resolveEventBus(item["bus"], busEnabled, where)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		if !explicit {
			moves = append(moves, eventMove{where: where})
		}
		if anyType {
			warnings = append(warnings, fmt.Sprintf(
				"%s has any: true, so the workload may publish every detail-type to the %s bus; list detail_types to narrow it", where, bus))
		}

		dt := make([]interface{}, 0, len(detailTypes))
		for _, d := range detailTypes {
			dt = append(dt, d)
		}
		out = append(out, map[string]interface{}{
			"bus":             bus,
			"detail_types":    dt,
			"any":             anyType,
			"own_source_only": ownSourceOnly,
		})
	}
	return out, problems, warnings, moves
}

// eventTaskRuleName is the AWS name modules/event_bridge_task gives the rule with
// this key (its naming.tf).
func eventTaskRuleName(project, env, key string) string {
	legacy := fmt.Sprintf("%s_rule_%s_%s", project, key, env)
	return awsName(project, env, []string{"rule", key}, awsNameLimits["event_rule"], "_", "", legacy)
}

// normalizeEventTaskRules resolves each event-task rule's bus into `_bus` and
// refuses two rules with the same AWS name on the same bus. The legacy single-rule
// form has no bus field and follows the environment's bus.
func normalizeEventTaskRules(envMap map[string]interface{}, project, env string, busEnabled bool) ([]string, []eventMove) {
	var problems []string
	var moves []eventMove

	type owner struct{ where, task string }
	seen := map[string]owner{}
	claim := func(bus, key, where, task string) {
		name := eventTaskRuleName(project, env, key)
		id := bus + "\x00" + name
		if other, dup := seen[id]; dup {
			problems = append(problems, fmt.Sprintf(
				"%s and %s both create the rule %s on the %s bus, and one bus holds one rule per name; rename one rule",
				other.where, where, name, bus))
			return
		}
		seen[id] = owner{where: where, task: task}
	}

	envBus := eventBusDefault
	if busEnabled {
		envBus = eventBusProject
	}

	items, _ := envMap["event_processor_tasks"].([]interface{})
	for i, rawItem := range items {
		task, ok := computeNodeMap(rawItem)
		if !ok {
			continue
		}
		taskName := stringFromMap(task, "name")
		taskWhere := fmt.Sprintf("event_processor_tasks[%d] ('%s')", i, taskName)

		rules, _ := task["rules"].([]interface{})
		if len(rules) == 0 {
			if key := stringFromMap(task, "rule_name"); key != "" {
				where := taskWhere + ".rule_name"
				moves = append(moves, eventMove{where: where})
				claim(envBus, key, where, taskName)
			}
			continue
		}
		for j, rawRule := range rules {
			where := fmt.Sprintf("%s.rules[%d]", taskWhere, j)
			bus, explicit, problem := resolveEventBus(ruleField(rawRule, "bus"), busEnabled, where)
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			if !explicit {
				moves = append(moves, eventMove{where: where})
			}
			setRuleField(rawRule, "_bus", bus)
			if key := stringFromRule(rawRule, "name"); key != "" {
				claim(bus, key, fmt.Sprintf("%s ('%s')", where, key), taskName)
			}
		}
	}
	return problems, moves
}

func ruleField(raw interface{}, key string) interface{} {
	switch m := raw.(type) {
	case map[string]interface{}:
		return m[key]
	case map[interface{}]interface{}:
		return m[key]
	}
	return nil
}

func stringFromRule(raw interface{}, key string) string {
	s, _ := ruleField(raw, key).(string)
	return s
}

// setRuleField writes into the rule map itself, whichever map type the loader
// produced, so the template sees the value inside {{#each rules}}.
func setRuleField(raw interface{}, key string, value interface{}) {
	switch m := raw.(type) {
	case map[string]interface{}:
		m[key] = value
	case map[interface{}]interface{}:
		m[key] = value
	}
}

// eventQueueNames returns the three queue names modules/events gives a consumer
// (its naming.tf): main, application DLQ, delivery DLQ.
func eventQueueNames(project, env, consumer string) []string {
	limit := awsNameLimits["sqs_queue"]
	return []string{
		awsName(project, env, []string{"events", consumer}, limit, "-", "", ""),
		awsName(project, env, []string{"events", consumer, "dlq"}, limit, "-", "", ""),
		awsName(project, env, []string{"events", consumer, "delivery-dlq"}, limit, "-", "", ""),
	}
}

// extensionQueueNames returns the names env/main.hbs gives an
// extensions.sqs_queues entry: the queue, and its DLQ when dlq_enabled is set.
func extensionQueueNames(project, env string, queue map[string]interface{}) []string {
	name := stringFromMap(queue, "name")
	suffix := awsNameSuffix(queue["fifo"])
	limit := awsNameLimits["sqs_queue"]
	main := []string{name}
	out := []string{awsName(project, env, main, limit, "-", suffix, legacyName(project, env, main, "-", suffix))}
	if isTruthy(queue["dlq_enabled"]) {
		dlq := []string{name, "dlq"}
		out = append(out, awsName(project, env, dlq, limit, "-", suffix, legacyName(project, env, dlq, "-", suffix)))
	}
	return out
}

// eventQueueNameClashes refuses an extensions.sqs_queues entry whose queue name
// equals one of an event consumer's queues. CreateQueue with an existing name
// and the same attributes succeeds, so two Terraform addresses would own one
// queue.
func eventQueueNameClashes(envMap map[string]interface{}, project, env string, consumers []interface{}) []string {
	ext, ok := yamlSubMap(envMap, "extensions")
	if !ok {
		return nil
	}
	queues, _ := ext["sqs_queues"].([]interface{})
	if len(queues) == 0 || len(consumers) == 0 {
		return nil
	}
	owned := map[string]string{}
	for _, raw := range consumers {
		c, _ := raw.(map[string]interface{})
		name := stringFromMap(c, "name")
		for _, q := range eventQueueNames(project, env, name) {
			owned[q] = name
		}
	}
	var problems []string
	for i, raw := range queues {
		q, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		for _, n := range extensionQueueNames(project, env, q) {
			if consumer, clash := owned[n]; clash {
				problems = append(problems, fmt.Sprintf(
					"extensions.sqs_queues[%d] ('%s') creates the queue %s, which is also the event queue of %s; rename the extension queue",
					i, stringFromMap(q, "name"), n, consumer))
			}
		}
	}
	return problems
}

// eventAlertsTopicName is the alerts topic modules/events creates when the
// environment has at least one consumer.
func eventAlertsTopicName(project, env string) string {
	parts := []string{"alerts"}
	return awsName(project, env, parts, awsNameLimits["sns_topic"], "-", "", legacyName(project, env, parts, "-", ""))
}

// eventAlertsTopicClashes refuses an extensions.sns_topics entry named like the
// alerts topic. CreateTopic is idempotent by name, so both would own one topic.
func eventAlertsTopicClashes(envMap map[string]interface{}, project, env string) []string {
	ext, ok := yamlSubMap(envMap, "extensions")
	if !ok {
		return nil
	}
	topics, _ := ext["sns_topics"].([]interface{})
	alerts := eventAlertsTopicName(project, env)
	var problems []string
	for i, raw := range topics {
		t, ok := computeNodeMap(raw)
		if !ok {
			continue
		}
		parts := []string{stringFromMap(t, "name")}
		suffix := awsNameSuffix(t["fifo"])
		name := awsName(project, env, parts, awsNameLimits["sns_topic"], "-", suffix, legacyName(project, env, parts, "-", suffix))
		if name == alerts {
			problems = append(problems, fmt.Sprintf(
				"extensions.sns_topics[%d] ('%s') creates the topic %s, which events creates for its dead-letter alarms because this environment has event consumers; rename the extension topic",
				i, stringFromMap(t, "name"), name))
		}
	}
	return problems
}

// eventBusToggleWarning lists everything that moves when event_bus.enabled
// changes: entries and rules without an explicit bus, and EVENT_BUS_NAME.
func eventBusToggleWarning(project, env string, busEnabled bool, moves []eventMove) string {
	from, to := "default", project+"-"+env
	if !busEnabled {
		from, to = to, from
	}
	lines := []string{fmt.Sprintf(
		"event_bus.enabled changed: EVENT_BUS_NAME moves from the %s bus to the %s bus in every container, and event-task rules on it are replaced.", from, to)}
	if len(moves) == 0 {
		lines = append(lines, "No events entry or rule follows the bus switch.")
	} else {
		where := make([]string, 0, len(moves))
		for _, m := range moves {
			where = append(where, m.where)
		}
		sort.Strings(where)
		header := "These have no bus field and move with it:"
		if busEnabled {
			header = "These have no bus field and move with it (set bus: default to keep one on the default bus):"
		}
		lines = append(lines, header)
		for _, w := range where {
			lines = append(lines, "    "+w)
		}
	}
	return strings.Join(lines, "\n  ")
}

// moveServiceEventsOut deletes `events` from every service map. The service list
// renders with {{{array services}}}, which does not escape ${, and
// modules/workloads has no events field; the normalised block lives under
// eventConsumersKey and eventProducersKey.
func moveServiceEventsOut(envMap map[string]interface{}) {
	services, _ := envMap["services"].([]interface{})
	for _, raw := range services {
		switch service := raw.(type) {
		case map[string]interface{}:
			delete(service, "events")
		case map[interface{}]interface{}:
			delete(service, "events")
		}
	}
}

// eventBusPreviousPattern finds the bus switch in a rendered module "events".
var eventBusPreviousPattern = regexp.MustCompile(`(?s)module "events" \{.*?\n\s*bus_enabled\s*=\s*(true|false)`)

// previousEventBusState reads the bus state from env/<env>/main.tf as last
// generated. A main.tf without module "events" predates the project bus, which
// means the default bus. ok is false when there is no main.tf: a new environment
// has nothing to move.
func previousEventBusState(env string) (enabled bool, ok bool) {
	content, err := os.ReadFile(filepath.Join("env", env, "main.tf"))
	if err != nil {
		return false, false
	}
	m := eventBusPreviousPattern.FindSubmatch(content)
	if m == nil {
		return false, true
	}
	return string(m[1]) == "true", true
}

// eventsBridgeBlock is the events part of local.bridge (generateBridgeFile):
// the project bus, the consumers' queue URLs and the alerts topic. A value whose
// resource does not exist is "" (or {} for the map), so custom Terraform that
// reads it still validates.
func eventsBridgeBlock(envMap map[string]interface{}) string {
	busEnabled, _ := eventBusEnabled(envMap)
	consumers, _ := envMap[eventConsumersKey].([]interface{})

	block := "\n    # Events (event_bus.enabled, and the workloads that subscribe)\n"
	if busEnabled {
		block += "    event_bus_name = module.events.bus_name\n"
		block += "    event_bus_arn  = module.events.bus_arn\n"
	} else {
		block += "    event_bus_name = \"\"\n"
		block += "    event_bus_arn  = \"\"\n"
	}
	if len(consumers) > 0 {
		block += "    event_queue_urls = module.events.queue_urls\n"
		block += "    alerts_topic_arn = module.events.alerts_topic_arn\n"
	} else {
		block += "    event_queue_urls = {}\n"
		block += "    alerts_topic_arn = \"\"\n"
	}
	return block
}
