package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// stubEventBusSubscribers makes the subscriber listing answer with subs (or
// err) for the rest of the test, and counts the calls.
func stubEventBusSubscribers(t *testing.T, subs func() ([]eventBusSubscriber, error)) *int {
	t.Helper()
	calls := 0
	saved := eventBusSubscribersLister
	t.Cleanup(func() { eventBusSubscribersLister = saved })
	eventBusSubscribersLister = func(_ context.Context, _ Env, busARN string) ([]eventBusSubscriber, error) {
		calls++
		if busARN != fakeBusARN {
			t.Errorf("listed bus %q, want %q", busARN, fakeBusARN)
		}
		return subs()
	}
	return &calls
}

const (
	foreignSub1 = "arn:aws:events:ap-southeast-2:000000000001:subscriber/feed/0001"
	foreignSub3 = "arn:aws:events:ap-southeast-2:000000000003:subscriber/audit/0003"
)

func foreignListing(accounts ...string) eventForeignSubscribers {
	f := eventForeignSubscribers{Needed: true, Read: true}
	for _, a := range accounts {
		f.Subscribers = append(f.Subscribers, eventForeignSubscriber{
			ARN: "arn:aws:events:ap-southeast-2:" + a + ":subscriber/s/" + a, Account: a,
		})
	}
	return f
}

