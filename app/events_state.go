package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// What the Terraform state holds of the events resources, for the render
// that writes main.tf.
//
// The state is shared (S3) while env/<env>/.terraform and the lock file are
// per machine, so neither local file can say what is deployed. The probe
// either reads the state document itself, or says it could not: an unread
// state is never reported as an empty one.

// eventsAwsccState is what the env's Terraform state holds of the events
// resources. Nothing else of the state is kept.
type eventsAwsccState struct {
	Readable      bool                // the state document was read (a verified absent state is readable and empty)
	Pulled        bool                // the state was read and parsed, so its answers are exact
	Needed        bool                // this render depends on the state (eventsStateRequired)
	NoRender      bool                // this machine has no previous main.tf of the env to go by
	Tried         bool                // a read was attempted; with Readable false the state may hold anything
	Err           string              // why the state could not be read
	Bus           bool                // module.event_bus_enhanced.awscc_eventsv2_event_bus.project[0]
	BusARN        string              // its event_bus_arn; "" when absent
	ClassicBus    bool                // module.events.aws_cloudwatch_event_bus.project[0]
	Revoke        *eventsRevokeRecord // module.event_bus_enhanced.terraform_data.revoke[0], the sweep; nil if absent
	RevokeRecord  *eventsRevokeRecord // module.event_bus_enhanced.terraform_data.revoke_record[0]; nil if absent
	Guard         bool                // module.event_bus_enhanced.terraform_data.bus_guard[0]
	RamPrincipals []string            // keys of aws_ram_principal_association.bus: "<access>/<kind>/<id>"
	Subscribers   map[string]string   // meroku subscriber module name -> attributes.name of its awscc_eventsv2_subscriber.this[0]
	FifoQueues    []string            // consumers with module.events.aws_sqs_queue.fifo_main["<c>"]
	Consumers     []string            // consumers with module.events.aws_sqs_queue.main["<c>"]
}

// unknown reports whether this render depends on the state and could not
// read it.
func (s eventsAwsccState) unknown() bool { return s.Needed && !s.Readable }

// eventsRevokeRecord is the input of a revoke step as applied: the sweep
// (terraform_data.revoke), or the record written after it succeeded
// (terraform_data.revoke_record).
type eventsRevokeRecord struct {
	Granted []string // input.granted: the subscribe-capable principals it was applied with
	Lost    []string // input.lost: what it had to revoke
	Round   int      // input.round: which revocation round it belongs to
	Judged  []string // input.judged: the other accounts' subscriber ARNs its sweep was given to judge
	Tainted bool     // its script failed (the sweep only)
}

// eventsStatePullTimeout bounds the state read.
const eventsStatePullTimeout = 2 * time.Minute

// awsccLockProvider is the lock-file entry of the awscc provider.
const awsccLockProvider = `"registry.terraform.io/hashicorp/awscc"`

// eventsStatePuller runs `terraform state pull` in dir. A variable so tests
// stub Terraform.
var eventsStatePuller = pullTerraformState

