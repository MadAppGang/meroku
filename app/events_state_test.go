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
	st, err := parseEventsAwsccState([]byte(eventsStateFixture))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"event_fifo_subscriber_orders": "myapp-events-orders-fifo"}
	if !reflect.DeepEqual(st.Subscribers, want) {
		t.Errorf("Subscribers = %v, want %v", st.Subscribers, want)
	}
	// A classic env whose state holds only a custom subscriber stays off.
	custom := `{"version": 4, "resources": [{"module": "module.audit_feed", "mode": "managed", "type": "awscc_eventsv2_subscriber", "name": "this", "instances": [{"attributes": {"name": "audit-feed"}}]}]}`
	st, err = parseEventsAwsccState([]byte(custom))
	if err != nil {
		t.Fatal(err)
	}
	d := eventDeployed{renderPath: true, state: st}
	if mode := d.awsccMode(eventBusSettings{Enabled: true}); mode != awsccModeOff {
		t.Errorf("mode = %s, want off", mode)
	}
}

func TestEventBusRevokeInputsFromState(t *testing.T) {
	share := []eventShareEntry{{Kind: eventShareKindAccount, ID: "000000000001", Access: eventShareAccessSubscribe}}
	stateWith := func(rec *eventsRevokeRecord, ram ...string) eventDeployed {
		s := emptyEventsState()
		s.Revoke, s.RamPrincipals = rec, ram
		return eventDeployed{renderPath: true, state: s}
	}
	lostOf := func(revoke map[string]interface{}) []string {
		out := []string{}
		for _, p := range revoke["lost"].([]interface{}) {
			out = append(out, p.(string))
		}
		return out
	}

	// An interrupted apply associated 000000000002 but never recorded it.
	revoke, owed := eventBusRevokeInputs(share, stateWith(
		&eventsRevokeRecord{Granted: []string{"account:000000000001"}},
		"subscribe/account/000000000001", "subscribe/account/000000000002"))
	if want := []string{"account:000000000002"}; !reflect.DeepEqual(owed, want) || !reflect.DeepEqual(lostOf(revoke), want) {
		t.Errorf("owed=%v lost=%v, want both %v", owed, lostOf(revoke), want)
	}

	// After the apply that revoked it, nothing is owed and the input is the
	// record's, so the next plan is empty.
	revoke, owed = eventBusRevokeInputs(share, stateWith(
		&eventsRevokeRecord{Granted: []string{"account:000000000001"}, Lost: []string{"account:000000000002"}},
		"subscribe/account/000000000001"))
	if len(owed) != 0 || !reflect.DeepEqual(lostOf(revoke), []string{"account:000000000002"}) {
		t.Errorf("owed=%v lost=%v, want nothing owed and the recorded lost", owed, lostOf(revoke))
	}

	// A failed step still owes what it recorded.
	_, owed = eventBusRevokeInputs(share, stateWith(
		&eventsRevokeRecord{Granted: []string{"account:000000000001"}, Lost: []string{"account:000000000002"}, Tainted: true},
		"subscribe/account/000000000001"))
	if !reflect.DeepEqual(owed, []string{"account:000000000002"}) {
		t.Errorf("owed=%v after a failed step", owed)
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