func TestEventBusRevokeInputsFromSubscriberListing(t *testing.T) {
	acct1, acct3 := "account:000000000001", "account:000000000003"
	publish := []eventShareEntry{{Kind: eventShareKindAccount, ID: "000000000002", Access: eventShareAccessPublish}}
	settled := func(granted, lost []string, round int) (*eventsRevokeRecord, *eventsRevokeRecord) {
		return &eventsRevokeRecord{Granted: granted, Lost: lost, Round: round},
			&eventsRevokeRecord{Granted: granted, Lost: lost, Round: round}
	}
	withListing := func(d eventDeployed, f eventForeignSubscribers) eventDeployed {
		d.foreign = f
		return d
	}

	// Every input in state reads as settled, and none names account 1: the
	// grant and its rollback each stopped before their sweep.
	t.Run("a subscriber no record names is owed", func(t *testing.T) {
		sweep, rec := settled(nil, nil, 5)
		revoke, owed, foreign := eventBusRevokeInputs(publish, withListing(revokeState(sweep, rec), foreignListing("000000000001")))
		if want := []string{acct1}; !reflect.DeepEqual(owed, want) || !reflect.DeepEqual(revokeLost(revoke), want) ||
			!reflect.DeepEqual(foreign, want) || revoke["round"] != 6 {
			t.Errorf("owed=%v lost=%v foreign=%v round=%v, want %v owed in round 6", owed, revokeLost(revoke), foreign, revoke["round"], want)
		}
	})

	t.Run("without the listing the same state reads as settled", func(t *testing.T) {
		sweep, rec := settled(nil, nil, 5)
		revoke, owed, _ := eventBusRevokeInputs(publish, revokeState(sweep, rec))
		if len(owed) != 0 || revoke["round"] != 5 {
			t.Errorf("owed=%v round=%v, want nothing owed in round 5", owed, revoke["round"])
		}
	})

	t.Run("a revoked or own subscriber is not owed", func(t *testing.T) {
		sweep, rec := settled(nil, []string{acct1}, 6)
		revoke, owed, _ := eventBusRevokeInputs(publish, withListing(revokeState(sweep, rec), foreignListing()))
		if len(owed) != 0 || revoke["round"] != 6 || !reflect.DeepEqual(revokeLost(revoke), []string{acct1}) {
			t.Errorf("owed=%v lost=%v round=%v, want settled in round 6", owed, revokeLost(revoke), revoke["round"])
		}
	})

	t.Run("an account entry that may subscribe keeps its subscriber", func(t *testing.T) {
		share := []eventShareEntry{{Kind: eventShareKindAccount, ID: "000000000001", Access: eventShareAccessSubscribe}}
		sweep, rec := settled([]string{acct1}, nil, 2)
		revoke, owed, _ := eventBusRevokeInputs(share, withListing(revokeState(sweep, rec, "subscribe/account/000000000001"),
			foreignListing("000000000001")))
		if len(owed) != 0 || revoke["round"] != 2 {
			t.Errorf("owed=%v round=%v, want settled in round 2", owed, revoke["round"])
		}
	})

	// An OU entry: the sweep of round 3 was given account 3's subscriber,
	// found the account inside the OU and kept it. The record confirms the
	// round and names the subscriber, so it is not owed again and the plan
	// stays empty.
	t.Run("a subscriber a finished round kept is settled", func(t *testing.T) {
		share := []eventShareEntry{{Kind: eventShareKindOU, ID: "ou-ab12-cd34ef56", Access: eventShareAccessSubscribe}}
		granted := []string{"ou:ou-ab12-cd34ef56"}
		listing := foreignListing("000000000003")
		sweep, rec := settled(granted, []string{acct3}, 3)
		sweep.Judged, rec.Judged = []string{listing.Subscribers[0].ARN}, []string{listing.Subscribers[0].ARN}
		revoke, owed, _ := eventBusRevokeInputs(share, withListing(revokeState(sweep, rec, "subscribe/ou/ou-ab12-cd34ef56"), listing))
		if len(owed) != 0 || revoke["round"] != 3 || !reflect.DeepEqual(revokeLost(revoke), []string{acct3}) ||
			!reflect.DeepEqual(revokeJudged(revoke), rec.Judged) {
			t.Errorf("owed=%v lost=%v judged=%v round=%v, want settled in round 3 with %s kept in lost and judged",
				owed, revokeLost(revoke), revokeJudged(revoke), revoke["round"], acct3)
		}
	})

	// The same OU entry and a record whose round owed account 3 and kept its
	// first subscriber. A second subscriber of account 3 appears: no sweep
	// judged it, so the account being in the record's lost does not settle
	// the bus.
	t.Run("a new subscriber of a kept account starts a round", func(t *testing.T) {
		share := []eventShareEntry{{Kind: eventShareKindOU, ID: "ou-ab12-cd34ef56", Access: eventShareAccessSubscribe}}
		granted := []string{"ou:ou-ab12-cd34ef56"}
		listing := foreignListing("000000000003")
		kept := listing.Subscribers[0].ARN
		listing.Subscribers = append(listing.Subscribers, eventForeignSubscriber{ARN: foreignSub3, Account: "000000000003"})
		sweep, rec := settled(granted, []string{acct3}, 3)
		sweep.Judged, rec.Judged = []string{kept}, []string{kept}
		revoke, owed, _ := eventBusRevokeInputs(share, withListing(revokeState(sweep, rec, "subscribe/ou/ou-ab12-cd34ef56"), listing))
		if !reflect.DeepEqual(owed, []string{acct3}) || revoke["round"] != 4 ||
			!reflect.DeepEqual(revokeJudged(revoke), principalsNotIn([]string{kept, foreignSub3}, nil)) {
			t.Errorf("owed=%v judged=%v round=%v, want %s owed in round 4 judging both subscribers", owed, revokeJudged(revoke), revoke["round"], acct3)
		}
	})

	// Round 6 revoked account 1 (its first subscriber is gone from the
	// listing). Then a grant stopped before its sweep, account 1 created a
	// new subscriber, and the rollback stopped before its sweep: the state
	// reads as round 6 again, and account 1 is already in its lost.
	t.Run("a new subscriber of an account a past round revoked is owed", func(t *testing.T) {
		sweep, rec := settled(nil, []string{acct1}, 6)
		sweep.Judged, rec.Judged = []string{"arn:aws:events:ap-southeast-2:000000000001:subscriber/feed/old"},
			[]string{"arn:aws:events:ap-southeast-2:000000000001:subscriber/feed/old"}
		revoke, owed, _ := eventBusRevokeInputs(publish, withListing(revokeState(sweep, rec), foreignListing("000000000001")))
		if !reflect.DeepEqual(owed, []string{acct1}) || revoke["round"] != 7 || !reflect.DeepEqual(revokeLost(revoke), []string{acct1}) {
			t.Errorf("owed=%v lost=%v round=%v, want %s owed again in round 7", owed, revokeLost(revoke), revoke["round"], acct1)
		}
	})

	t.Run("a subscriber an account entry entitles is not judged", func(t *testing.T) {
		share := []eventShareEntry{{Kind: eventShareKindAccount, ID: "000000000001", Access: eventShareAccessSubscribe}}
		revoke, _, _ := eventBusRevokeInputs(share, withListing(revokeState(nil, nil), foreignListing("000000000001", "000000000003")))
		if want := []string{foreignListing("000000000003").Subscribers[0].ARN}; !reflect.DeepEqual(revokeJudged(revoke), want) {
			t.Errorf("judged=%v, want %v", revokeJudged(revoke), want)
		}
	})

	// The same OU entry, before any round judged account 3: it is owed, so
	// lost is not empty and a sweep that cannot classify the account stops.
	t.Run("an unjudged subscriber under an OU entry starts a round", func(t *testing.T) {
		share := []eventShareEntry{{Kind: eventShareKindOU, ID: "ou-ab12-cd34ef56", Access: eventShareAccessSubscribe}}
		granted := []string{"ou:ou-ab12-cd34ef56"}
		sweep, rec := settled(granted, nil, 3)
		revoke, owed, _ := eventBusRevokeInputs(share, withListing(revokeState(sweep, rec, "subscribe/ou/ou-ab12-cd34ef56"),
			foreignListing("000000000003")))
		if !reflect.DeepEqual(owed, []string{acct3}) || revoke["round"] != 4 || revoke["keep_ous"].([]interface{})[0] != "ou-ab12-cd34ef56" {
			t.Errorf("owed=%v round=%v keep_ous=%v, want %s owed in round 4 with the OU kept", owed, revoke["round"], revoke["keep_ous"], acct3)
		}
	})

	t.Run("a principal both removed and listed keeps the removal wording", func(t *testing.T) {
		sweep, rec := settled([]string{acct1}, nil, 1)
		_, owed, foreign := eventBusRevokeInputs(publish, withListing(revokeState(sweep, rec), foreignListing("000000000001")))
		if !reflect.DeepEqual(owed, []string{acct1}) || len(foreign) != 0 {
			t.Errorf("owed=%v foreign=%v, want %s owed through the record only", owed, foreign, acct1)
		}
	})
}