// pullTerraformState is the real eventsStatePuller.
func pullTerraformState(ctx context.Context, dir, profile, region string) ([]byte, error) {
	cmd, err := eventsStatePullCmd(ctx, dir, profile, region)
	if err != nil {
		return nil, err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Enough lines to keep the "Error:" line of a diagnostic with its
		// source excerpt and detail.
		if tail := strings.TrimSpace(lastLines(stderr.String(), 20)); tail != "" {
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
	cmd, err := newTerraformCmd(ctx, tfState, dir, "state", "pull", "-no-color")
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

// eventsRemoteStateReader reads the state document straight from the S3
// backend (state_bucket, state_file, region of the env YAML), with the YAML's
// profile pinned. It is the read for an env directory that terraform init has
// not connected yet: a fresh checkout, or a first deploy. errRemoteStateAbsent
// means the bucket or the object does not exist. A variable so tests stub S3.
var eventsRemoteStateReader = func(ctx context.Context, e Env) ([]byte, error) {
	if e.StateBucket == "" || e.StateFile == "" || e.Region == "" {
		return nil, errors.New("the env YAML sets no state_bucket, state_file and region, so the state cannot be found")
	}
	opts := []func(*config.LoadOptions) error{config.WithRegion(e.Region)}
	if e.AWSProfile != "" {
		opts = append(opts, config.WithSharedConfigProfile(e.AWSProfile))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("loading the AWS configuration: %w", err)
	}
	key := eventsStateObjectKey(e.StateFile, os.Getenv("TF_WORKSPACE"))
	out, err := s3.NewFromConfig(cfg).GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(e.StateBucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isRemoteStateAbsentError(err) {
			return nil, errRemoteStateAbsent
		}
		return nil, fmt.Errorf("reading s3://%s/%s: %w", e.StateBucket, key, err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(io.LimitReader(out.Body, maxRemoteStateBytes))
	if err != nil {
		return nil, fmt.Errorf("reading s3://%s/%s: %w", e.StateBucket, key, err)
	}
	return data, nil
}

// s3WorkspaceKeyPrefix is the S3 backend's default workspace_key_prefix;
// env/main.hbs sets none.
const s3WorkspaceKeyPrefix = "env:"

// eventsStateObjectKey is the object the S3 backend keeps a workspace's state
// in: the configured key for the default workspace, else
// <workspace_key_prefix>/<workspace>/<key>. An uninitialised directory has no
// .terraform/environment, so only TF_WORKSPACE can select a workspace there,
// as it does for Terraform.
func eventsStateObjectKey(stateFile, workspace string) string {
	if workspace == "" || workspace == "default" {
		return stateFile
	}
	return s3WorkspaceKeyPrefix + "/" + workspace + "/" + stateFile
}

// eventsAwsccStateFor reads envDir's state when needed says the render
// depends on it (eventsStateRequired); otherwise nothing is read, so classic
// generate stays offline. The read is `terraform state pull` when envDir is
// initialised, otherwise the S3 backend directly. A read that fails comes back
// with Tried set and Readable false, never as an empty state. A variable so
// tests stub it. ctx is the caller's: a web request that is cancelled stops
// its read.
var eventsAwsccStateFor = func(ctx context.Context, envDir string, e Env, needed bool) eventsAwsccState {
	if !needed {
		return eventsAwsccState{}
	}
	ctx, cancel := context.WithTimeout(ctx, eventsStatePullTimeout)
	defer cancel()

	var out []byte
	var err error
	if _, statErr := os.Stat(filepath.Join(envDir, ".terraform")); statErr == nil {
		out, err = eventsStatePuller(ctx, envDir, e.AWSProfile, e.Region)
		if isNoStateFileError(err) {
			return eventsAwsccState{Readable: true, Pulled: true, Tried: true, Subscribers: map[string]string{}}
		}
	} else {
		out, err = eventsRemoteStateReader(ctx, e)
		if errors.Is(err, errRemoteStateAbsent) {
			return eventsAwsccState{Readable: true, Pulled: true, Tried: true, Subscribers: map[string]string{}}
		}
	}
	if err != nil {
		return eventsAwsccState{Tried: true, Err: eventsStateErrorLine(err)}
	}
	st, err := parseEventsAwsccState(out, projectBusID{Name: e.Project + "-" + e.Env, Region: e.Region, Account: e.AccountID})
	if err != nil {
		return eventsAwsccState{Tried: true, Err: err.Error()}
	}
	st.Tried = true
	return st
}

// ansiEscapePattern matches the colour and style sequences Terraform writes
// when -no-color is not in effect.
var ansiEscapePattern = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// eventsStateErrorLine is a failed read's error on one line: the error, and
// the line of what Terraform printed that names the cause (expired SSO,
// access denied, module not installed): its "Error:" line, or else its first
// line with text. Colour codes and the diagnostic box are removed.
func eventsStateErrorLine(err error) string {
	first, rest, _ := strings.Cut(err.Error(), "\n")
	cause := ""
	for _, line := range strings.Split(ansiEscapePattern.ReplaceAllString(rest, ""), "\n") {
		line = strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│╷╵"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "Error:") {
			cause = line
			break
		}
		if cause == "" {
			cause = line
		}
	}
	if cause == "" {
		return first
	}
	return first + ": " + cause
}

// terraformInitCausePattern matches the Terraform errors of a working
// directory that terraform init has not prepared for its configuration.
var terraformInitCausePattern = regexp.MustCompile(`Module not installed|Backend initialization required|Required plugins are not installed|Inconsistent dependency lock file|Plugin reinitialization required|Module source has changed|Module version requirements have changed|Backend configuration changed|terraform init`)

// eventsStateNeedsInit reports whether a failed read failed because the env
// directory is not initialised for its current main.tf.
func eventsStateNeedsInit(cause string) bool {
	return terraformInitCausePattern.MatchString(cause)
}

// eventsStateNeeded reports whether this render depends on what the state
// holds: the YAML asks for the enhanced bus, the previous main.tf has an
// events marker (it was enhanced, in cleanup, shared, or kept a FIFO set), or
// a workload keeps a FIFO set with fifo_retired.
func eventsStateNeeded(envMap map[string]interface{}) bool {
	if block, ok := yamlSubMap(envMap, "event_bus"); ok {
		if t, _ := block["type"].(string); t == eventBusTypeEnhanced {
			return true
		}
	}
	if prev, ok := envMap[eventPreviousKey].(eventPrevious); ok && prev.any() {
		return true
	}
	retired := func(owner map[string]interface{}) bool {
		events, _ := yamlSubMap(owner, "events")
		queue, _ := yamlSubMap(events, "queue")
		b, _ := queue["fifo_retired"].(bool)
		return b
	}
	if w, ok := yamlSubMap(envMap, "workload"); ok && retired(w) {
		return true
	}
	items, _ := envMap["services"].([]interface{})
	for _, raw := range items {
		if item, ok := computeNodeMap(raw); ok && retired(item) {
			return true
		}
	}
	return false
}

// prepareEventStateProbe reads the env's Terraform state for normalizeEvents.
// Generate and the deployed-events endpoint call it, after
// prepareEventInputs, with the context the read runs under.
func prepareEventStateProbe(ctx context.Context, envMap map[string]interface{}, env string) {
	e := Env{
		Project:     stringFromMap(envMap, "project"),
		Env:         stringFromMap(envMap, "env"),
		Region:      stringFromMap(envMap, "region"),
		AccountID:   stringFromMap(envMap, "account_id"),
		AWSProfile:  stringFromMap(envMap, "aws_profile"),
		StateBucket: stringFromMap(envMap, "state_bucket"),
		StateFile:   stringFromMap(envMap, "state_file"),
	}
	envDir := filepath.Join("env", env)
	_, rendered := envMap[eventPreviousKey].(eventPrevious)
	needed := eventsStateRequired(eventsStateNeeded(envMap), rendered, lockNamesAwscc(envDir), e)
	st := eventsAwsccStateFor(ctx, envDir, e, needed)
	st.Needed = needed
	st.NoRender = !rendered
	envMap[eventsAwsccStateKey] = st
}

// eventsStateRequired decides whether a render that writes main.tf reads the
// state. yamlOrMarkers is eventsStateNeeded. Besides that, the state decides
// when this machine has no previous render to go by (no env/<env>/main.tf, or
// none that can be read) and the env has an S3 backend, which is every fresh
// checkout: env/<env>/ is build output, so another machine may have deployed
// the enhanced bus from the same state. And it decides when the lock file
// names awscc, which only an awscc render writes. Once this machine has
// rendered the env, a main.tf without events markers is its record that the
// last render had nothing of the enhanced bus, and generate stays offline.
func eventsStateRequired(yamlOrMarkers, rendered, lockAwscc bool, e Env) bool {
	switch {
	case yamlOrMarkers, lockAwscc:
		return true
	case !rendered:
		return e.StateBucket != "" && e.StateFile != ""
	}
	return false
}

// lockNamesAwscc reports whether envDir's lock file has the awscc provider.
func lockNamesAwscc(envDir string) bool {
	lock, err := os.ReadFile(filepath.Join(envDir, ".terraform.lock.hcl"))
	return err == nil && bytes.Contains(lock, []byte(awsccLockProvider))
}

// errEventsStateUnreadable marks a generate refused because the events state
// could not be read. The deploy retries once after its AWS pre-flight, which
// refreshes an expired SSO session.
var errEventsStateUnreadable = errors.New("the Terraform state of the events resources could not be read")

// eventsStateUnreadableError wraps a refusal that included the unreadable
// state, so errors.Is finds errEventsStateUnreadable and the message stays
// the refusal's own.
type eventsStateUnreadableError struct{ err error }

func (e eventsStateUnreadableError) Error() string { return e.err.Error() }
func (e eventsStateUnreadableError) Unwrap() []error {
	return []error{e.err, errEventsStateUnreadable}
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

// projectBusID is what the env YAML says of the project bus: its name,
// <project>-<env>, and the region and account it lives in ("" when the YAML
// sets none).
type projectBusID struct {
	Name, Region, Account string
}

// parseEventsAwsccState reads the events addresses from a state document.
// Empty output is an empty state. A subscriber counts as meroku's only with
// two pieces of evidence: its module address is one env/main.hbs can render
// (parseEventSubscriberModule), and its event_bus_arn is the project bus's,
// the one in state or, when the state holds none, an EventBridge V2 bus with
// the project bus's name, region and account. A custom awscc subscriber is
// none of meroku's business, whatever its module is called.
func parseEventsAwsccState(out []byte, bus projectBusID) (eventsAwsccState, error) {
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
	busARN := ""
	subscriberBus, subscriberName := map[string]string{}, map[string]string{}
	for _, r := range doc.Resources {
		if r.Mode != "managed" || len(r.Instances) == 0 {
			continue
		}
		switch {
		case r.Module == "module.event_bus_enhanced" && r.Type == "awscc_eventsv2_event_bus" && r.Name == "project":
			st.Bus = true
			busARN, _ = r.Instances[0].Attributes["event_bus_arn"].(string)
			st.BusARN = busARN
		case r.Module == "module.events" && r.Type == "aws_cloudwatch_event_bus" && r.Name == "project":
			st.ClassicBus = true
		case r.Module == "module.event_bus_enhanced" && r.Type == "terraform_data" && (r.Name == "revoke" || r.Name == "revoke_record"):
			inst := r.Instances[0]
			input := terraformDataInput(inst.Attributes["input"])
			rec := &eventsRevokeRecord{
				Granted: stringsOf(input["granted"]),
				Lost:    stringsOf(input["lost"]),
				Round:   intOf(input["round"]),
				Judged:  stringsOf(input["judged"]),
				Tainted: inst.Status == "tainted",
			}
			if r.Name == "revoke" {
				st.Revoke = rec
			} else {
				st.RevokeRecord = rec
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
			if _, _, _, ours := parseEventSubscriberModule(module); !ours {
				continue
			}
			subscriberName[module], _ = r.Instances[0].Attributes["name"].(string)
			subscriberBus[module], _ = r.Instances[0].Attributes["event_bus_arn"].(string)
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
	for module, arn := range subscriberBus {
		if onProjectBus(arn, busARN, bus) {
			st.Subscribers[module] = subscriberName[module]
		}
	}
	sort.Strings(st.RamPrincipals)
	sort.Strings(st.FifoQueues)
	sort.Strings(st.Consumers)
	return st, nil
}

// onProjectBus reports whether a subscriber's event_bus_arn is the project
// bus: busARN when the state holds the bus, else a V2 bus with bus's name
// and, where the YAML sets them, its region and account.
func onProjectBus(arn, busARN string, bus projectBusID) bool {
	switch {
	case arn == "":
		return false
	case busARN != "":
		return arn == busARN
	}
	name, ok := eventBusV2Name(arn)
	if !ok || bus.Name == "" || name != bus.Name {
		return false
	}
	parts := strings.SplitN(arn, ":", 6)
	return (bus.Region == "" || parts[3] == bus.Region) && (bus.Account == "" || parts[4] == bus.Account)
}

// eventBusV2Name is the bus name of an EventBridge V2 bus ARN,
// arn:<partition>:events:<region>:<account>:event-busv2/<name>/<id>.
func eventBusV2Name(arn string) (string, bool) {
	parts := strings.SplitN(arn, ":", 6)
	if len(parts) != 6 || parts[0] != "arn" || parts[2] != "events" {
		return "", false
	}
	resource := strings.Split(parts[5], "/")
	if len(resource) != 3 || resource[0] != "event-busv2" || resource[1] == "" {
		return "", false
	}
	return resource[1], true
}

// intOf reads a state number (JSON float64); anything else is 0.
func intOf(raw interface{}) int {
	f, _ := raw.(float64)
	return int(f)
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
