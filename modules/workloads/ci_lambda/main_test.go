package main

import (
	"context"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/stretchr/testify/require"
	"madappgang.com/infrastructure/ci_lambda/internal/handler"
	"madappgang.com/infrastructure/ci_lambda/internal/testsupport"
)

// TestInitFailureIgnoresEventsInsteadOfRetryingThem pins the initialization
// policy.
//
// EventBridge invokes this function asynchronously. A failed invocation is
// redelivered twice more, and an initialization failure fails *every*
// invocation — so os.Exit(1) at startup does not stop one event, it triples
// every event in the account that matches any of this project's rules, for as
// long as the configuration stays broken. The self-check on the line below it
// already refuses that trade for a single bad map entry; the two now agree.
func TestInitFailureIgnoresEventsInsteadOfRetryingThem(t *testing.T) {
	cause := errors.New("invalid configuration:\n  - PROJECT_NAME is required")
	h := inertHandler(testsupport.Logger(), cause)

	res, err := h(context.Background(), events.CloudWatchEvent{Source: "aws.ecr", DetailType: "ECR Image Action"})

	require.NoError(t, err, "returning an error here is what makes EventBridge redeliver the event")
	require.Equal(t, handler.StatusIgnored, res.Status)
	require.Contains(t, res.Detail, "PROJECT_NAME is required",
		"the reason has to reach whoever reads the invocation result")
}

// TestNoExitOnStartup is the guard for the same thing at the source level: a
// re-added os.Exit is invisible in behaviour tests, because nothing in this
// package is exercised at startup.
func TestNoExitOnStartup(t *testing.T) {
	src, err := os.ReadFile("main.go")
	require.NoError(t, err)

	code := regexp.MustCompile(`(?m)^\s*//.*$`).ReplaceAllString(string(src), "")
	require.False(t, strings.Contains(code, "os.Exit"),
		"main.go calls os.Exit: an initialization failure then fails every async invocation, "+
			"and EventBridge redelivers each of them twice more. Serve the event as `ignored` and "+
			"log the reason instead (see startInert).")
}
