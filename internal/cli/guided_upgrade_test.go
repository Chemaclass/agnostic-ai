package cli

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestGuidedUpgrade_AcceptUsesVerifiedBinaryBeforeProjectChanges(t *testing.T) {
	var output bytes.Buffer
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(&output)
	var calls []string
	deps := guidedUpgradeDeps{
		check: func(string) (upgradeOffer, error) {
			return upgradeOffer{Latest: "0.82.0", Guidance: "## v0.82.0\nBreaking: migrate old specs"}, nil
		},
		project: func() (string, error) { return "/project", nil },
		install: func(io.Writer, string, string) (string, error) {
			calls = append(calls, "install")
			return "/new/agnostic-ai", nil
		},
		run: func(path, root string, args []string, out io.Writer) (string, error) {
			if path != "/new/agnostic-ai" {
				t.Errorf("binary %s", path)
			}
			calls = append(calls, strings.Join(args, " "))
			if args[0] == "--version" {
				return "agnostic-ai version 0.82.0\n", nil
			}
			return "", nil
		},
		reconcile: func(root, version string) error { calls = append(calls, "pin "+version); return nil },
	}
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Error("accepted upgrade did not handle invocation")
	}
	want := []string{"install", "--version", "pin 0.82.0", "migrate --dry-run", "migrate", "sync", "sync --check"}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %v, want %v", calls, want)
	}
	for _, want := range []string{"[Y/n]", "0.82.0", "requires", "schema", "Breaking", "Rerun"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %s in %s", want, output.String())
		}
	}
}

func TestGuidedUpgrade_DeclineAndOfflineContinueOriginalCommand(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		offline     bool
	}{{"decline", "n\n", false}, {"offline", "\n", true}, {"closed input", "", false}} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "status"}
			cmd.SetIn(strings.NewReader(tc.input))
			cmd.SetOut(io.Discard)
			deps := guidedUpgradeDeps{check: func(string) (upgradeOffer, error) {
				if tc.offline {
					return upgradeOffer{}, errors.New("offline")
				}
				return upgradeOffer{Latest: "0.82.0"}, nil
			}, project: func() (string, error) { return "", nil }, install: func(io.Writer, string, string) (string, error) { t.Error("must not install"); return "", nil }}
			handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
			if handled || err != nil {
				t.Errorf("handled %v, err %v", handled, err)
			}
		})
	}
}

func TestGuidedUpgrade_FailureStopsLaterSteps(t *testing.T) {
	for _, fail := range []string{"install", "--version", "migrate --dry-run", "migrate", "sync", "sync --check"} {
		t.Run(fail, func(t *testing.T) {
			cmd := &cobra.Command{Use: "status"}
			cmd.SetIn(strings.NewReader("y\n"))
			cmd.SetOut(io.Discard)
			reached := false
			deps := guidedUpgradeDeps{check: func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }, project: func() (string, error) { return "/project", nil }, reconcile: func(string, string) error {
				if reached {
					t.Error("reconciled after failure")
				}
				return nil
			}}
			deps.install = func(io.Writer, string, string) (string, error) {
				if fail == "install" {
					reached = true
					return "", errors.New("failed")
				}
				return "/new/tool", nil
			}
			deps.run = func(_, _ string, args []string, _ io.Writer) (string, error) {
				if reached {
					t.Error("ran after failure")
				}
				if strings.Join(args, " ") == fail {
					reached = true
					return "", errors.New("failed")
				}
				return "agnostic-ai version 0.82.0", nil
			}
			handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
			if !handled || err == nil || !reached {
				t.Errorf("handled %v, err %v, reached %v", handled, err, reached)
			}
		})
	}
}

func TestGuidedUpgrade_InstalledVersionMismatchDoesNotChangeProject(t *testing.T) {
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(io.Discard)
	deps := guidedUpgradeDeps{check: func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "0.82.0"}, nil }, project: func() (string, error) { return "/project", nil }, install: func(io.Writer, string, string) (string, error) { return "/tool", nil }, run: func(_, _ string, _ []string, _ io.Writer) (string, error) { return "agnostic-ai version 0.81.0", nil }, reconcile: func(string, string) error { t.Error("must not change pins"); return nil }}
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if !handled || err == nil || !strings.Contains(err.Error(), "project changes did not start") {
		t.Errorf("handled %v, err %v", handled, err)
	}
}

func TestGuidedUpgrade_OutsideProjectUpdatesToolOnly(t *testing.T) {
	cmd := &cobra.Command{Use: "init"}
	cmd.SetIn(strings.NewReader("\n"))
	var out bytes.Buffer
	cmd.SetOut(&out)
	deps := guidedUpgradeDeps{check: func(string) (upgradeOffer, error) {
		return upgradeOffer{Latest: "0.82.0", GuidanceError: "offline docs"}, nil
	}, project: func() (string, error) { return "", nil }, install: func(io.Writer, string, string) (string, error) { return "/tool", nil }, run: func(_, _ string, args []string, _ io.Writer) (string, error) {
		if len(args) != 1 || args[0] != "--version" {
			t.Errorf("unexpected %v", args)
		}
		return "agnostic-ai version 0.82.0", nil
	}, reconcile: func(string, string) error { t.Error("must not reconcile"); return nil }}
	handled, err := runGuidedUpgrade(cmd, "0.81.0", deps)
	if !handled || err != nil {
		t.Errorf("handled %v, err %v", handled, err)
	}
	if !strings.Contains(out.String(), "offline docs") || strings.Contains(out.String(), "sets this project's requires") {
		t.Errorf("output %s", out.String())
	}
}