func TestEventShareRevokeDetailForListedSubscribers(t *testing.T) {
	acct1 := "account:000000000001"
	got := eventShareRevokeDetail("myapp", "dev", []string{acct1}, []string{acct1}, nil)
	if !strings.Contains(got, "has subscribers on myapp-dev and no account entry names it. The next deploy revokes them.") {
		t.Errorf("explicit-only detail = %q", got)
	}
	got = eventShareRevokeDetail("myapp", "dev", []string{acct1}, []string{acct1}, []string{"organization"})
	if !strings.Contains(got, "unless a remaining organization or OU entry covers the account; if it cannot tell, it stops") ||
		!strings.Contains(got, "organizations:ListParents") {
		t.Errorf("organization detail = %q", got)
	}
}

func TestListEventForeignSubscribers(t *testing.T) {
	envMap := map[string]interface{}{"project": "myapp", "env": "dev", "region": "ap-southeast-2", "aws_profile": "none"}

	t.Run("keeps other accounts' subscribers that are not revoked", func(t *testing.T) {
		stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) {
			return []eventBusSubscriber{
				{ARN: foreignSub3, Account: "000000000003"},
				{ARN: "arn:aws:events:ap-southeast-2:000000000000:subscriber/myapp-events-x/0000", Account: "000000000000"},
				{ARN: "arn:aws:events:ap-southeast-2:000000000002:subscriber/old/0002", Account: "000000000002", Revoked: true},
				{ARN: foreignSub1, Account: "000000000001"},
			}, nil
		})
		f := listEventForeignSubscribers(context.Background(), envMap, fakeBusARN)
		want := []eventForeignSubscriber{{ARN: foreignSub1, Account: "000000000001"}, {ARN: foreignSub3, Account: "000000000003"}}
		if !f.Read || f.unknown() || !reflect.DeepEqual(f.Subscribers, want) {
			t.Errorf("listing = %+v, want read with %+v", f, want)
		}
		if got := f.accounts(); !reflect.DeepEqual(got, []string{"000000000001", "000000000003"}) {
			t.Errorf("accounts = %v", got)
		}
	})

	t.Run("a failed call is unknown, never empty", func(t *testing.T) {
		stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) {
			return nil, errors.New("operation error EventBridge V2: ListSubscribers, AccessDeniedException")
		})
		f := listEventForeignSubscribers(context.Background(), envMap, fakeBusARN)
		if !f.unknown() || !strings.Contains(f.Err, "AccessDeniedException") {
			t.Errorf("listing = %+v, want unknown with the cause", f)
		}
	})

	t.Run("an entry without an account is unknown", func(t *testing.T) {
		stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) {
			return []eventBusSubscriber{{ARN: foreignSub1}}, nil
		})
		if f := listEventForeignSubscribers(context.Background(), envMap, fakeBusARN); !f.unknown() {
			t.Errorf("listing = %+v, want unknown", f)
		}
	})

	t.Run("a bus ARN that is not a V2 bus is unknown without a call", func(t *testing.T) {
		calls := stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) { return nil, nil })
		if f := listEventForeignSubscribers(context.Background(), envMap, ""); !f.unknown() || *calls != 0 {
			t.Errorf("listing = %+v after %d calls, want unknown and no call", f, *calls)
		}
	})
}

