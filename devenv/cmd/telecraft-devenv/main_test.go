package main

import (
	"bytes"
	"strings"
	"testing"
)

// An invocation with no subcommand, or one nobody implements, prints the
// usage banner rather than guessing at which half of the environment was
// meant. The banner is the only list of the subcommands a user is shown,
// so it names both.
func TestUsageNamesBothSubcommands(t *testing.T) {
	for name, args := range map[string][]string{
		"no subcommand":      nil,
		"unknown subcommand": {"start"},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(args, &stdout, &stderr); code != 2 {
				t.Fatalf("exit %d, want 2", code)
			}
			for _, sub := range []string{"telecraft-devenv prepare", "telecraft-devenv run"} {
				if !strings.Contains(stderr.String(), sub) {
					t.Errorf("the usage banner does not name %q:\n%s", sub, stderr.String())
				}
			}
			if stdout.Len() != 0 {
				t.Errorf("usage went to stdout:\n%s", stdout.String())
			}
		})
	}
}

// Deliberately uncovered: main itself. It is os.Exit around run, and a
// test that called it would take the test binary down with it.
