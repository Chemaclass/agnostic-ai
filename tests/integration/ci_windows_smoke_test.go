package integration

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestWindowsSmoke_PiCoverageStaysBounded(t *testing.T) {
	data, err := os.ReadFile(ciWorkflowPath)
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Timeout string         `yaml:"timeout-minutes"`
			Steps   []workflowStep `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var step workflowStep
	for _, candidate := range workflow.Jobs["test"].Steps {
		if candidate.Name == "Windows smoke" {
			step = candidate
			break
		}
	}
	if step.Name == "" {
		t.Fatal("CI has no Windows smoke step")
	}
	if !strings.Contains(step.Run, "./internal/adapters/pi") {
		t.Error("Windows smoke omits the Pi adapter package")
	}
	for path, names := range map[string][]string{
		"./internal/cli":      {"TestImportPi_NativeRoundTrip", "TestImportGlobal_PiAdoptsNativeFilesWithoutChangingBytes", "TestSyncGlobal_PiInvalidMCPPreservesManagedFiles", "TestPiExposure_GlobalUserReorderSurvivesRemovalAndRootMove", "TestPiBOM_ProjectImportAndInPlaceSync", "TestPiBOM_GlobalOwnedUpdateAndRemovalRestoreOriginalBytes"},
		"./tests/integration": {"TestPiRoundTrip_NativeProject"},
	} {
		var pattern string
		for _, line := range strings.Split(step.Run, "\n") {
			if strings.Contains(line, path) {
				match := regexp.MustCompile(`-run '([^']+)'`).FindStringSubmatch(line)
				if len(match) == 2 {
					pattern = match[1]
				}
				if strings.Contains(line, " -race ") {
					t.Errorf("Windows smoke uses race detection for %s", path)
				}
			}
		}
		if pattern == "" {
			t.Errorf("Windows smoke needs a bounded -run selector for %s", path)
			continue
		}
		selector, err := regexp.Compile(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range names {
			if !selector.MatchString(name) {
				t.Errorf("Windows smoke does not run %s", name)
			}
		}
	}
	if strings.Contains(step.Run, "./...") {
		t.Error("Windows smoke runs the whole repository suite")
	}
	if got := workflow.Jobs["test"].Timeout; got != "${{ matrix.os == 'windows-latest' && inputs.full_windows != true && 7 || 10 }}" {
		t.Errorf("Windows default job budget changed: %s", got)
	}
}
