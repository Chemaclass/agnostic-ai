package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// signalsPath is the capability signal history. Its date columns measure
// how long a vendor capability takes to reach an audit and then a release,
// so a row that drifts out of shape would quietly break that measurement.
const signalsPath = "../../scripts/target-audit/signals.tsv"

var signalDateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func TestAuditSignals_RowsKeepTheirColumns(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash(signalsPath))
	if err != nil {
		t.Fatalf("read %s: %v", signalsPath, err)
	}
	sources := map[string]bool{"": true, "docs": true, "changelog": true, "schema": true, "code": true}
	seen := map[string]bool{}
	for n, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 9 {
			t.Errorf("line %d has %d columns, want 9: %q", n+1, len(f), line)
			continue
		}
		id, firstSeen, lastSeen, vendorDate, firstSource, shippedDate := f[0], f[1], f[2], f[5], f[6], f[7]
		if seen[id] {
			t.Errorf("line %d repeats signal %s", n+1, id)
		}
		seen[id] = true
		for name, v := range map[string]string{"first-seen": firstSeen, "last-seen": lastSeen} {
			if !signalDateRE.MatchString(v) {
				t.Errorf("%s: %s %q is not YYYY-MM-DD", id, name, v)
			}
		}
		for name, v := range map[string]string{"vendor-date": vendorDate, "shipped-date": shippedDate} {
			if v != "" && !signalDateRE.MatchString(v) {
				t.Errorf("%s: %s %q is not YYYY-MM-DD or empty", id, name, v)
			}
		}
		if !sources[firstSource] {
			t.Errorf("%s: first-source %q is not docs, changelog, schema, code, or empty", id, firstSource)
		}
		if vendorDate != "" && vendorDate > firstSeen {
			t.Errorf("%s: vendor-date %s is after first-seen %s", id, vendorDate, firstSeen)
		}
		if vendorDate != "" && shippedDate != "" && shippedDate < vendorDate {
			t.Errorf("%s: shipped-date %s is before vendor-date %s", id, shippedDate, vendorDate)
		}
	}
}
