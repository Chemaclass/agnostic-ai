package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestCompanionUpdateStatus_ReportsTheLatestStableReleaseAndHowToUpgrade(t *testing.T) {
	info := upgradeInfo{Method: installHomebrew, Version: "0.96.0", Latest: "0.97.0", Command: upgradeCommandFor(installHomebrew)}
	var out bytes.Buffer

	if err := writeCompanionUpdateStatus(&out, info); err != nil {
		t.Fatal(err)
	}

	var got companionUpdateStatus
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("output %q: %v", out.String(), err)
	}
	want := companionUpdateStatus{Installed: "0.96.0", Latest: "0.97.0", Method: "homebrew", Command: upgradeCommandFor(installHomebrew), OneClick: true}
	if got != want {
		t.Errorf("status = %+v; want %+v", got, want)
	}
}

func TestCompanionUpdateStatus_OffersNoOneClickForAnUnknownInstall(t *testing.T) {
	var out bytes.Buffer
	if err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installUnknown, Version: "0.96.0", Latest: "0.97.0"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"oneClick":false`) || !strings.Contains(out.String(), `"method":"unknown"`) {
		t.Errorf("output = %s", out.String())
	}
}

func TestCompanionUpdateStatus_FailsWhenTheLatestReleaseIsUnknown(t *testing.T) {
	var out bytes.Buffer
	err := writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", LatestError: errors.New("offline")})
	if err == nil || !strings.Contains(err.Error(), "offline") || out.Len() != 0 {
		t.Errorf("err = %v, output %q", err, out.String())
	}
	err = writeCompanionUpdateStatus(&out, upgradeInfo{Method: installNPM, Version: "0.96.0", Latest: "0.97.0-rc.1"})
	if err == nil || !strings.Contains(err.Error(), "not a stable release") {
		t.Errorf("prerelease err = %v", err)
	}
}

func TestCompanionUpdateStatus_OneClickFollowsWhatUpgradeCanRun(t *testing.T) {
	status, err := companionUpdateStatusOf(upgradeInfo{Method: installBinary, Version: "0.96.0", Latest: "0.97.0"})
	if err != nil {
		t.Fatal(err)
	}
	if want := standalonePlatformError(runtime.GOOS) == nil; status.OneClick != want {
		t.Errorf("binary install on %s: oneClick = %v; want %v", runtime.GOOS, status.OneClick, want)
	}
}
