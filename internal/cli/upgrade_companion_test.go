package cli

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestUpgradeCompanion_UsesUpdatedExecutableAfterBothInstallPaths(t *testing.T) {
	for _, method := range []installMethod{installBinary, installGoInstall, installNPM} {
		t.Run(method.String(), func(t *testing.T) {
			var events []string
			deps := upgradeDeps{
				detect: func(string) (upgradeInfo, error) {
					return upgradeInfo{Path: "/updated/tool", LinkPath: "/updated/tool", Method: method, Version: "1.0.0", Latest: "2.0.0", Command: "update-package"}, nil
				},
				run: func(io.Writer, string) error { events = append(events, "install"); return nil },
				install: func(io.Writer, string, string, string, *http.Client) error {
					events = append(events, "install")
					return nil
				},
				companion: &upgradeCompanionDeps{
					installed: func() (bool, error) { events = append(events, "detect companion"); return true, nil },
					run: func(path, _ string, args []string, _ io.Writer) (string, error) {
						if path != "/updated/tool" {
							t.Errorf("path = %q", path)
						}
						events = append(events, strings.Join(args, " "))
						return "agnostic-ai version 2.0.0", nil
					},
				},
			}
			if err := runUpgradeWithDeps(io.Discard, false, "", "1.0.0", deps); err != nil {
				t.Fatal(err)
			}
			want := []string{"detect companion", "install", "--version", "companion install"}
			if !reflect.DeepEqual(events, want) {
				t.Errorf("events = %v, want %v", events, want)
			}
		})
	}
}

