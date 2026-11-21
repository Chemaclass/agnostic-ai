package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestFindBinary_UsesBundledCommandBeforeOtherLocations(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", home)
	t.Setenv("APPDATA", home)
	t.Setenv("PATH", "")
	t.Setenv("AGNOSTIC_AI_BIN", "")
	previousFolders, previousSystem := usualFolders, usualSystemFolders
	t.Cleanup(func() {
		usualFolders, usualSystemFolders = previousFolders, previousSystem
	})
	usualFolders, usualSystemFolders = nil, nil
	command := fakeBundledCommand(t)
	found, err := findBinary()
	if err != nil || found != command {
		t.Errorf("findBinary() = %q, %v; want bundled %q", found, err, command)
	}
	external := filepath.Join(home, binaryName)
	if runtime.GOOS == "windows" {
		external += ".exe"
	}
	if err := os.WriteFile(external, []byte("external"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", home)
	found, err = findBinary()
	if err != nil || found != command {
		t.Errorf("with an older command on PATH got %q, %v; want bundled %q", found, err, command)
	}
	t.Setenv("AGNOSTIC_AI_BIN", external)
	found, err = findBinary()
	if err != nil || found != external {
		t.Errorf("findBinary() = %q, %v; want external %q", found, err, external)
	}
}

func TestLatestCompanionRelease_OffersOnlyNewStableVersions(t *testing.T) {
	cases := []struct {
		current, tag      string
		draft, prerelease bool
		want              string
	}{
		{"0.95.0", "v0.96.0", false, false, "0.96.0"},
		{"0.95.0", "v0.95.0", false, false, ""},
		{"0.96.0", "v0.95.0", false, false, ""},
		{"0.95.0", "v0.97.0-rc.1", false, true, ""},
		{"0.95.0", "v0.97.0", true, false, ""},
		{"dev", "v0.97.0", false, false, ""},
		{"0.9.0", "v0.10.0", false, false, "0.10.0"},
	}
	for _, tc := range cases {
		t.Run(tc.current+"/"+tc.tag, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Accept") != "application/vnd.github+json" {
					t.Error("missing GitHub accept header")
				}
				_, _ = fmt.Fprintf(w, `{"tag_name":%q,"draft":%t,"prerelease":%t,"assets":[{"name":%q,"state":"uploaded","size":123}]}`, tc.tag, tc.draft, tc.prerelease, updateImageName())
			}))
			defer srv.Close()
			got, err := latestCompanionRelease(context.Background(), srv.Client(), srv.URL, tc.current, updateImageName())
			if err != nil || got != tc.want {
				t.Errorf("latest release = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestLatestCompanionRelease_RequiresImageForThisMac(t *testing.T) {
	const arm = "agnostic-ai-companion_darwin_arm64.dmg"
	for _, asset := range []releaseAsset{
		{},
		{Name: "agnostic-ai-companion_darwin_amd64.dmg", State: "uploaded", Size: 100},
		{Name: "agnostic-ai-companion_darwin_arm64.tar.gz", State: "uploaded", Size: 100},
		{Name: arm, State: "starter", Size: 100},
		{Name: arm, State: "uploaded", Size: 0},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v0.96.0", "assets": []releaseAsset{asset}})
		}))
		got, err := latestCompanionRelease(t.Context(), srv.Client(), srv.URL, "0.95.0", arm)
		srv.Close()
		if err != nil || got != "" {
			t.Errorf("with asset %+v got %q, %v; want no update", asset, got, err)
		}
	}
}

func TestLatestCompanionRelease_ReportsFailedChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "rate limited", http.StatusForbidden) }))
	defer srv.Close()
	_, err := latestCompanionRelease(context.Background(), srv.Client(), srv.URL, "0.95.0", "agnostic-ai-companion_darwin_arm64.dmg")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Errorf("failed release check = %v", err)
	}
}

func fakeBundledCommand(t *testing.T) string {
	t.Helper()
	app := filepath.Join(t.TempDir(), "Applications", "agnostic-ai companion.app")
	command := filepath.Join(app, "Contents", "Resources", binaryName)
	if err := os.MkdirAll(filepath.Dir(command), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(command, []byte("command"), 0o700); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(app, "Contents", "MacOS", "agnostic-ai-companion")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("companion"), 0o700); err != nil {
		t.Fatal(err)
	}
	previousExecutable := companionExecutable
	t.Cleanup(func() { companionExecutable = previousExecutable })
	companionExecutable = func() (string, error) { return filepath.Join(app, "Contents", "MacOS", "agnostic-ai-companion"), nil }
	command, err := filepath.EvalSymlinks(command)
	if err != nil {
		t.Fatal(err)
	}
	return command
}

func TestAppMenu_OffersInstallAndUpdatesForBundledCommand(t *testing.T) {
	command := fakeBundledCommand(t)
	menu := application.NewMenu()
	controls := (&shell{}).addAppMenu(menu)
	if controls == nil || controls.command != command {
		t.Fatalf("app controls = %+v; want bundled command %s", controls, command)
	}
	for _, label := range []string{"Install command line tool", "Check for updates"} {
		if item := menu.FindByLabel(label); item == nil || !item.Enabled() {
			t.Errorf("menu has no enabled %q item", label)
		}
	}
}