func TestEventForeignNeeded(t *testing.T) {
	enhanced := func(share ...interface{}) map[string]interface{} {
		block := map[string]interface{}{"enabled": true, "type": eventBusTypeEnhanced}
		if len(share) > 0 {
			block["share"] = share
		}
		return map[string]interface{}{"event_bus": block}
	}
	bus := func(edit func(*eventsAwsccState)) eventsAwsccState {
		s := emptyEventsState()
		s.Bus, s.BusARN = true, fakeBusARN
		if edit != nil {
			edit(&s)
		}
		return s
	}
	cases := []struct {
		name   string
		envMap map[string]interface{}
		state  eventsAwsccState
		want   bool
	}{
		{"never shared", enhanced(), bus(nil), false},
		{"a share entry", enhanced(map[string]interface{}{"account": "000000000002", "access": "publish"}), bus(nil), true},
		{"the guard", enhanced(), bus(func(s *eventsAwsccState) { s.Guard = true }), true},
		{"a revoke record", enhanced(), bus(func(s *eventsAwsccState) { s.RevokeRecord = &eventsRevokeRecord{} }), true},
		{"no bus deployed", enhanced(map[string]interface{}{"account": "000000000002", "access": "publish"}), emptyEventsState(), false},
		{"unreadable state", enhanced(map[string]interface{}{"account": "000000000002", "access": "publish"}), unreadableEventsState(), false},
		{"leaving enhanced", map[string]interface{}{"event_bus": map[string]interface{}{"enabled": true}}, bus(func(s *eventsAwsccState) { s.Guard = true }), false},
	}
	for _, c := range cases {
		if got := eventForeignNeeded(c.envMap, c.state); got != c.want {
			t.Errorf("%s: needed = %v, want %v", c.name, got, c.want)
		}
	}
}

// A shared bus whose subscribers cannot be listed refuses, as an unreadable
// state does, before main.tf is written.
func TestEnhancedRenderRefusesUnlistedSubscribers(t *testing.T) {
	root := newEventsRenderRoot(t)
	stubEventsState(t, func() eventsAwsccState {
		s := emptyEventsState()
		s.Bus, s.BusARN, s.Guard = true, fakeBusARN, true
		return s
	})
	stubEventBusSubscribers(t, func() ([]eventBusSubscriber, error) {
		return nil, errors.New("operation error EventBridge V2: ListSubscribers, expired token")
	})
	_, _, output, err := root.render(t, chainStepYAML(chainEnhancedPublishOnly, `
  - name: orders
    container_port: 3000
`))
	if err == nil {
		t.Fatalf("generate accepted a shared bus whose subscribers could not be listed:\n%s", output)
	}
	if !errors.Is(err, errEventsStateUnreadable) || !strings.Contains(err.Error(), "could not be listed") ||
		!strings.Contains(err.Error(), "expired token") || !strings.Contains(err.Error(), "IAM action events:ListSubscribers") ||
		!strings.Contains(err.Error(), "aws eventsv2 list-subscribers") || strings.Contains(err.Error(), "eventsv2:ListSubscribers") {
		t.Errorf("refusal = %v, want the listing refusal, retried after the AWS pre-flight", err)
	}
}
