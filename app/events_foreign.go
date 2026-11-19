package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridgev2"
)

// Other accounts' subscribers on the project bus, as the bus owner lists
// them at generate (eventsv2 ListSubscribers names every subscriber with its
// account and whether it is revoked).
//
// The revoke accounting in state (RAM, the sweep, the record) says who could
// subscribe, as far as the applies that wrote it got. An apply can associate
// a principal in RAM and stop before its sweep, and a later apply can remove
// that association and stop before its sweep, so no record names a principal
// whose subscribers still deliver. The listing answers from what exists: a
// subscriber of another account that no account entry names is owed a
// revocation until a sweep has judged it (see eventBusRevokeInputs).

// eventForeignKey is the envMap key of the listing.
const eventForeignKey = "_event_bus_foreign_subscribers"

// eventForeignSubscribers is the listing of the bus's subscribers that
// belong to other accounts and are not revoked.
type eventForeignSubscribers struct {
	Needed      bool                     // this render depends on the listing
	Read        bool                     // the listing was read
	Err         string                   // why it could not be read
	Subscribers []eventForeignSubscriber // other accounts' subscribers that are not revoked
}

// unknown reports whether this render depends on the listing and could not
// read it.
func (f eventForeignSubscribers) unknown() bool { return f.Needed && !f.Read }

// accounts is the sorted, de-duplicated accounts of the subscribers.
func (f eventForeignSubscribers) accounts() []string {
	var out []string
	for _, s := range f.Subscribers {
		out = append(out, s.Account)
	}
	return principalsNotIn(out, nil)
}

// unentitled is the sorted ARNs of the subscribers whose account no entry of
// keepAccounts names: the ones only a sweep can judge.
func (f eventForeignSubscribers) unentitled(keepAccounts []string) []string {
	var out []string
	for _, s := range f.Subscribers {
		if !containsString(keepAccounts, s.Account) {
			out = append(out, s.ARN)
		}
	}
	return principalsNotIn(out, nil)
}

// eventForeignSubscriber is one subscriber of another account.
type eventForeignSubscriber struct {
	ARN     string
	Account string
}

// eventBusSubscriber is one entry of ListSubscribers.
type eventBusSubscriber struct {
	ARN     string
	Account string
	Revoked bool
}

// eventBusSubscribersLister lists every subscriber of busARN with the env
// YAML's profile and region, following NextToken. A variable so tests stub
// AWS.
var eventBusSubscribersLister = func(ctx context.Context, e Env, busARN string) ([]eventBusSubscriber, error) {
	cfg, err := loadTestEventAWSConfig(ctx, e)
	if err != nil {
		return nil, fmt.Errorf("loading the AWS configuration: %w", err)
	}
	pages := eventbridgev2.NewListSubscribersPaginator(eventbridgev2.NewFromConfig(cfg),
		&eventbridgev2.ListSubscribersInput{EventBusArn: aws.String(busARN)})
	var out []eventBusSubscriber
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("eventsv2 ListSubscribers on %s: %w", busARN, err)
		}
		for _, s := range page.Subscribers {
			out = append(out, eventBusSubscriber{
				ARN:     aws.ToString(s.SubscriberArn),
				Account: aws.ToString(s.SubscriberAccountId),
				Revoked: aws.ToBool(s.Revoked),
			})
		}
	}
	return out, nil
}

// eventBusV2Account is the account of an EventBridge V2 bus ARN.
func eventBusV2Account(arn string) (string, bool) {
	if _, ok := eventBusV2Name(arn); !ok {
		return "", false
	}
	account := strings.SplitN(arn, ":", 6)[4]
	return account, isAWSAccountID(account)
}

