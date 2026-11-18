package cli

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func companionAppForCommand(path string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	resources := filepath.Dir(path)
	contents := filepath.Dir(resources)
	app := filepath.Dir(contents)
	return app, filepath.Base(path) == binaryName && filepath.Base(resources) == "Resources" && filepath.Base(contents) == "Contents" && filepath.Ext(app) == ".app"
}

func downloadableCompanionApp(app string) (string, error) {
	if !filepath.IsAbs(app) || filepath.Ext(app) != ".app" {
		return "", fmt.Errorf("%s: expected an absolute app path", app)
	}
	resolved, err := filepath.EvalSymlinks(app)
	if err != nil {
		return "", fmt.Errorf("%s: %w", app, err)
	}
	for _, part := range []string{filepath.Join("Contents", "MacOS", companionName), filepath.Join("Contents", "Resources", binaryName)} {
		path := filepath.Join(resolved, part)
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("%s: %w", path, err)
		}
		if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
			return "", fmt.Errorf("%s: expected an executable file", path)
		}
	}
	return resolved, nil
}

func installCompanionCommand(out io.Writer, app string, folders []string, lookPath func(string) (string, error)) error {
	app, err := downloadableCompanionApp(app)
	if err != nil {
		return err
	}
	source := filepath.Join(app, "Contents", "Resources", binaryName)
	if existing, err := lookPath(binaryName); err == nil {
		resolved, _ := filepath.EvalSymlinks(existing)
		if resolved == source {
			_, err = fmt.Fprintf(out, "The command is already installed at %s\n", existing)
			return err
		}
		return fmt.Errorf("%s already supplies agnostic-ai; keep updating it through its install method", existing)
	}
	var failures []error
	for _, folder := range folders {
		target := filepath.Join(folder, binaryName)
		if existing, err := os.Readlink(target); err == nil && existing == source {
			_, err = fmt.Fprintf(out, "The command is already installed at %s\n", target)
			return err
		}
		if _, err := os.Lstat(target); err == nil {
			return fmt.Errorf("%s already exists; it was left unchanged", target)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s: %w", target, err)
		}
		if err := os.MkdirAll(folder, 0755); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", folder, err))
			continue
		}
		if err := os.Symlink(source, target); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", target, err))
			continue
		}
		_, err := fmt.Fprintf(out, "Installed the command at %s\nAdd %s to PATH if your terminal cannot find agnostic-ai.\n", target, folder)
		return err
	}
	return fmt.Errorf("install command line tool: %w", errors.Join(failures...))
}

type companionAppDeps struct {
	prepare func(context.Context, io.Writer, string, string) (string, error)
	start   func(string) error
}

func updateCompanionApp(ctx context.Context, out io.Writer, app, version string, tools companionTools, deps companionAppDeps) error {
	app, err := downloadableCompanionApp(app)
	if err != nil {
		return err
	}
	version, ok := stableReleaseVersion(version)
	if !ok {
		return fmt.Errorf("update %s: expected a stable release version", app)
	}
	staging, err := os.MkdirTemp(filepath.Dir(app), ".companion-update-")
	if err != nil {
		return fmt.Errorf("%s: stage update: %w", app, err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.RemoveAll(staging)
		}
	}()
	built, err := deps.prepare(ctx, out, version, staging)
	if err != nil {
		return fmt.Errorf("prepare %s: %w", app, err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("update %s: %w", app, err)
	}
	ctx = context.WithoutCancel(ctx)
	if _, err := endCompanion(ctx, tools, filepath.Join(app, "Contents", "MacOS", companionName)); err != nil {
		return err
	}
	err = replaceCompanionApp(tools.rename, built, app, staging)
	var aside companionKeptAside
	if errors.As(err, &aside) {
		keep = true
		return err
	}
	if startErr := deps.start(app); startErr != nil {
		return errors.Join(err, fmt.Errorf("start %s: %w", app, startErr))
	}
	if err != nil {
		return fmt.Errorf("replace %s: %w", app, err)
	}
	_, err = fmt.Fprintf(out, "Updated %s to %s and started it again.\n", app, version)
	return err
}

func downloadVerifiedCompanionAsset(ctx context.Context, out io.Writer, client *http.Client, baseURL, version, asset string, verify provenanceCheck) ([]byte, error) {
	checksums, err := downloadReleaseAssetContext(ctx, client, releaseAssetURL(baseURL, version, "checksums.txt"), maxChecksumsFile)
	if err != nil {
		return nil, fmt.Errorf("checksums.txt: %w", err)
	}
	want, err := releaseChecksum(checksums, asset)
	if err != nil {
		return nil, err
	}
	data, err := downloadReleaseAssetContext(ctx, client, releaseAssetURL(baseURL, version, asset), 128<<20)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", asset, err)
	}
	if sha256.Sum256(data) != want {
		return nil, fmt.Errorf("%s: checksum mismatch", asset)
	}
	if err := checkReleaseProvenance(out, data, asset, version, verify); err != nil {
		return nil, err
	}
	return data, nil
}

