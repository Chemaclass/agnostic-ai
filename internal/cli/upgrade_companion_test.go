package cli

import (
	"bytes"
	"errors"
	"io"
	"net/http"
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
		name                         string
		installed, check, fail, same bool
	}{
		{name: "uninstalled"}, {name: "check", installed: true, check: true}, {name: "installation failed", installed: true, fail: true}, {name: "already current", installed: true, same: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			latest := "2.0.0"
			if tc.same {
				latest = "1.0.0"
			}
			deps := upgradeDeps{
				detect: func(string) (upgradeInfo, error) {
					return upgradeInfo{Method: installGoInstall, Version: "1.0.0", Latest: latest, Command: "update"}, nil
				},
				run: func(io.Writer, string) error {
					if tc.fail {
						return errors.New("install failed")
					}
					return nil
				},
				companion: &upgradeCompanionDeps{installed: func() (bool, error) {
					if tc.check || tc.same {
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
	if want := []string{"/brew/bin/agnostic-ai", "/brew/bin/agnostic-ai"}; !reflect.DeepEqual(paths, want) {
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