func TestUpgradeCompanion_SkipsUninstalledAndNonInstallingRuns(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		installed, check, fail bool
	}{
		{name: "uninstalled"}, {name: "check", installed: true, check: true}, {name: "installation failed", installed: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps := upgradeDeps{
				detect: func(string) (upgradeInfo, error) {
					return upgradeInfo{Method: installGoInstall, Version: "1.0.0", Latest: "2.0.0", Command: "update"}, nil
				},
				run: func(io.Writer, string) error {
					if tc.fail {
						return errors.New("install failed")
					}
					return nil
				},
				companion: &upgradeCompanionDeps{installed: func() (bool, error) {
					if tc.check {
						t.Error("checked companion without an installation")
					}
					return tc.installed, nil
				}, run: func(string, string, []string, io.Writer) (string, error) {
					t.Error("ran companion update")
					return "", nil
				}},
			}
			err := runUpgradeWithDeps(io.Discard, tc.check, "", "1.0.0", deps)
			if (err != nil) != tc.fail {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestUpgradeCompanion_ReportsPartialFailure(t *testing.T) {
	buildErr := errors.New("Go is missing")
	deps := upgradeDeps{
		detect: func(string) (upgradeInfo, error) {
			return upgradeInfo{Path: "/new/tool", Method: installGoInstall, Version: "1.0.0", Latest: "2.0.0", Command: "update"}, nil
		},
		run: func(io.Writer, string) error { return nil },
		companion: &upgradeCompanionDeps{installed: func() (bool, error) { return true, nil }, run: func(_ string, _ string, args []string, _ io.Writer) (string, error) {
			if args[0] == "--version" {
				return "agnostic-ai version 2.0.0", nil
			}
			return "", buildErr
		}},
	}
	err := runUpgradeWithDeps(io.Discard, false, "", "1.0.0", deps)
	if !errors.Is(err, buildErr) {
		t.Errorf("error = %v", err)
	}
	for _, text := range []string{"CLI upgrade succeeded", "companion update failed", "agnostic-ai companion install"} {
		if err == nil || !strings.Contains(err.Error(), text) {
			t.Errorf("error lacks %q: %v", text, err)
		}
	}
}

func TestUpgradeCompanion_UsesHomebrewStablePath(t *testing.T) {
	info := upgradeInfo{Path: "/brew/Caskroom/agnostic-ai/1.0.0/agnostic-ai", LinkPath: "/brew/Caskroom/agnostic-ai/1.0.0/agnostic-ai", Method: installHomebrew, Version: "1.0.0", Latest: "2.0.0"}
	var paths []string
	deps := upgradeCompanionDeps{run: func(path, _ string, args []string, _ io.Writer) (string, error) {
		paths = append(paths, path)
		return "agnostic-ai version 2.0.0", nil
	}}
	if err := refreshUpgradedCompanion(io.Discard, info, "2.0.0", deps); err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.FromSlash("/brew/bin/agnostic-ai"), filepath.FromSlash("/brew/bin/agnostic-ai")}; !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}
}

func TestUpgradeCompanion_RefusesOldExecutable(t *testing.T) {
	calls := 0
	deps := upgradeCompanionDeps{run: func(_ string, _ string, args []string, _ io.Writer) (string, error) {
		calls++
		if args[0] != "--version" {
			t.Error("rebuilt with old executable")
		}
		return "agnostic-ai version 1.0.0", nil
	}}
	if err := refreshUpgradedCompanion(io.Discard, upgradeInfo{Path: "/old/tool", Version: "1.0.0"}, "2.0.0", deps); err == nil {
		t.Error("accepted old executable")
	}
	if calls != 1 {
		t.Errorf("calls = %d", calls)
	}
}

func TestUpgradeCompanion_ExplicitVersionUsesRequestedRelease(t *testing.T) {
	var verified string
	deps := upgradeDeps{
		detect: func(string) (upgradeInfo, error) {
			return upgradeInfo{Path: "/updated/tool", Method: installBinary, Version: "2.0.0", Latest: "3.0.0"}, nil
		},
		install: func(_ io.Writer, _ string, version string, _ string, _ *http.Client) error {
			verified = version
			return nil
		},
		companion: &upgradeCompanionDeps{installed: func() (bool, error) { return true, nil }, run: func(_ string, _ string, args []string, _ io.Writer) (string, error) {
			return "agnostic-ai version " + verified, nil
		}},
	}
	if err := runUpgradeWithDeps(io.Discard, false, "1.5.0", "2.0.0", deps); err != nil {
		t.Fatal(err)
	}
	if verified != "1.5.0" {
		t.Errorf("verified = %q", verified)
	}
}

func TestUpgradeCompanion_SkipsMatchingOrRemovedCompanion(t *testing.T) {
	for _, tc := range []struct {
		name, version string
		found         bool
	}{{"matching", "v2.0.0", true}, {"removed", "", false}, {"unknown", "", true}} {
		t.Run(tc.name, func(t *testing.T) {
			var builds int
			deps := upgradeCompanionDeps{
				current: func() (string, bool, error) { return tc.version, tc.found, nil },
				run: func(_ string, _ string, args []string, _ io.Writer) (string, error) {
					if args[0] == "--version" {
						return "agnostic-ai version 2.0.0", nil
					}
					builds++
					if !reflect.DeepEqual(args, []string{"companion", "install"}) {
						return "", errors.New("unknown flag")
					}
					return "", nil
				},
			}
			if err := refreshUpgradedCompanion(io.Discard, upgradeInfo{Path: "/updated/tool"}, "2.0.0", deps); err != nil {
				t.Fatal(err)
			}
			want := 0
			if tc.name == "unknown" {
				want = 1
			}
			if builds != want {
				t.Errorf("builds = %d, want %d", builds, want)
			}
		})
	}
}

func TestGuidedUpgrade_ContinuesAfterCompanionUpdateFails(t *testing.T) {
	var out bytes.Buffer
	var commands []string
	deps := guidedUpgradeDeps{
		check:   func(string) (upgradeOffer, error) { return upgradeOffer{Latest: "2.0.0"}, nil },
		project: func() (string, error) { return "", nil },
		install: func(io.Writer, string, string) (string, error) {
			return "/updated/tool", &companionUpgradeError{cause: errors.New("Go is missing")}
		},
		run: func(path, _ string, args []string, _ io.Writer) (string, error) {
			if path != "/updated/tool" {
				t.Errorf("path = %q", path)
			}
			commands = append(commands, strings.Join(args, " "))
			return "agnostic-ai version 2.0.0", nil
		},
	}
	cmd := &cobra.Command{Use: "status"}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetOut(&out)
	handled, err := runGuidedUpgrade(cmd, "1.0.0", deps)
	if !handled || err != nil {
		t.Errorf("handled=%v err=%v", handled, err)
	}
	if !strings.Contains(out.String(), "CLI upgrade succeeded, but companion update failed") {
		t.Errorf("output = %s", out.String())
	}
	if want := []string{"--version"}; !reflect.DeepEqual(commands, want) {
		t.Errorf("commands = %v, want %v", commands, want)
	}
}

func TestUpgradeCompanion_RefreshesAnOldCompanionWhenTheCLIIsCurrent(t *testing.T) {
	for _, tc := range []struct {
		name, builtFor string
		want           []string
	}{
		{"old companion", "0.9.0", []string{"--version", "companion install"}},
		{"current companion", "1.0.0", []string{"--version"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var commands []string
			deps := upgradeDeps{
				detect: func(string) (upgradeInfo, error) {
					return upgradeInfo{Path: "/tool", Method: installHomebrew, Version: "1.0.0", Latest: "1.0.0"}, nil
				},
				companion: &upgradeCompanionDeps{
					installed: func() (bool, error) { return true, nil },
					current:   func() (string, bool, error) { return tc.builtFor, true, nil },
					run: func(path, _ string, args []string, _ io.Writer) (string, error) {
						commands = append(commands, strings.Join(args, " "))
						return "agnostic-ai version 1.0.0", nil
					},
				},
			}
			if err := runUpgradeWithDeps(io.Discard, false, "", "1.0.0", deps); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(commands, tc.want) {
				t.Errorf("commands = %v, want %v", commands, tc.want)
			}
		})
	}
}

func TestUpgradeCompanion_SaysWhenThePackageManagerHasNotPublishedTheRelease(t *testing.T) {
	deps := upgradeDeps{
		detect: func(string) (upgradeInfo, error) {
			return upgradeInfo{Path: "/brew/Caskroom/agnostic-ai/1.0.0/agnostic-ai", Method: installHomebrew, Version: "1.0.0", Latest: "2.0.0", Command: "update"}, nil
		},
		run: func(io.Writer, string) error { return nil },
		companion: &upgradeCompanionDeps{
			installed: func() (bool, error) { return true, nil },
			run: func(_ string, _ string, args []string, _ io.Writer) (string, error) {
				if args[0] != "--version" {
					t.Error("updated the companion with the old CLI")
				}
				return "agnostic-ai version 1.0.0", nil
			},
		},
	}

	err := runUpgradeWithDeps(io.Discard, false, "", "1.0.0", deps)

	if err == nil || err.Error() != "the CLI is still 1.0.0: Homebrew does not serve 2.0.0 yet. Run agnostic-ai upgrade again later" {
		t.Errorf("err = %v", err)
	}
}

func TestUpgradeCompanion_BlamesTheNewCLIWhenItCannotRun(t *testing.T) {
	info := upgradeInfo{Path: "/brew/Caskroom/agnostic-ai/1.0.0/agnostic-ai", Method: installHomebrew, Version: "1.0.0"}
	deps := upgradeCompanionDeps{run: func(path, _ string, _ []string, _ io.Writer) (string, error) {
		if path == info.Path {
			return "agnostic-ai version 1.0.0", nil
		}
		return "", errors.New("killed")
	}}

	err := refreshUpgradedCompanion(io.Discard, info, "2.0.0", deps)

	var notYet cliNotYetPublished
	if err == nil || errors.As(err, &notYet) || !strings.Contains(err.Error(), "killed") {
		t.Errorf("err = %v", err)
	}
}

func TestUpgradeCompanion_ConfirmsReleaseBeforeChangingInstall(t *testing.T) {
	for _, tc := range []struct {
		name, installed, latest, outcome string
	}{
		{"matched", "1.0.0", "2.0.0", "Terminal update completed."},
		{"changed", "1.0.0", "3.0.0", "Terminal update failed: release changed since confirmation:"},
		{"unreachable", "1.0.0", "", "Terminal update failed: could not confirm release 2.0.0:"},
		{"would downgrade", "3.0.0", "2.0.0", "Terminal update failed: release changed since confirmation:"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ran := false
			deps := upgradeDeps{
				detect: func(string) (upgradeInfo, error) {
					return upgradeInfo{Method: installNPM, Version: tc.installed, Latest: tc.latest, Command: "npm install"}, nil
				},
				run: func(io.Writer, string) error { ran = true; return nil },
			}
			cmd := newUpgradeCmdWithDeps(deps)
			cmd.SetArgs([]string{"--companion-confirmed-release", "2.0.0"})
			var log bytes.Buffer
			cmd.SetOut(&log)
			cmd.SetErr(&log)
			err := cmd.Execute()
			wantRun := tc.name == "matched"
			if ran != wantRun || (err == nil) != wantRun || !strings.Contains(log.String(), tc.outcome) {
				t.Errorf("ran %t, error %v, log %q; want ran %t and %q", ran, err, &log, wantRun, tc.outcome)
			}
		})
	}
}