func prepareCompanionApp(ctx context.Context, out io.Writer, version, staging string) (string, error) {
	asset := fmt.Sprintf("%s_darwin_%s.dmg", companionName, machineArch())
	data, err := downloadVerifiedCompanionAsset(ctx, out, &http.Client{Timeout: companionDownloadWait}, releasesBaseURL, version, asset, ghProvenance)
	if err != nil {
		return "", err
	}
	image := filepath.Join(staging, asset)
	if err := os.WriteFile(image, data, 0600); err != nil {
		return "", fmt.Errorf("%s: %w", image, err)
	}
	mount := filepath.Join(staging, "mount")
	if err := os.Mkdir(mount, 0700); err != nil {
		return "", fmt.Errorf("%s: %w", mount, err)
	}
	if err := runCompanionAppTool(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mount, image); err != nil {
		return "", err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = runCompanionAppTool(cleanup, "hdiutil", "detach", mount)
	}()
	app := filepath.Join(staging, companionAppName+".app")
	if err := runCompanionAppTool(ctx, "ditto", filepath.Join(mount, companionAppName+".app"), app); err != nil {
		return "", err
	}
	if _, err := downloadableCompanionApp(app); err != nil {
		return "", err
	}
	if err := runCompanionAppTool(ctx, "codesign", "--verify", "--deep", "--strict", app); err != nil {
		return "", err
	}
	if err := verifyReleaseBinary(filepath.Join(app, "Contents", "Resources", binaryName), version); err != nil {
		return "", err
	}
	got, err := companionBuiltFor(ctx, filepath.Join(app, "Contents", "MacOS", companionName))
	if err != nil {
		return "", err
	}
	if got != version {
		return "", fmt.Errorf("%s: companion version %q, expected %s", app, got, version)
	}
	return app, nil
}

func runCompanionAppTool(ctx context.Context, name string, args ...string) error {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", name, withWhatItSaid(err, output))
	}
	return nil
}

func companionAppCommands(tools companionTools) []*cobra.Command {
	var installApp, updateApp, version string
	install := &cobra.Command{Use: "install-command", Short: "Link the app's bundled command into PATH.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("install command from %s: supported on macOS only", installApp)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("find home folder: %w", err)
		}
		folders := []string{filepath.Join(home, ".local", "bin")}
		if strings.Contains(os.Getenv("PATH"), "/usr/local/bin") {
			folders = append([]string{"/usr/local/bin"}, folders...)
		}
		return installCompanionCommand(cmd.OutOrStdout(), installApp, folders, exec.LookPath)
	}}
	install.Flags().StringVar(&installApp, "app", "", "Absolute path of the companion app.")
	_ = install.MarkFlagRequired("app")
	update := &cobra.Command{Use: "update-app", Short: "Download and replace the companion app and its bundled command.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if runtime.GOOS != "darwin" {
			return fmt.Errorf("update %s: supported on macOS only", updateApp)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), companionDownloadWait)
		defer cancel()
		return updateCompanionApp(ctx, cmd.OutOrStdout(), updateApp, version, tools, companionAppDeps{prepare: prepareCompanionApp, start: func(app string) error { return runCompanionAppTool(context.Background(), "open", "-a", app) }})
	}}
	update.Flags().StringVar(&updateApp, "app", "", "Absolute path of the companion app.")
	update.Flags().StringVar(&version, "version", "", "Stable release to install.")
	_ = update.MarkFlagRequired("app")
	_ = update.MarkFlagRequired("version")
	return []*cobra.Command{install, update}
}
