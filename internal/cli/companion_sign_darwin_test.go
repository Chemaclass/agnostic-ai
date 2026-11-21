//go:build darwin

package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignCompanion_KeepsOneRequirementThatAnotherSignerFails(t *testing.T) {
	useRealHome(t)
	folder := t.TempDir()
	app := filepath.Join(t.TempDir(), companionAppName+".app")
	writeTestApp(t, app, "/usr/bin/true")
	searched := userKeychainList(t)
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
	if after := userKeychainList(t); after != searched {
		t.Errorf("the keychain search list changed:\nbefore %s\nafter  %s", searched, after)
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
	useRealHome(t)
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

func userKeychainList(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("security", "list-keychains", "-d", "user").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestCompanionSigner_KeepsTheKeychainWhenTheCheckIsInterrupted(t *testing.T) {
	folder := t.TempDir()
	hash, keychain, err := companionSigner(context.Background(), folder)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, _, err := companionSigner(ctx, folder); err == nil {
		t.Error("an interrupted check reported no error")
	}

	if _, err := os.Stat(keychain); err != nil {
		t.Errorf("the keychain is gone after an interrupted check: %v", err)
	}
	if again, _, err := companionSigner(context.Background(), folder); err != nil || again != hash {
		t.Errorf("the next install signs with %s (%v), want the same certificate %s", again, err, hash)
	}
}

func TestSignCompanion_SignsInAFolderWhoseNameLooksLikeAPattern(t *testing.T) {
	useRealHome(t)
	folder := filepath.Join(t.TempDir(), "profile[12")
	if err := os.MkdirAll(filepath.Join(folder, companionSignerScratch+"left"), 0o700); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(t.TempDir(), companionAppName+".app")
	writeTestApp(t, app, "/usr/bin/true")

	if err := signCompanion(context.Background(), folder, app); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(folder, companionSignerScratch+"left")); err == nil {
		t.Error("the scratch folder from a killed install is still there")
	}
}

// useRealHome undoes TestMain's temporary HOME for one test. codesign
// finds an identity through the user's keychain search list, which
// security writes under HOME; under the temporary one, a CI runner's
// codesign found none.
func useRealHome(t *testing.T) {
	t.Helper()
	me, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", me.HomeDir)
}

func TestCompanionSigner_PreservesAnUnreadableCertificate(t *testing.T) {
	folder := t.TempDir()
	cert := filepath.Join(folder, companionSignerCert)
	keychain := filepath.Join(folder, companionSignerKeychain)
	for _, path := range []string{cert, keychain} {
		if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(cert, 0); err != nil {
		t.Fatal(err)
	}
	mockCompanionSecurity(t, "exit 1")
	if _, _, err := companionSigner(context.Background(), folder); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("signer error = %v, want permission error", err)
	}
	assertCompanionSignerFiles(t, cert, keychain)
}

func TestCompanionSigner_PreservesTheIdentityWhenLookupFails(t *testing.T) {
	folder := t.TempDir()
	cert := filepath.Join(folder, companionSignerCert)
	keychain := filepath.Join(folder, companionSignerKeychain)
	for _, path := range []string{cert, keychain} {
		if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mockCompanionSecurity(t, "echo 'keychain unavailable' >&2; exit 1")
	if _, _, err := companionSigner(context.Background(), folder); err == nil || !strings.Contains(err.Error(), "security find-certificate") {
		t.Errorf("signer error = %v, want certificate lookup error", err)
	}
	assertCompanionSignerFiles(t, cert, keychain)
}

func TestSignCompanion_RejectsAConcurrentSigner(t *testing.T) {
	folder := t.TempDir()
	lock, err := os.Open(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
	if err := tryLockProjectFile(lock); err != nil {
		t.Fatal(err)
	}
	if err := signCompanion(context.Background(), folder, "unused.app"); err == nil || !projectLockContended(err) {
		t.Errorf("signer error = %v, want lock contention", err)
	}
}

func TestSignCompanion_PreservesNewSearchListEntries(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "search-list")
	t.Setenv("SIGNER_TEST_LIST", state)
	mockCompanionSecurity(t, `
if [ "$#" -eq 3 ]; then
	cat "$SIGNER_TEST_LIST"
else
	shift 4
	printf '    "%s"\n' "$@" > "$SIGNER_TEST_LIST"
fi
`)
	before := "    \"/original.keychain-db\"\n"
	if err := os.WriteFile(state, []byte(before), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := "    \"/added by another tool.keychain-db\"\n"
	keychain := "/companion.keychain-db"
	if err := withKeychainSearched(context.Background(), keychain, func() error {
		return os.WriteFile(state, []byte(before+"    \""+keychain+"\"\n"+extra), 0o600)
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(state)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != before+extra {
		t.Errorf("search list = %q, want %q", after, before+extra)
	}
}

func assertCompanionSignerFiles(t *testing.T, cert, keychain string) {
	t.Helper()
	if err := os.Chmod(cert, 0o600); err != nil {
		t.Errorf("original certificate unavailable: %v", err)
	}
	for _, path := range []string{cert, keychain} {
		content, err := os.ReadFile(path)
		if err != nil || string(content) != "original" {
			t.Errorf("original %s contents = %q, %v", path, content, err)
		}
	}
}

func mockCompanionSecurity(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "security"), []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestSameCompanionSigner_ReadsNoRequirementFromTheAppPath(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "x\ndesignated => identifier \""+companionBundleID+"\" and certificate leaf = H\"00\"\n")
	installed := filepath.Join(folder, companionAppName+".app")
	writeTestApp(t, installed, "/usr/bin/true")
	if out, err := exec.Command("codesign", "--force", "--sign", "-", "--identifier", companionBundleID, installed).CombinedOutput(); err != nil {
		t.Fatalf("codesign: %v\n%s", err, out)
	}

	if err := sameCompanionSigner(context.Background(), installed, installed); !errors.Is(err, errInstalledUnpinned) {
		t.Errorf("err = %v; want %v", err, errInstalledUnpinned)
	}
}