func TestUpgradeCompanion_ConfirmedReleaseRepairsAnOldCompanionWhenTheCLIIsCurrent(t *testing.T) {
	var commands []string
	deps := upgradeDeps{
		detect: func(string) (upgradeInfo, error) {
			return upgradeInfo{Path: "/tool", Method: installHomebrew, Version: "2.0.0", Latest: "2.0.0", Command: "brew upgrade"}, nil
		},
		run: func(io.Writer, string) error { t.Error("upgraded a current CLI"); return nil },
		companion: &upgradeCompanionDeps{
			installed: func() (bool, error) { return true, nil },
			current:   func() (string, bool, error) { return "1.0.0", true, nil },
			run: func(_, _ string, args []string, _ io.Writer) (string, error) {
				commands = append(commands, strings.Join(args, " "))
				return "agnostic-ai version 2.0.0", nil
			},
		},
	}
	cmd := newUpgradeCmdWithDeps(deps)
	cmd.SetArgs([]string{"--companion-confirmed-release", "2.0.0"})
	var log bytes.Buffer
	cmd.SetOut(&log)
	cmd.SetErr(&log)
	if err := cmd.Execute(); err != nil || !strings.Contains(log.String(), "Terminal update completed.") {
		t.Fatalf("error %v, log %q; want a completed update", err, &log)
	}
	if want := []string{"--version", "companion install"}; !reflect.DeepEqual(commands, want) {
		t.Errorf("commands = %v, want %v", commands, want)
	}
}
