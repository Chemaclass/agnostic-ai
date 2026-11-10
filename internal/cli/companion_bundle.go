package cli

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

// On macOS the companion is an app, so the system shows its name and
// icon wherever it lists it, such as among the items that open at login.
const (
	companionAppName  = "agnostic-ai companion"
	companionBundleID = "dev.agnostic-ai.companion"
	// companionIconName is the icon file in the app's Resources, without its extension.
	companionIconName = "companion"
)

// companionIconPNG is docs/site/static/icon-512.png, which go:embed cannot reach.
//
//go:embed companion-icon.png
var companionIconPNG []byte

// companionICNS wraps a 512-point PNG in the icon format macOS reads
// from an app, as its one image.
func companionICNS(png []byte) []byte {
	const header = 8
	var icns bytes.Buffer
	icns.WriteString("icns")
	_ = binary.Write(&icns, binary.BigEndian, uint32(2*header+len(png)))
	icns.WriteString("ic09")
	_ = binary.Write(&icns, binary.BigEndian, uint32(header+len(png)))
	icns.Write(png)
	return icns.Bytes()
}

func companionInfoPlist(version string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(version))
	return fmt.Appendf(nil, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleIdentifier</key>
	<string>%[1]s</string>
	<key>CFBundleName</key>
	<string>%[2]s</string>
	<key>CFBundleDisplayName</key>
	<string>%[2]s</string>
	<key>CFBundleExecutable</key>
	<string>%[3]s</string>
	<key>CFBundleIconFile</key>
	<string>%[4]s</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>%[5]s</string>
	<key>CFBundleVersion</key>
	<string>%[5]s</string>
	<key>LSUIElement</key>
	<true/>
	<key>LSMinimumSystemVersion</key>
	<string>11.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
</dict>
</plist>
`, companionBundleID, companionAppName, companionName, companionIconName, escaped.String())
}

func companionBundle(dir string) string {
	return filepath.Join(companionFolder(dir), companionAppName+".app")
}

// bareCompanionFile is where installs before the app put the companion on macOS.
func bareCompanionFile(dir string) string {
	return filepath.Join(companionFolder(dir), companionName)
}

// assembleCompanionApp makes app around the built program and signs it.
// macOS ties what the app registers, such as start at login, to its
// signature; a signature made on this machine, with no developer
// identity, is enough.
func assembleCompanionApp(ctx context.Context, tools companionTools, program, app, version string) error {
	contents := filepath.Join(app, "Contents")
	for _, folder := range []string{"MacOS", "Resources"} {
		if err := os.MkdirAll(filepath.Join(contents, folder), 0o700); err != nil {
			return err
		}
	}
	files := map[string][]byte{
		filepath.Join(contents, "Info.plist"):                           companionInfoPlist(version),
		filepath.Join(contents, "Resources", companionIconName+".icns"): companionICNS(companionIconPNG),
	}
	for path, content := range files {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			return err
		}
	}
	inner := filepath.Join(contents, "MacOS", companionName)
	if err := os.Rename(program, inner); err != nil {
		return err
	}
	if err := tools.sign(ctx, app); err != nil {
		return fmt.Errorf("sign the companion: %w", err)
	}
	return nil
}

// replaceCompanionApp puts the app built in staging where the installed
// one is. A folder cannot be renamed onto another, so the installed one
// moves aside first and comes back when the new one cannot take its place.
func replaceCompanionApp(built, installed, staging string) error {
	aside := filepath.Join(staging, "replaced.app")
	if err := os.Rename(installed, aside); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.Rename(built, installed); err != nil {
		if _, statErr := os.Stat(aside); statErr == nil {
			return errors.Join(err, os.Rename(aside, installed))
		}
		return err
	}
	return nil
}

func signCompanion(ctx context.Context, app string) error {
	out, err := exec.CommandContext(ctx, "codesign", "--force", "--sign", "-", app).CombinedOutput()
	if err != nil {
		return withWhatItSaid(err, out)
	}
	return nil
}

// startedAtLogin reports whether one of the login entries an earlier
// install left is there.
func startedAtLogin(entries []string) bool {
	for _, entry := range entries {
		if _, err := os.Stat(entry); err == nil {
			return true
		}
	}
	return false
}

// removeLoginEntries deletes the login entries that are left. It
// reports whether there was one.
func removeLoginEntries(entries []string) (bool, error) {
	left := false
	for _, entry := range entries {
		err := os.Remove(entry)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return left, err
		}
		left = left || err == nil
	}
	return left, nil
}

// leaveBareCompanion removes the bare program an install before the app
// left, with its launch agent, and moves its start at login to the app.
// alone says that no companion runs, so the app's own request reaches
// the system rather than a running bare program.
func leaveBareCompanion(ctx context.Context, out io.Writer, tools companionTools, bare, app string, loginEntries []string, alone bool) error {
	if found, err := companionExists(bare); err != nil || !found {
		return err
	}
	wasOn := startedAtLogin(loginEntries)
	if wasOn {
		// Its launch agent is removed below either way; this also unloads it.
		_ = tools.loginOff(ctx, bare)
	}
	if err := os.Remove(bare); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := removeLoginEntries(loginEntries); err != nil {
		return fmt.Errorf("turn off start at login: %w", err)
	}
	_, _ = fmt.Fprintf(out, "✓ Removed the earlier companion at %s\n", bare)
	switch {
	case !wasOn:
	case !alone:
		_, _ = fmt.Fprintln(out, "! Start at login was on. Turn it on again in the companion's settings.")
	default:
		if err := tools.loginOn(ctx, app); err != nil {
			_, _ = fmt.Fprintf(out, "! Start at login was on and could not be moved to the app (%v). Turn it on again in the companion's settings.\n", err)
			return nil
		}
		_, _ = fmt.Fprintln(out, "✓ Start at login stays on")
	}
	return nil
}
