package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

type statusResult struct {
	ProjectName  string
	Layers       []layerInfo
	Specs        specCounts
	Targets      []string
	LastSync     *time.Time
	FilesChanged *int
	DriftFiles   int
}

type layerInfo struct {
	Name string
	Path string
}

type specCounts struct {
	Agents   int
	Skills   int
	Rules    int
	Hooks    int
	MCPs     int
	Commands int
	Settings int
}

func newStatusCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show project configuration and sync state.",
		Example: `  # Print project status
  agnostic-ai status

  # Output as JSON for editor extensions or dashboards
  agnostic-ai status --json`,
		RunE: func(cmd *cobra.Command, args []string) error {
			r, err := gatherStatus(".")
			if err != nil {
				return err
			}
			if jsonOut {
				return printStatusJSON(cmd, r)
			}
			printStatus(cmd, r)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	return cmd
}

func gatherStatus(projectRoot string) (*statusResult, error) {
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	cfg, b, err := loadProject(projectRoot)
	if err != nil {
		return nil, err
	}

	driftFiles, allPaths, err := captureAllAndDiff(cfg.Targets, cfg, b)
	if err != nil {
		return nil, err
	}

	syncedAt, filesChanged := readSyncState(projectRoot, allPaths)

	return &statusResult{
		ProjectName:  filepath.Base(abs),
		Layers:       buildLayerInfos(projectRoot, cfg),
		Specs:        countSpecs(b),
		Targets:      cfg.Targets,
		LastSync:     syncedAt,
		FilesChanged: filesChanged,
		DriftFiles:   driftFiles,
	}, nil
}

// captureAllAndDiff runs every target adapter in capture mode, counts files
// whose on-disk content differs from what would be emitted, and returns all
// would-be emitted paths (for mtime fallback).
func captureAllAndDiff(targets []string, cfg *config.Config, b spec.Bundle) (driftFiles int, allPaths []string, err error) {
	for _, t := range targets {
		adapter, err := adapters.Resolve(t)
		if err != nil {
			continue
		}
		adapters.StartCapture()
		if emitErr := adapters.EmitWithProvenance(adapter, b, cfg, false); emitErr != nil {
			adapters.StopCapture()
			return 0, nil, fmt.Errorf("%s: %w", t, emitErr)
		}
		files := adapters.StopCapture()
		for _, f := range files {
			allPaths = append(allPaths, f.Path)
			disk, readErr := os.ReadFile(f.Path)
			if os.IsNotExist(readErr) {
				driftFiles++
				continue
			}
			if readErr != nil {
				return 0, nil, fmt.Errorf("read %s: %w", f.Path, readErr)
			}
			if string(disk) != f.Content {
				driftFiles++
			}
		}
	}
	return driftFiles, allPaths, nil
}

