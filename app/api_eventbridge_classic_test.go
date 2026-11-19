package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge"
	"github.com/aws/aws-sdk-go-v2/service/eventbridge/types"
)

// fakeEventBridge records the classic calls Test Event makes.
type fakeEventBridge struct {
	described []string
	put       []string
}

func (f *fakeEventBridge) PutEvents(_ context.Context, in *eventbridge.PutEventsInput, _ ...func(*eventbridge.Options)) (*eventbridge.PutEventsOutput, error) {
	for _, entry := range in.Entries {
		f.put = append(f.put, aws.ToString(entry.EventBusName))
	}
	return &eventbridge.PutEventsOutput{Entries: []types.PutEventsResultEntry{{EventId: aws.String("evt-1")}}}, nil
}

func (f *fakeEventBridge) DescribeEventBus(_ context.Context, in *eventbridge.DescribeEventBusInput, _ ...func(*eventbridge.Options)) (*eventbridge.DescribeEventBusOutput, error) {
	f.described = append(f.described, aws.ToString(in.Name))
	return &eventbridge.DescribeEventBusOutput{Name: in.Name}, nil
}

// useClassicTestEventFakes points the handler at env and a recording classic
// client, and fails the test if the enhanced client or the account lookup is
// reached.
func useClassicTestEventFakes(t *testing.T, env Env) *fakeEventBridge {
	t.Helper()
	fake := &fakeEventBridge{}
	prevLoad, prevClassic, prevV2, prevAccount := loadEnvForTestEvent, newEventBridgeClient, newEventBridgeV2Client, callerAccount
	loadEnvForTestEvent = func(string) (Env, error) { return env, nil }
	newEventBridgeClient = func(context.Context, Env) (eventBridgeAPI, error) { return fake, nil }
	newEventBridgeV2Client = func(context.Context, Env) (eventBridgeV2API, error) {
		t.Error("the enhanced client was built on the classic path")
		return nil, context.Canceled
	}
	callerAccount = func(context.Context, Env) (string, error) {
		t.Error("the caller account was looked up on the classic path")
		return "", context.Canceled
	}
	t.Cleanup(func() {
		loadEnvForTestEvent, newEventBridgeClient, newEventBridgeV2Client, callerAccount = prevLoad, prevClassic, prevV2, prevAccount
	})
	return fake
}

// The classic Test Event path sends to the default bus for "" and "default",
// to <project>-<env> only when event_bus.enabled is true (after checking the
// bus exists), and refuses every other name before any AWS call.
func TestSendTestEventClassicBusResolution(t *testing.T) {
	enabled, disabled := true, false
	withBus := Env{Project: "myapp", Env: "dev", Region: "us-east-1",
		EventBus: &EventBus{Enabled: &enabled, Type: eventBusTypeClassic}}
	withBusNoType := Env{Project: "myapp", Env: "dev", Region: "us-east-1",
		EventBus: &EventBus{Enabled: &enabled}}
	busOff := Env{Project: "myapp", Env: "dev", Region: "us-east-1",
		EventBus: &EventBus{Enabled: &disabled}}
	noBlock := Env{Project: "myapp", Env: "dev", Region: "us-east-1"}

	cases := []struct {
		name          string
		env           Env
		bus           string
		wantStatus    int
		wantPut       []string
		wantDescribed []string
		wantInBody    string
	}{
		{"empty name, bus enabled", withBus, "", http.StatusOK, []string{"default"}, nil, "default bus"},
		{"default, bus enabled", withBus, "default", http.StatusOK, []string{"default"}, nil, "default bus"},
		{"default, no event_bus block", noBlock, "default", http.StatusOK, []string{"default"}, nil, "default bus"},
		{"empty name, bus disabled", busOff, "", http.StatusOK, []string{"default"}, nil, "default bus"},
		{"project bus, classic type", withBus, "myapp-dev", http.StatusOK, []string{"myapp-dev"}, []string{"myapp-dev"}, "myapp-dev bus"},
		{"project bus, type unset", withBusNoType, "myapp-dev", http.StatusOK, []string{"myapp-dev"}, []string{"myapp-dev"}, "myapp-dev bus"},
		{"project bus, bus disabled", busOff, "myapp-dev", http.StatusBadRequest, nil, nil, "event_bus.enabled is not true in dev.yaml"},
		{"project bus, no event_bus block", noBlock, "myapp-dev", http.StatusBadRequest, nil, nil, "event_bus.enabled is not true in dev.yaml"},
		{"the word project is not a bus name", withBus, "project", http.StatusBadRequest, nil, nil, "not a bus of this environment"},
		{"another env's bus", withBus, "myapp-prod", http.StatusBadRequest, nil, nil, "use default or myapp-dev"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := useClassicTestEventFakes(t, tc.env)
			body := `{"env":"dev","eventBusName":` + jsonQuote(tc.bus) + `,"source":"myapp.api","detailType":"x"}`
			rec := httptest.NewRecorder()
			sendTestEvent(rec, httptest.NewRequest(http.MethodPost, "/api/eventbridge/send-test-event", strings.NewReader(body)))

			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d; body %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var resp TestEventResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if !strings.Contains(resp.Message, tc.wantInBody) {
				t.Errorf("message %q, want it to contain %q", resp.Message, tc.wantInBody)
			}
			if resp.Success != (tc.wantStatus == http.StatusOK) {
				t.Errorf("success %v for status %d", resp.Success, rec.Code)
			}
			if !slices.Equal(fake.put, tc.wantPut) {
				t.Errorf("PutEvents buses %v, want %v", fake.put, tc.wantPut)
			}
			if !slices.Equal(fake.described, tc.wantDescribed) {
				t.Errorf("DescribeEventBus names %v, want %v", fake.described, tc.wantDescribed)
			}
		})
	}
}
