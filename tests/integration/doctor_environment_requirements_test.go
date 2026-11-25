package integration

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDoctorEnvironment_ReportsRequirementNamesWithoutValues(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, copilot, cursor]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", `install: npm ci
variables:
  - name: LOGS_API_TOKEN
    description: Token for the log search API
    secret: true
  - name: APP_ENV
allowed-domains: [logs.example.com]
`)
	t.Setenv("LOGS_API_TOKEN", "opaque-test-value-not-in-report")
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment", "--json")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	var report struct {
		Targets []struct {
			Target    string `json:"target"`
			Variables []struct {
				Name     string `json:"name"`
				Secret   bool   `json:"secret"`
				Location string `json:"location"`
			} `json:"variables"`
			Domains []struct {
				Domain   string `json:"domain"`
				Location string `json:"location"`
			} `json:"allowed_domains"`
		} `json:"targets"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("report JSON: %v\n%s", err, out)
	}
	if len(report.Targets) != 4 {
		t.Errorf("got %d targets, want 4", len(report.Targets))
	}
	for _, target := range report.Targets {
		if len(target.Variables) != 2 || target.Variables[0].Name != "LOGS_API_TOKEN" || !target.Variables[0].Secret || !strings.Contains(strings.ToLower(target.Variables[0].Location), "secret") {
			t.Errorf("%s secret settings: %+v", target.Target, target.Variables)
		}
		if len(target.Variables) == 2 && (target.Variables[1].Name != "APP_ENV" || target.Variables[1].Secret || target.Variables[1].Location == "") {
			t.Errorf("%s plain variable settings: %+v", target.Target, target.Variables)
		}
		if len(target.Domains) != 1 || target.Domains[0].Domain != "logs.example.com" || target.Domains[0].Location == "" {
			t.Errorf("%s domain settings: %+v", target.Target, target.Domains)
		}
		if target.Target == "cursor" && len(target.Domains) == 1 && !strings.Contains(target.Domains[0].Location, "egressAllowlist") {
			t.Errorf("Cursor domain should name the repository mapping: %+v", target.Domains)
		}
	}
	if strings.Contains(out, "opaque-test-value-not-in-report") {
		t.Error("doctor environment displayed a variable value")
	}
}

func TestDoctorEnvironment_SkipsAnInvalidVariableWithoutEchoingItsValue(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "variables: [LOGS_API_TOKEN=opaque-test-value-not-in-report]\n")
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	if !strings.Contains(out, "agnostic-ai lint") || strings.Contains(out, "opaque-test-value-not-in-report") {
		t.Errorf("unexpected report:\n%s", out)
	}
}

func TestDoctorEnvironment_KeepsChecklistWhenCursorCannotMapDomains(t *testing.T) {
	dir := t.TempDir()
	binary := builtBinary(t)
	writeProjectFile(t, dir, "agnostic-ai.yaml", "version: 1\ntargets: [cursor, codex]\n")
	writeProjectFile(t, dir, ".agnostic-ai/environments/dev.yaml", "install: npm ci\nallowed-domains: [https://opaque-test-value-not-in-report@example.com]\n")
	out, err := ellipsisCommand(t, binary, dir, "doctor", "environment")
	if err != nil {
		t.Fatalf("doctor environment: %v\n%s", err, out)
	}
	for _, want := range []string{"Cursor", "Codex Cloud", "agnostic-ai lint", "npm ci"} {
		if !strings.Contains(out, want) {
			t.Errorf("report lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "opaque-test-value-not-in-report") {
		t.Error("report printed the invalid domain")
	}
}
