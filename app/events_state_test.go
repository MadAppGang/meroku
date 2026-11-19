package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A synthetic state document: the enhanced bus, one meroku FIFO subscriber,
// one custom awscc subscriber, one meroku-shaped subscriber on another bus,
// and the FIFO queue of orders.
const eventsStateFixture = `{
  "version": 4,
  "resources": [
    {"module": "module.event_bus_enhanced", "mode": "managed", "type": "awscc_eventsv2_event_bus", "name": "project",
     "instances": [{"attributes": {"event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/b-1"}}]},
    {"module": "module.event_fifo_subscriber_orders", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this",
     "instances": [{"attributes": {"name": "myapp-events-orders-fifo", "event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/b-1"}}]},
    {"module": "module.audit_feed", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this",
     "instances": [{"attributes": {"name": "audit-feed"}}]},
    {"module": "module.event_subscriber_billing_t", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this",
     "instances": [{"attributes": {"name": "other-t", "event_bus_arn": "arn:aws:events:ap-southeast-2:000000000000:event-busv2/other/b-2"}}]},
    {"module": "module.events", "mode": "managed", "type": "aws_sqs_queue", "name": "fifo_main",
     "instances": [{"index_key": "orders", "attributes": {}}]}
  ]
}`

func eventsStateDir(t *testing.T, lock string, initialised bool) string {
	t.Helper()
	dir := t.TempDir()
	if lock != "" {
		if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(lock), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if initialised {
		if err := os.Mkdir(filepath.Join(dir, ".terraform"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func stubEventsReaders(t *testing.T, pull func() ([]byte, error), remote func(Env) ([]byte, error)) (pulls, remotes *int) {
	t.Helper()
	savedPull, savedRemote := eventsStatePuller, eventsRemoteStateReader
	t.Cleanup(func() { eventsStatePuller, eventsRemoteStateReader = savedPull, savedRemote })
	pulls, remotes = new(int), new(int)
	eventsStatePuller = func(context.Context, string, string, string) ([]byte, error) {
		*pulls++
		return pull()
	}
	eventsRemoteStateReader = func(_ context.Context, e Env) ([]byte, error) {
		*remotes++
		return remote(e)
	}
	return pulls, remotes
}

const awsOnlyLock = `provider "registry.terraform.io/hashicorp/aws" {}` + "\n"

// The lock file is per machine and the state is shared: a lock without awscc
// says nothing about what another machine deployed.
func TestEventsAwsccStateForNeverReportsAnUnreadStateAsEmpty(t *testing.T) {
	e := Env{Region: "ap-southeast-2", AWSProfile: "none", StateBucket: "fake-bucket", StateFile: "state.tfstate"}
	fail := func(Env) ([]byte, error) { t.Fatal("the S3 backend was read"); return nil, nil }

	t.Run("initialised, lock without awscc: terraform state pull runs", func(t *testing.T) {
		pulls, _ := stubEventsReaders(t, func() ([]byte, error) { return []byte(eventsStateFixture), nil }, fail)
		st := eventsAwsccStateFor(eventsStateDir(t, awsOnlyLock, true), e, true)
		if *pulls != 1 || !st.Readable || !st.Pulled || !st.Bus || !reflect.DeepEqual(st.FifoQueues, []string{"orders"}) {
			t.Errorf("pulls=%d state=%+v, want one pull and the fixture's bus and FIFO queue", *pulls, st)
		}
	})

	t.Run("not needed, lock without awscc: nothing is read", func(t *testing.T) {
		pulls, remotes := stubEventsReaders(t, func() ([]byte, error) { return nil, errors.New("unexpected") }, fail)
		st := eventsAwsccStateFor(eventsStateDir(t, awsOnlyLock, true), e, false)
		if *pulls+*remotes != 0 || st.Readable || st.Tried {
			t.Errorf("pulls=%d remotes=%d state=%+v, want no read and an unread state", *pulls, *remotes, st)
		}
	})

	t.Run("not initialised: the S3 backend is read directly", func(t *testing.T) {
		_, remotes := stubEventsReaders(t, func() ([]byte, error) { t.Fatal("terraform ran"); return nil, nil },
			func(got Env) ([]byte, error) {
				if got.StateBucket != "fake-bucket" || got.StateFile != "state.tfstate" || got.AWSProfile != "none" {
					t.Errorf("S3 read with %+v", got)
				}
				return []byte(eventsStateFixture), nil
			})
		st := eventsAwsccStateFor(eventsStateDir(t, "", false), e, true)
		if *remotes != 1 || !st.Pulled || !reflect.DeepEqual(st.FifoQueues, []string{"orders"}) {
			t.Errorf("remotes=%d state=%+v", *remotes, st)
		}
	})

	t.Run("not initialised, no state object: verified empty", func(t *testing.T) {
		stubEventsReaders(t, nil, func(Env) ([]byte, error) { return nil, errRemoteStateAbsent })
		st := eventsAwsccStateFor(eventsStateDir(t, awsOnlyLock, false), e, true)
		if !st.Readable || !st.Pulled || st.unknown() {
			t.Errorf("state=%+v, want readable and empty", st)
		}
	})

	t.Run("a failed read is unknown and keeps Terraform's cause", func(t *testing.T) {
		stubEventsReaders(t, func() ([]byte, error) {
			return nil, errors.New("terraform state pull: exit status 1\n╷\n│ Error: the SSO session has expired\n│ more")
		}, fail)
		st := eventsAwsccStateFor(eventsStateDir(t, awsOnlyLock, true), e, true)
		st.Needed = true
		if st.Readable || !st.Tried || !st.unknown() {
			t.Errorf("state=%+v, want an unknown state", st)
		}
		if st.Err != "terraform state pull: exit status 1: Error: the SSO session has expired" {
			t.Errorf("Err = %q", st.Err)
		}
	})
}

// Only the addresses env/main.hbs renders, on the project bus, are meroku's
// subscribers.
func TestParseEventsAwsccStateKeepsOnlyMerokuSubscribers(t *testing.T) {
	st, err := parseEventsAwsccState([]byte(eventsStateFixture), "myapp-dev")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"event_fifo_subscriber_orders": "myapp-events-orders-fifo"}
	if !reflect.DeepEqual(st.Subscribers, want) {
		t.Errorf("Subscribers = %v, want %v", st.Subscribers, want)
	}
	// A classic env whose state holds only a custom subscriber stays off.
	custom := `{"version": 4, "resources": [{"module": "module.audit_feed", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this", "instances": [{"attributes": {"name": "audit-feed"}}]}]}`
	st, err = parseEventsAwsccState([]byte(custom), "myapp-dev")
	if err != nil {
		t.Fatal(err)
	}
	d := eventDeployed{renderPath: true, state: st}
	if mode := d.awsccMode(eventBusSettings{Enabled: true}); mode != awsccModeOff {
		t.Errorf("mode = %s, want off", mode)
	}
}

// revokeState is a readable state with the given sweep, record and RAM keys.
func revokeState(sweep, record *eventsRevokeRecord, ram ...string) eventDeployed {
	s := emptyEventsState()
	s.Revoke, s.RevokeRecord, s.RamPrincipals = sweep, record, ram
	return eventDeployed{renderPath: true, state: s}
}

func revokeLost(revoke map[string]interface{}) []string {
	out := []string{}
	for _, p := range revoke["lost"].([]interface{}) {
		out = append(out, p.(string))
	}
	return out
}

func TestEventBusRevokeInputsFromState(t *testing.T) {
	acct1, acct2 := "account:000000000001", "account:000000000002"
	share := []eventShareEntry{{Kind: eventShareKindAccount, ID: "000000000001", Access: eventShareAccessSubscribe}}
	settled := func(granted, lost []string, round int) (*eventsRevokeRecord, *eventsRevokeRecord) {
		return &eventsRevokeRecord{Granted: granted, Lost: lost, Round: round},
			&eventsRevokeRecord{Granted: granted, Lost: lost, Round: round}
	}

	t.Run("an association no step recorded is owed", func(t *testing.T) {
		sweep, rec := settled([]string{acct1}, nil, 3)
		revoke, owed := eventBusRevokeInputs(share, revokeState(sweep, rec,
			"subscribe/account/000000000001", "subscribe/account/000000000002"))
		if want := []string{acct2}; !reflect.DeepEqual(owed, want) || !reflect.DeepEqual(revokeLost(revoke), want) || revoke["round"] != 4 {
			t.Errorf("owed=%v lost=%v round=%v, want %v in round 4", owed, revokeLost(revoke), revoke["round"], want)
		}
	})

	t.Run("a settled bus keeps the recorded lost and round", func(t *testing.T) {
		sweep, rec := settled([]string{acct1}, []string{acct2}, 4)
		revoke, owed := eventBusRevokeInputs(share, revokeState(sweep, rec, "subscribe/account/000000000001"))
		if len(owed) != 0 || !reflect.DeepEqual(revokeLost(revoke), []string{acct2}) || revoke["round"] != 4 {
			t.Errorf("owed=%v lost=%v round=%v, want nothing owed, the recorded lost and round 4", owed, revokeLost(revoke), revoke["round"])
		}
	})

	t.Run("a failed sweep still owes what it was given", func(t *testing.T) {
		_, rec := settled([]string{acct1, acct2}, nil, 4)
		sweep := &eventsRevokeRecord{Granted: []string{acct1}, Lost: []string{acct2}, Round: 5, Tainted: true}
		revoke, owed := eventBusRevokeInputs(share, revokeState(sweep, rec, "subscribe/account/000000000001"))
		if !reflect.DeepEqual(owed, []string{acct2}) || revoke["round"] != 6 {
			t.Errorf("owed=%v round=%v after a failed sweep, want %s in round 6", owed, revoke["round"], acct2)
		}
	})

	// acct2 was revoked in round 2. An apply re-shared it, associated it in
	// RAM and stopped before its sweep; the YAML is then reverted. granted
	// and lost read exactly as round 2's, but acct2 holds a grant again, and
	// may have subscribed while it did.
	t.Run("a regrant rolled back before its sweep is new debt", func(t *testing.T) {
		sweep, rec := settled([]string{acct1}, []string{acct2}, 2)
		revoke, owed := eventBusRevokeInputs(share, revokeState(sweep, rec,
			"subscribe/account/000000000001", "subscribe/account/000000000002"))
		if !reflect.DeepEqual(owed, []string{acct2}) || !reflect.DeepEqual(revokeLost(revoke), []string{acct2}) {
			t.Errorf("owed=%v lost=%v, want %s owed again", owed, revokeLost(revoke), acct2)
		}
		if revoke["round"] != 3 {
			t.Errorf("round=%v, want 3: the same granted and lost as round 2 must still replace the sweep", revoke["round"])
		}
	})

	// The apply that removed acct2 destroyed the old sweep and the RAM
	// association, then stopped before the new sweep. Only the record is left.
	t.Run("an apply interrupted between disassociation and the sweep", func(t *testing.T) {
		_, rec := settled([]string{acct1, acct2}, nil, 7)
		revoke, owed := eventBusRevokeInputs(share, revokeState(nil, rec, "subscribe/account/000000000001"))
		if !reflect.DeepEqual(owed, []string{acct2}) || revoke["round"] != 8 || revoke["enabled"] != true {
			t.Errorf("owed=%v round=%v enabled=%v, want %s owed in round 8", owed, revoke["round"], revoke["enabled"], acct2)
		}
	})

	// The sweep of round 8 succeeded and the apply stopped before the record
	// was updated: the record does not confirm round 8, so it runs again.
	t.Run("a sweep the record does not confirm runs again", func(t *testing.T) {
		_, rec := settled([]string{acct1, acct2}, nil, 7)
		sweep := &eventsRevokeRecord{Granted: []string{acct1}, Lost: []string{acct2}, Round: 8}
		revoke, owed := eventBusRevokeInputs(share, revokeState(sweep, rec, "subscribe/account/000000000001"))
		if !reflect.DeepEqual(owed, []string{acct2}) || revoke["round"] != 9 {
			t.Errorf("owed=%v round=%v, want %s owed in round 9", owed, revoke["round"], acct2)
		}
	})
}

// A fresh checkout has no env/<env>/main.tf and no lock file: the S3 state is
// the only record of what another machine deployed.
func TestEventsStateRequired(t *testing.T) {
	backend := Env{StateBucket: "fake-bucket", StateFile: "state.tfstate"}
	cases := []struct {
		name                    string
		yamlOrMarkers, rendered bool
		lockAwscc               bool
		e                       Env
		want                    bool
	}{
		{"YAML or markers ask for it", true, true, false, backend, true},
		{"the lock names awscc", false, true, true, backend, true},
		{"no previous render, S3 backend", false, false, false, backend, true},
		{"no previous render, no backend", false, false, false, Env{}, false},
		{"rendered here, no markers, no awscc", false, true, false, backend, false},
	}
	for _, tc := range cases {
		if got := eventsStateRequired(tc.yamlOrMarkers, tc.rendered, tc.lockAwscc, tc.e); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A lock that names awscc and no main.tf: the state is needed, so a failed
// read is unknown and the render is refused, never taken as nothing deployed.
func TestPrepareEventStateProbeLockWithoutMainTf(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join("env", "dev", ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	lock := `provider ` + awsccLockProvider + ` {}` + "\n"
	if err := os.WriteFile(filepath.Join("env", "dev", ".terraform.lock.hcl"), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	stubEventsReaders(t, func() ([]byte, error) { return nil, errors.New("terraform state pull: exit status 1") },
		func(Env) ([]byte, error) { t.Fatal("the S3 backend was read"); return nil, nil })
	envMap := map[string]interface{}{"project": "myapp", "env": "dev", "region": "ap-southeast-2"}
	prepareEventStateProbe(envMap, "dev")
	st := envMap[eventsAwsccStateKey].(eventsAwsccState)
	if !st.Needed || !st.unknown() || !st.NoRender {
		t.Errorf("state=%+v, want needed, unknown and no previous render", st)
	}
}

func TestEventsStateObjectKey(t *testing.T) {
	for _, tc := range []struct{ workspace, want string }{
		{"", "state.tfstate"},
		{"default", "state.tfstate"},
		{"blue", "env:/blue/state.tfstate"},
	} {
		if got := eventsStateObjectKey("state.tfstate", tc.workspace); got != tc.want {
			t.Errorf("workspace %q: key %q, want %q", tc.workspace, got, tc.want)
		}
	}
}

// Ownership takes the address grammar env/main.hbs renders and the project
// bus: a module name alone is no evidence.
func TestParseEventsAwsccStateSubscriberOwnership(t *testing.T) {
	sub := func(module, name, arn string) string {
		return `{"module": "module.` + module + `", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this",
     "instances": [{"attributes": {"name": "` + name + `", "event_bus_arn": "` + arn + `"}}]}`
	}
	doc := func(resources ...string) []byte {
		return []byte(`{"version": 4, "resources": [` + strings.Join(resources, ",") + `]}`)
	}
	projectBus := "arn:aws:events:ap-southeast-2:000000000000:event-busv2/myapp-dev/b-1"
	otherBus := "arn:aws:events:ap-southeast-2:000000000000:event-busv2/audit/b-9"

	st, err := parseEventsAwsccState(doc(
		sub("event_fifo_subscriber_orders", "myapp-events-orders-fifo", projectBus),
		sub("event_subscriber_billing_t", "myapp-events-billing.t", projectBus),
		sub("event_fifo_subscriber_orders_x", "custom", projectBus),
		sub("event_subscriber_billing_t_x", "custom", projectBus),
		sub("event_subscriber_Billing_t", "custom", projectBus),
		sub("event_subscriber_billing_nobus", "custom", ""),
	), "myapp-dev")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"event_fifo_subscriber_orders": "myapp-events-orders-fifo",
		"event_subscriber_billing_t":   "myapp-events-billing.t",
	}
	if !reflect.DeepEqual(st.Subscribers, want) {
		t.Errorf("project bus derived from its name: Subscribers = %v, want %v", st.Subscribers, want)
	}

	// A classic env with a custom module at a generated-looking address on
	// its own bus, and no meroku bus: nothing is meroku's, the env stays off.
	st, err = parseEventsAwsccState(doc(sub("event_subscriber_audit_feed", "audit-feed", otherBus)), "myapp-dev")
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Subscribers) != 0 {
		t.Errorf("Subscribers = %v, want none", st.Subscribers)
	}
	d := eventDeployed{renderPath: true, state: st}
	if mode := d.awsccMode(eventBusSettings{Enabled: true}); mode != awsccModeOff {
		t.Errorf("mode = %s, want off", mode)
	}
}

// A file whose stem differs from the env it declares is not its own sibling.
func TestClassifyShareTargetsSkipsTheDeclaredEnv(t *testing.T) {
	probes := []siblingProbe{
		{File: "dev.yaml", Probe: vpcCIDRProbe{Project: "myapp", Env: "dev", AccountID: "000000000001"}},
		{File: "prod.yaml", Probe: vpcCIDRProbe{Project: "myapp", Env: "prod", AccountID: "000000000002"}},
	}
	targets := classifyShareTargets(probes, "dev-copy", "dev", "myapp", "000000000000")
	var envs []string
	for _, tgt := range targets {
		envs = append(envs, tgt.Env)
	}
	if strings.Join(envs, ",") != "prod" {
		t.Errorf("targets = %v, want only prod", envs)
	}
}
