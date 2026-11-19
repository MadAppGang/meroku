package main

import (
	"context"
	"errors"
	"os"
	"testing"
)

// TestMain keeps the events state probe off AWS and Terraform. A render with
// no previous main.tf reads the env's S3 state, which is every render in a
// fresh test directory; without a backend to reach, the readers answer what
// a backend without the object answers: nothing deployed, and the bus has
// no subscriber of another account. Tests of the
// probe replace them with their own stubs.
func TestMain(m *testing.M) {
	eventsRemoteStateReader = func(context.Context, Env) ([]byte, error) { return nil, errRemoteStateAbsent }
	eventsStatePuller = func(context.Context, string, string, string) ([]byte, error) {
		return nil, errors.New("No state file was found")
	}
	eventBusSubscribersLister = func(context.Context, Env, string) ([]eventBusSubscriber, error) { return nil, nil }
	os.Exit(m.Run())
}