func isAWSAccountID(s string) bool {
	if len(s) != 12 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// eventBusShareListed reports whether the YAML's event_bus block lists any
// share entry.
func eventBusShareListed(envMap map[string]interface{}) bool {
	block, _ := yamlSubMap(envMap, "event_bus")
	items, _ := block["share"].([]interface{})
	return len(items) > 0
}

// eventForeignNeeded reports whether a render must list the bus's
// subscribers: the YAML keeps the enhanced bus, the state holds it, and the
// bus is or was shared. A principal can be associated in RAM only after the
// guard exists, and the guard stays while the bus does, so a bus with no
// guard, no RAM association, no revoke step and no share entry has never let
// another account subscribe.
func eventForeignNeeded(envMap map[string]interface{}, st eventsAwsccState) bool {
	block, _ := yamlSubMap(envMap, "event_bus")
	if t, _ := block["type"].(string); t != eventBusTypeEnhanced {
		return false
	}
	if !st.Readable || !st.Bus {
		return false
	}
	return eventBusShareListed(envMap) || st.Guard || len(st.RamPrincipals) > 0 ||
		st.Revoke != nil || st.RevokeRecord != nil
}

// prepareEventForeignSubscribers lists the other accounts' subscribers for
// normalizeEvents, after prepareEventStateProbe. Only a render that writes
// main.tf calls it: the listing decides whether this apply sweeps.
func prepareEventForeignSubscribers(ctx context.Context, envMap map[string]interface{}) {
	st, _ := envMap[eventsAwsccStateKey].(eventsAwsccState)
	f := eventForeignSubscribers{Needed: eventForeignNeeded(envMap, st)}
	if f.Needed {
		f = listEventForeignSubscribers(ctx, envMap, st.BusARN)
	}
	envMap[eventForeignKey] = f
}

// listEventForeignSubscribers reads the listing. A bus ARN that is not an
// EventBridge V2 bus, a failed call or an entry without a readable account
// leaves the listing unread, never empty.
func listEventForeignSubscribers(ctx context.Context, envMap map[string]interface{}, busARN string) eventForeignSubscribers {
	f := eventForeignSubscribers{Needed: true}
	owner, ok := eventBusV2Account(busARN)
	if !ok {
		f.Err = fmt.Sprintf("the state holds no EventBridge V2 bus ARN for the project bus (%q)", busARN)
		return f
	}
	e := Env{
		Project:    stringFromMap(envMap, "project"),
		Env:        stringFromMap(envMap, "env"),
		Region:     stringFromMap(envMap, "region"),
		AWSProfile: stringFromMap(envMap, "aws_profile"),
	}
	ctx, cancel := context.WithTimeout(ctx, eventsStatePullTimeout)
	defer cancel()
	subs, err := eventBusSubscribersLister(ctx, e, busARN)
	if err != nil {
		f.Err = eventsStateErrorLine(err)
		return f
	}
	for _, s := range subs {
		if !isAWSAccountID(s.Account) {
			f.Err = fmt.Sprintf("ListSubscribers gave subscriber %s no 12-digit account", s.ARN)
			return f
		}
		if s.Account == owner || s.Revoked {
			continue
		}
		f.Subscribers = append(f.Subscribers, eventForeignSubscriber{ARN: s.ARN, Account: s.Account})
	}
	sort.Slice(f.Subscribers, func(i, j int) bool { return f.Subscribers[i].ARN < f.Subscribers[j].ARN })
	f.Read = true
	return f
}

// eventForeignUnreadableProblem is the refusal of a render that could not
// list the bus's subscribers.
func eventForeignUnreadableProblem(envMap map[string]interface{}, env string, f eventForeignSubscribers) string {
	profile := stringFromMap(envMap, "aws_profile")
	login := "aws sso login"
	if profile != "" {
		login += " --profile " + profile
	}
	return fmt.Sprintf(
		"event_bus: the subscribers of the enhanced bus of %s could not be listed (%s). The bus is or was shared, and only the listing shows whether another account still has a subscriber that must be revoked. Allow the IAM action events:ListSubscribers (resource *) for AWS_PROFILE %s in region %s (check with aws eventsv2 list-subscribers --event-bus-arn <bus ARN>; for an expired session run %s) and run it again",
		env, f.Err, profile, stringFromMap(envMap, "region"), login)
}

// eventsReadsUnknown reports whether generate refused, or may refuse, for a
// read it could not make: the events state or the subscriber listing. The
// deploy retries such a refusal once after its AWS pre-flight.
func eventsReadsUnknown(envMap map[string]interface{}) bool {
	st, _ := envMap[eventsAwsccStateKey].(eventsAwsccState)
	f, _ := envMap[eventForeignKey].(eventForeignSubscribers)
	return st.unknown() || f.unknown()
}
