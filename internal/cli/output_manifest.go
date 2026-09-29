package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// outputManifestPath is the committed list sync.output-manifest writes.
const outputManifestPath = config.SourceBaseDir + "/outputs.lock"

// renderOutputManifest lists each output as `<sum>  <path>`, sorted by
// path, under the provenance header. The sum is the ContentSum of what
// sync writes, so a file edited since does not count as proven. The spec
// source and the manifest itself are not outputs, so they are left out.
func renderOutputManifest(sums map[string]string) string {
	paths := make([]string, 0, len(sums))
	bySlash := map[string]string{}
	for p, sum := range sums {
		p = filepath.ToSlash(p)
		if p == adapters.AgnosticEntryPointPath || p == outputManifestPath {
			continue
		}
		if _, dup := bySlash[p]; !dup {
			paths = append(paths, p)
		}
		bySlash[p] = sum
	}
	sort.Strings(paths)
	var sb strings.Builder
	for _, p := range paths {
		sb.WriteString(bySlash[p] + "  " + p + "\n")
	}
	return header.With(sb.String(), header.FormatShell)
}

// readOutputManifest returns the sum the manifest on disk lists for each
// path, or nil when there is none.
func readOutputManifest() map[string]string {
	data, err := os.ReadFile(outputManifestPath)
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if sum, p, ok := strings.Cut(line, "  "); ok {
			out[strings.TrimSpace(p)] = sum
		}
	}
	return out
}

// writeOutputManifest writes the manifest for a complete sync of every
// configured target, when the config asks for one, from the sums of what
// it wrote, and returns the write events.
func writeOutputManifest(sess *adapters.Session, cfg *config.Config, sums map[string]string, complete, dryRun bool) ([]adapters.WrittenFile, error) {
	if !cfg.Sync.OutputManifest || !complete || dryRun {
		return nil, nil
	}
	sess.StartDetailedRecording()
	err := sess.WriteFile(outputManifestPath, renderOutputManifest(sums), dryRun)
	return sess.StopDetailedRecording(), err
}

// checkOutputManifest compares the manifest on disk with the one a full
// sync would write for the outputs a check rendered, and files it in rep.
func checkOutputManifest(cfg *config.Config, rep *driftReport, sums map[string]string) error {
	if !cfg.Sync.OutputManifest {
		return nil
	}
	f := adapters.CapturedFile{Path: outputManifestPath, Content: renderOutputManifest(sums)}
	disk, err := os.ReadFile(f.Path)
	switch {
	case notOnDisk(err):
		rep.Missing = append(rep.Missing, f)
	case err != nil:
		return err
	case string(disk) != f.Content:
		rep.Stale = append(rep.Stale, f)
	default:
		rep.Current = append(rep.Current, f)
	}
	return nil
}