// readSyncState reads the state file written by sync. If absent or unreadable,
// falls back to the newest mtime across fallbackPaths. Returns (nil, nil) when
// no timestamp can be determined.
func readSyncState(projectRoot string, fallbackPaths []string) (syncedAt *time.Time, filesChanged *int) {
	data, err := os.ReadFile(stateFilePath(projectRoot))
	if err == nil {
		var s syncStateFile
		if json.Unmarshal(data, &s) == nil && !s.SyncedAt.IsZero() {
			fc := s.FilesChanged
			return &s.SyncedAt, &fc
		}
	}
	var newest time.Time
	for _, p := range fallbackPaths {
		info, statErr := os.Stat(p)
		if statErr != nil {
			continue
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	if !newest.IsZero() {
		return &newest, nil
	}
	return nil, nil
}

// buildLayerInfos returns display-friendly layer entries. The display path for
// each layer is the directory that contains the spec subdirectories.
func buildLayerInfos(projectRoot string, cfg *config.Config) []layerInfo {
	layers := resolveLayers(projectRoot, cfg)
	infos := make([]layerInfo, 0, len(layers))
	for _, l := range layers {
		displayPath := filepath.Dir(filepath.Join(l.Root, l.Sources.Agents))
		if rel, relErr := filepath.Rel(projectRoot, displayPath); relErr == nil && !strings.HasPrefix(rel, "..") {
			displayPath = rel
		}
		infos = append(infos, layerInfo{Name: l.Name, Path: displayPath + "/"})
	}
	return infos
}

func countSpecs(b spec.Bundle) specCounts {
	return specCounts{
		Agents:   len(b.Agents),
		Skills:   len(b.Skills),
		Rules:    len(b.Rules),
		Hooks:    len(b.Hooks),
		MCPs:     len(b.MCPs),
		Commands: len(b.Commands),
		Settings: len(b.Settings),
	}
}

func printStatus(cmd *cobra.Command, r *statusResult) {
	cmd.Printf("Project: %s\n", r.ProjectName)

	parts := make([]string, len(r.Layers))
	for i, l := range r.Layers {
		parts[i] = fmt.Sprintf("%s (%s)", l.Name, l.Path)
	}
	cmd.Printf("Layers:  %s\n", strings.Join(parts, ", "))

	cmd.Printf("Specs:   %s\n", formatSpecCounts(r.Specs))

	cmd.Printf("Targets: %s\n", strings.Join(r.Targets, ", "))

	switch {
	case r.LastSync == nil:
		cmd.Printf("Last sync: unknown\n")
	case r.FilesChanged != nil:
		cmd.Printf("Last sync: %s (%d file%s changed)\n",
			r.LastSync.Local().Format("2006-01-02 15:04"), *r.FilesChanged, pluralS(*r.FilesChanged))
	default:
		cmd.Printf("Last sync: %s\n", r.LastSync.Local().Format("2006-01-02 15:04"))
	}

	if r.DriftFiles == 0 {
		cmd.Printf("Drift:   in sync\n")
	} else {
		cmd.Printf("Drift:   %d file%s out of date (run `agnostic-ai sync` or `agnostic-ai doctor --fix`)\n",
			r.DriftFiles, pluralS(r.DriftFiles))
	}
}

func printStatusJSON(cmd *cobra.Command, r *statusResult) error {
	type layerJSON struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	type specJSON struct {
		Agents   int `json:"agents"`
		Skills   int `json:"skills"`
		Rules    int `json:"rules"`
		Hooks    int `json:"hooks"`
		MCPs     int `json:"mcps"`
		Commands int `json:"commands"`
		Settings int `json:"settings"`
	}
	type statusJSON struct {
		Project              string      `json:"project"`
		Layers               []layerJSON `json:"layers"`
		Specs                specJSON    `json:"specs"`
		Targets              []string    `json:"targets"`
		LastSync             *string     `json:"last_sync"`
		FilesChangedLastSync *int        `json:"files_changed_last_sync"`
		DriftFiles           int         `json:"drift_files"`
	}

	out := statusJSON{
		Project: r.ProjectName,
		Layers:  make([]layerJSON, len(r.Layers)),
		Specs: specJSON{
			Agents:   r.Specs.Agents,
			Skills:   r.Specs.Skills,
			Rules:    r.Specs.Rules,
			Hooks:    r.Specs.Hooks,
			MCPs:     r.Specs.MCPs,
			Commands: r.Specs.Commands,
			Settings: r.Specs.Settings,
		},
		Targets:              r.Targets,
		FilesChangedLastSync: r.FilesChanged,
		DriftFiles:           r.DriftFiles,
	}
	for i, l := range r.Layers {
		out.Layers[i] = layerJSON(l)
	}
	if r.LastSync != nil {
		s := r.LastSync.UTC().Format(time.RFC3339)
		out.LastSync = &s
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func formatSpecCounts(s specCounts) string {
	var parts []string
	if s.Rules > 0 {
		parts = append(parts, fmt.Sprintf("%d rules", s.Rules))
	}
	if s.Agents > 0 {
		parts = append(parts, fmt.Sprintf("%d agents", s.Agents))
	}
	if s.Skills > 0 {
		parts = append(parts, fmt.Sprintf("%d skills", s.Skills))
	}
	if s.Hooks > 0 {
		parts = append(parts, fmt.Sprintf("%d hooks", s.Hooks))
	}
	if s.MCPs > 0 {
		parts = append(parts, fmt.Sprintf("%d mcps", s.MCPs))
	}
	if s.Commands > 0 {
		parts = append(parts, fmt.Sprintf("%d commands", s.Commands))
	}
	if s.Settings > 0 {
		parts = append(parts, fmt.Sprintf("%d settings", s.Settings))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
