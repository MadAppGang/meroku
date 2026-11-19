package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// What `terraform state pull` prints, in colour, in a directory whose main.tf
// names a module terraform init has not installed.
const moduleNotInstalledColour = "\x1b[31m\x1b[31m╷\x1b[0m\x1b[0m\n" +
	"\x1b[31m│\x1b[0m \x1b[0m\x1b[1m\x1b[31mError: \x1b[0m\x1b[0m\x1b[1mModule not installed\x1b[0m\n" +
	"\x1b[31m│\x1b[0m \x1b[0m\n" +
	"\x1b[31m│\x1b[0m \x1b[0m\x1b[0m  on main.tf line 1:\n" +
	"\x1b[31m│\x1b[0m \x1b[0m   1: \x1b[4mmodule \"event_subscriber_orders_a\"\x1b[0m {\x1b[0m\n" +
	"\x1b[31m│\x1b[0m \x1b[0m\n" +
	"\x1b[31m│\x1b[0m \x1b[0mThis module is not yet installed. Run \"terraform init\" to install all modules\n" +
	"\x1b[31m│\x1b[0m \x1b[0mrequired by this configuration.\n" +
	"\x1b[31m╵\x1b[0m\x1b[0m"

// The same diagnostic with -no-color.
const moduleNotInstalledPlain = "\nError: Module not installed\n\n  on main.tf line 1:\n" +
	"   1: module \"event_subscriber_orders_a\" {\n\n" +
	"This module is not yet installed. Run \"terraform init\" to install all modules\n" +
	"required by this configuration."

func TestEventsStateErrorLineNamesTerraformsCause(t *testing.T) {
	const want = "terraform state pull: exit status 1: Error: Module not installed"
	for name, stderr := range map[string]string{
		"colour":    moduleNotInstalledColour,
		"no colour": moduleNotInstalledPlain,
	} {
		t.Run(name, func(t *testing.T) {
			got := eventsStateErrorLine(errors.New("terraform state pull: exit status 1\n" + stderr))
			if got != want {
				t.Errorf("eventsStateErrorLine = %q, want %q", got, want)
			}
			if strings.ContainsAny(got, "\x1b│╷╵") {
				t.Errorf("colour codes or box characters left in %q", got)
			}
		})
	}

	t.Run("no Error line: the first line with text", func(t *testing.T) {
		got := eventsStateErrorLine(errors.New("terraform state pull: exit status 1\n\nsomething went wrong\nmore"))
		if got != "terraform state pull: exit status 1: something went wrong" {
			t.Errorf("eventsStateErrorLine = %q", got)
		}
	})
}

func TestEventsStatePullRunsWithoutColour(t *testing.T) {
	cmd, err := eventsStatePullCmd(context.Background(), t.TempDir(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cmd.Args[1:], []string{"state", "pull", "-no-color"}) {
		t.Errorf("args = %v, want state pull -no-color", cmd.Args[1:])
	}
}

// A state read that failed because the env directory is not initialised for
// its main.tf still refuses, and says to run terraform init, not to log in.
func TestEventsStateUnreadableProblemForAnUninitialisedDirectory(t *testing.T) {
	envMap := map[string]interface{}{"aws_profile": "example-profile", "region": "ap-southeast-2"}

	t.Run("module not installed", func(t *testing.T) {
		st := eventsAwsccState{Tried: true, Needed: true, Err: eventsStateErrorLine(errors.New("terraform state pull: exit status 1\n" + moduleNotInstalledColour))}
		msg := eventsStateUnreadableProblem(envMap, "dev", st)
		assertContains(t, "refusal", msg,
			"the Terraform state of dev could not be read (terraform state pull: exit status 1: Error: Module not installed)",
			"env/dev is not initialised",
			"Run terraform -chdir=env/dev init and run it again",
		)
		assertNotContains(t, "refusal", msg, "sso login", "\x1b")
	})

	t.Run("backend not initialised", func(t *testing.T) {
		st := eventsAwsccState{Tried: true, Needed: true, Err: "terraform state pull: exit status 1: Error: Backend initialization required, please run \"terraform init\""}
		msg := eventsStateUnreadableProblem(envMap, "dev", st)
		assertContains(t, "refusal", msg, "terraform -chdir=env/dev init")
		assertNotContains(t, "refusal", msg, "sso login")
	})

	t.Run("an expired session keeps the login hint", func(t *testing.T) {
		st := eventsAwsccState{Tried: true, Needed: true, Err: "terraform state pull: exit status 1: Error: the SSO session has expired"}
		msg := eventsStateUnreadableProblem(envMap, "dev", st)
		assertContains(t, "refusal", msg, "aws sso login --profile example-profile")
		assertNotContains(t, "refusal", msg, "-chdir=env/dev init")
	})
}

// The real `terraform state pull` in a directory with a module terraform init
// has not installed: the read is unknown, and its cause is the missing module.
// Needs terraform on PATH; no provider is downloaded.
func TestEventsStatePullModuleNotInstalled(t *testing.T) {
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform is not on PATH")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".terraform"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "m"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte("module \"event_subscriber_orders_a\" {\n  source = \"./m\"\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := pullTerraformState(context.Background(), dir, "", "")
	if err == nil {
		t.Fatal("state pull succeeded with a module that is not installed")
	}
	line := eventsStateErrorLine(err)
	if !strings.HasSuffix(line, ": Error: Module not installed") {
		t.Errorf("cause = %q, want Terraform's Module not installed", line)
	}
	if !eventsStateNeedsInit(line) {
		t.Errorf("%q is not recognised as an uninitialised directory", line)
	}
}
