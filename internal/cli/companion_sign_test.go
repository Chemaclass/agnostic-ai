package cli

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSignCompanion_KeepsOneRequirementThatAnotherSignerFails(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("codesign runs on macOS only")
	}
	folder := t.TempDir()
	app := filepath.Join(t.TempDir(), companionAppName+".app")
	writeTestApp(t, app, "/usr/bin/true")
	var requirements []string
	for _, build := range []string{"/usr/bin/true", "/usr/bin/false"} {
		writeTestApp(t, app, build)
		if err := signCompanion(context.Background(), folder, app); err != nil {
			t.Fatal(err)
		}
		out, err := exec.Command("codesign", "-d", "-r-", app).CombinedOutput()
		if err != nil {
			t.Fatalf("codesign -d: %v\n%s", err, out)
		}
		requirements = append(requirements, strings.TrimSpace(string(out)))
	}
	if requirements[0] != requirements[1] {
		t.Errorf("the requirement changed between builds:\n%s\n%s", requirements[0], requirements[1])
	}
	if !strings.Contains(requirements[0], `certificate leaf = H"`) {
		t.Errorf("the requirement does not pin the certificate:\n%s", requirements[0])
	}
	der, err := os.ReadFile(filepath.Join(folder, companionSignerCert))
	if err != nil {
		t.Fatal(err)
	}
	pinned := "-R=" + companionRequirement(certHash(der))
	if out, err := exec.Command("codesign", "--verify", pinned, app).CombinedOutput(); err != nil {
		t.Errorf("the signed app fails its own requirement: %v\n%s", err, out)
	}

	impostor := filepath.Join(t.TempDir(), companionAppName+".app")
	writeTestApp(t, impostor, "/usr/bin/true")
	if out, err := exec.Command("codesign", "--force", "--sign", "-", "--identifier", companionBundleID, impostor).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v\n%s", err, out)
	}
	if err := exec.Command("codesign", "--verify", pinned, impostor).Run(); err == nil {
		t.Error("an app signed ad hoc with the same identifier meets the requirement")
	}
}

func writeTestApp(t *testing.T, app, program string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(app, "Contents", "MacOS"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "Info.plist"), companionInfoPlist("1.2.3"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "Contents", "MacOS", companionName), raw, 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestSignCompanion_MakesTheCertificateAgainWhenTheKeychainLacksIt(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("codesign runs on macOS only")
	}
	folder := t.TempDir()
	app := filepath.Join(t.TempDir(), companionAppName+".app")
	writeTestApp(t, app, "/usr/bin/true")
	if err := signCompanion(context.Background(), folder, app); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, companionSignerCert), []byte("cut short"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(folder, companionSignerScratch+"left"), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := signCompanion(context.Background(), folder, app); err != nil {
		t.Fatalf("signing after a broken certificate file: %v", err)
	}

	der, err := os.ReadFile(filepath.Join(folder, companionSignerCert))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("codesign", "--verify", "-R="+companionRequirement(certHash(der)), app).CombinedOutput(); err != nil {
		t.Errorf("the app fails the new certificate's requirement: %v\n%s", err, out)
	}
	if left, _ := filepath.Glob(filepath.Join(folder, companionSignerScratch+"*")); len(left) != 0 {
		t.Errorf("scratch folders left: %v", left)
	}
}
