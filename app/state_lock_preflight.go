package main

// state_lock_preflight.go clears a state lock whose run is gone before a
// deploy starts, so no step of the deploy fails on it.
//
// Terraform releases its lock when it exits normally or is interrupted. A run
// killed outright (a closed terminal, a crash, kill -9) leaves the lock in the
// state bucket, and every later plan, import or apply fails on it. When the
// lock was taken on this machine and no terraform is running in env/<env>,
// nothing can still be writing the state, and the lock is released. A lock
// held by a live run, or taken on another machine, stops the deploy with the
// reason instead.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// stateLockKey is the object terraform's native S3 locking writes.
func stateLockKey(e Env) string {
	return e.StateFile + ".tflock"
}

func stateBucketClient(ctx context.Context, e Env) (*s3.Client, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithSharedConfigProfile(e.AWSProfile), config.WithRegion(e.Region))
	if err != nil {
		return nil, fmt.Errorf("loading AWS config for profile %q: %w", e.AWSProfile, err)
	}
	return s3.NewFromConfig(cfg), nil
}

// stateLockReader returns the lock on the environment's state, if any.
var stateLockReader = func(ctx context.Context, e Env) (stateLock, bool, error) {
	client, err := stateBucketClient(ctx, e)
	if err != nil {
		return stateLock{}, false, err
	}
	obj, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(e.StateBucket),
		Key:    aws.String(stateLockKey(e)),
	})
	if err != nil {
		var noKey *types.NoSuchKey
		if errors.As(err, &noKey) {
			return stateLock{}, false, nil
		}
		return stateLock{}, false, fmt.Errorf("reading the state lock: %w", err)
	}
	defer obj.Body.Close()
	body, err := io.ReadAll(obj.Body)
	if err != nil {
		return stateLock{}, false, fmt.Errorf("reading the state lock: %w", err)
	}
	var l stateLock
	if err := json.Unmarshal(body, &l); err != nil || l.ID == "" {
		return stateLock{}, false, fmt.Errorf("the state lock is not terraform's lock format: %s", body)
	}
	return l, true, nil
}

// stateLockRemover deletes the lock, as terraform force-unlock does for the
// S3 backend.
var stateLockRemover = func(ctx context.Context, e Env) error {
	client, err := stateBucketClient(ctx, e)
	if err != nil {
		return err
	}
	_, err = client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(e.StateBucket),
		Key:    aws.String(stateLockKey(e)),
	})
	return err
}

// clearStaleStateLock releases a lock left by a run that is gone, and stops
// the deploy on a lock that something may still be using.
func clearStaleStateLock(ctx context.Context, e Env) error {
	if e.StateBucket == "" || e.StateFile == "" {
		return nil
	}
	lock, held, err := stateLockReader(ctx, e)
	if err != nil {
		// Unreadable is not proof of a lock. Terraform reports a real one.
		fmt.Printf("⚠️  Could not check the state lock: %v\n", err)
		return nil
	}
	if !held {
		return nil
	}

	envDir, _ := filepath.Abs(filepath.Join("env", e.Env))
	h := findLockHolder(lock, envDir)
	switch {
	case h.PID != 0:
		return fmt.Errorf("%s's state is locked by a terraform %s still running on this machine (PID %d, started %s), "+
			"probably in another meroku window; let it finish or stop it, then deploy again",
			e.Env, lockOperation(lock), h.PID, lock.Created)
	case !h.ThisMachine:
		return fmt.Errorf("%s's state is locked by a terraform %s started %s by %s on another machine; "+
			"if that run has ended, release it with: cd env/%s && terraform force-unlock %s",
			e.Env, lockOperation(lock), lock.Created, lock.Who, e.Env, lock.ID)
	}

	// Read again just before deleting, so a run that took the lock in between
	// keeps it.
	again, held, err := stateLockReader(ctx, e)
	if err != nil || !held || again.ID != lock.ID {
		return nil
	}
	if err := stateLockRemover(ctx, e); err != nil {
		return fmt.Errorf("releasing the stale state lock %s: %w", lock.ID, err)
	}
	fmt.Printf("🔓 Released a state lock left by a terraform %s started %s that is no longer running\n",
		lockOperation(lock), lock.Created)
	return nil
}

func lockOperation(l stateLock) string {
	if op := strings.TrimPrefix(strings.ToLower(l.Operation), "operationtype"); op != "" {
		return op
	}
	return "run"
}
