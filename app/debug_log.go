package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Debug logs.
//
// With MEROKU_TUI_DEBUG=1, meroku writes TUI and Terraform traces to
// <project root>/.meroku/debug/<name>, a directory only the user can read,
// inside .meroku/ which ignores itself in git. Every line is redacted before
// it is written. Without the variable, openDebugLog returns a writer that
// discards everything.

const merokuTUIDebugEnv = "MEROKU_TUI_DEBUG"

// openDebugLog opens the debug log name for appending. name is reduced to its
// base name, so it can never point outside the debug directory.
func openDebugLog(name string) (io.WriteCloser, error) {
	if os.Getenv(merokuTUIDebugEnv) != "1" {
		return nopWriteCloser{}, nil
	}
	base := filepath.Base(filepath.Clean(name))
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return nil, fmt.Errorf("debug log: invalid name %q", name)
	}

	dir, err := debugLogDir()
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, base), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("debug log: %w", err)
	}
	return &redactingLineWriter{w: f, c: f}, nil
}

// tuiDebugLog is openDebugLog for the TUI render and parse hot paths: it
// fails fast when debugging is off, so the guarded blocks are skipped instead
// of formatting lines for a writer that drops them.
func tuiDebugLog(name string) (io.WriteCloser, error) {
	if !tuiDebugEnabled {
		return nil, errTUIDebugDisabled
	}
	return openDebugLog(name)
}

// debugLogDir is .meroku/debug under the project root, or the user cache
// directory when that cannot be created.
func debugLogDir() (string, error) {
	if meroku, err := ensureMerokuDir(debugLogProjectRoot()); err == nil {
		dir := filepath.Join(meroku, "debug")
		if err := os.MkdirAll(dir, 0o700); err == nil {
			return dir, nil
		}
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("debug log: no directory to write to: %w", err)
	}
	dir := filepath.Join(cache, "meroku", "debug")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("debug log: creating %s: %w", dir, err)
	}
	return dir, nil
}

// debugLogProjectRoot is the working directory, or the project root when the
// working directory is env/<env>, where meroku runs Terraform.
func debugLogProjectRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	if filepath.Base(filepath.Dir(wd)) == "env" {
		return filepath.Dir(filepath.Dir(wd))
	}
	return wd
}

// ensureMerokuDir creates <root>/.meroku and, when there is none, a
// .gitignore in it that ignores everything, so briefings and debug logs never
// get committed. An existing .gitignore is the user's and is left alone.
func ensureMerokuDir(root string) (string, error) {
	meroku := filepath.Join(root, ".meroku")
	if err := os.MkdirAll(meroku, 0o755); err != nil {
		return "", fmt.Errorf("creating %s: %w", meroku, err)
	}
	ignore := filepath.Join(meroku, ".gitignore")
	if _, err := os.Stat(ignore); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(ignore, []byte("*\n"), 0o644); err != nil {
			return "", fmt.Errorf("writing %s: %w", ignore, err)
		}
	}
	return meroku, nil
}

// redactingLineWriter buffers writes into lines and redacts each complete
// line before passing it on, so a secret is never split across two writes to
// the file. Close writes what is left.
type redactingLineWriter struct {
	mu  sync.Mutex
	w   io.Writer
	c   io.Closer
	buf []byte
}

func (r *redactingLineWriter) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, p...)
	for {
		i := bytes.IndexByte(r.buf, '\n')
		if i < 0 {
			break
		}
		line := redactSecrets(string(r.buf[:i+1]))
		r.buf = r.buf[i+1:]
		if _, err := io.WriteString(r.w, line); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

func (r *redactingLineWriter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	var werr error
	if len(r.buf) > 0 {
		_, werr = io.WriteString(r.w, redactSecrets(string(r.buf)))
		r.buf = nil
	}
	cerr := r.c.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// nopWriteCloser discards every write.
type nopWriteCloser struct{}

func (nopWriteCloser) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriteCloser) Close() error                { return nil }
