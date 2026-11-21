//go:build darwin

package cli

import (
	"context"
	"crypto/sha1" //nolint:gosec // codesign names a certificate by its SHA-1 hash.
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// The companion is signed with a certificate made on this Mac, kept in a
// keychain of its own in the companion folder. macOS keeps a privacy
// grant, such as Accessibility, against the app's designated requirement.
// Pinning that requirement to the certificate keeps it the same across
// installs, while an app that only copies the bundle identifier fails it.
const (
	companionSignerCN       = "agnostic-ai companion (this Mac)"
	companionSignerKeychain = "signer.keychain-db"
	companionSignerCert     = "signer.cer"
	companionSignerScratch  = "signer-"
)

func signCompanion(ctx context.Context, folder, app string) error {
	lock, err := os.Open(folder)
	if err != nil {
		return fmt.Errorf("lock the companion signer: %w", err)
	}
	defer func() { _ = lock.Close() }()
	if err := tryLockProjectFile(lock); err != nil {
		if projectLockContended(err) {
			return fmt.Errorf("another companion install is signing; retry when it finishes: %w", err)
		}
		return fmt.Errorf("lock the companion signer in %s: %w", folder, err)
	}
	hash, keychain, err := companionSigner(ctx, folder)
	if err != nil {
		return fmt.Errorf("make this Mac's signing certificate: %w", err)
	}
	// The partition list lets codesign use the key without a dialog on headless runners.
	for _, args := range [][]string{
		{"unlock-keychain", "-p", "", keychain},
		{"set-key-partition-list", "-S", "apple-tool:,apple:,codesign:", "-s", "-k", "", keychain},
	} {
		if err := runSecurity(ctx, args...); err != nil {
			return err
		}
	}
	requirement := "-r=designated => " + companionRequirement(hash)
	return withKeychainSearched(ctx, keychain, func() error {
		out, err := exec.CommandContext(ctx, "codesign", "--force", "--keychain", keychain, "--sign", hash, "--identifier", companionBundleID, requirement, app).CombinedOutput()
		if err != nil {
			// What security sees explains a "no identity found".
			found, _ := exec.CommandContext(ctx, "security", "find-identity", "-p", "codesigning", keychain).CombinedOutput()
			return fmt.Errorf("%w\n%s", withWhatItSaid(err, out), strings.TrimSpace(string(found)))
		}
		return nil
	})
}

// Some macOS versions find a signing identity only in a listed keychain,
// whatever codesign --keychain says.
func withKeychainSearched(ctx context.Context, keychain string, sign func() error) error {
	paths, err := userKeychainPaths(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(paths, keychain) {
		return sign()
	}
	if err := setUserKeychainPaths(ctx, append(paths, keychain)); err != nil {
		return err
	}
	signErr := sign()
	ctx = context.WithoutCancel(ctx)
	paths, err = userKeychainPaths(ctx)
	if err != nil {
		return errors.Join(signErr, err)
	}
	if !slices.Contains(paths, keychain) {
		return signErr
	}
	paths = slices.DeleteFunc(paths, func(path string) bool { return path == keychain })
	return errors.Join(signErr, setUserKeychainPaths(ctx, paths))
}

func userKeychainPaths(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "security", "list-keychains", "-d", "user").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("security list-keychains: %w", withWhatItSaid(err, out))
	}
	var paths []string
	for line := range strings.Lines(string(out)) {
		if path := strings.Trim(strings.TrimSpace(line), `"`); path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

func setUserKeychainPaths(ctx context.Context, paths []string) error {
	return runSecurity(ctx, append([]string{"list-keychains", "-d", "user", "-s"}, paths...)...)
}

func companionRequirement(certHash string) string {
	return fmt.Sprintf(`identifier %q and certificate leaf = H"%s"`, companionBundleID, certHash)
}

// companionSigner returns the signing certificate's SHA-1 hash and its
// keychain, making both on first use. The certificate file is written
// last, so a keychain without it is from an install that stopped midway.
// A keychain that does not hold the recorded certificate is made again.
func companionSigner(ctx context.Context, folder string) (hash, keychain string, err error) {
	resolved, err := filepath.EvalSymlinks(folder)
	if err != nil {
		return "", "", err
	}
	keychain, err = filepath.Abs(filepath.Join(resolved, companionSignerKeychain))
	if err != nil {
		return "", "", err
	}
	if err := removeSignerScratch(folder); err != nil {
		return "", "", err
	}
	certFile := filepath.Join(folder, companionSignerCert)
	der, err := os.ReadFile(certFile)
	switch {
	case err == nil:
		hash := certHash(der)
		holds, err := keychainHolds(ctx, keychain, hash)
		if err != nil {
			return "", "", err
		}
		if holds {
			return hash, keychain, nil
		}
	case !errors.Is(err, fs.ErrNotExist):
		return "", "", err
	}
	for _, stale := range []string{certFile, keychain} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
	}
	der, err = makeCompanionSigner(ctx, folder, keychain)
	if err != nil {
		_ = os.Remove(keychain)
		return "", "", err
	}
	if err := os.WriteFile(certFile, der, 0o600); err != nil {
		return "", "", err
	}
	return certHash(der), keychain, nil
}

// companionSignerConfig asks openssl for a self-signed certificate that
// can sign code, valid for 100 years.
const companionSignerConfig = `[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = ` + companionSignerCN + `
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
`

// makeCompanionSigner makes the certificate with the system openssl and
// imports it, with its key, as one .p12, the same way the release signs.
// The .p12 password guards nothing, since the file is deleted at once;
// macOS refuses a .p12 without one.
func makeCompanionSigner(ctx context.Context, folder, keychain string) ([]byte, error) {
	scratch, err := os.MkdirTemp(folder, companionSignerScratch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	file := func(name string) string { return filepath.Join(scratch, name) }
	if err := os.WriteFile(file("cert.cnf"), []byte(companionSignerConfig), 0o600); err != nil {
		return nil, err
	}
	for _, args := range [][]string{
		{"req", "-x509", "-newkey", "rsa:2048", "-nodes", "-keyout", file("key.pem"), "-out", file("cert.pem"), "-days", "36500", "-config", file("cert.cnf")},
		{"pkcs12", "-export", "-inkey", file("key.pem"), "-in", file("cert.pem"), "-out", file("id.p12"), "-passout", "pass:agnostic-ai"},
		{"x509", "-in", file("cert.pem"), "-outform", "der", "-out", file("cert.der")},
	} {
		if out, err := exec.CommandContext(ctx, "/usr/bin/openssl", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("openssl %s: %w", args[0], withWhatItSaid(err, out))
		}
	}
	for _, args := range [][]string{
		{"create-keychain", "-p", "", keychain},
		{"set-keychain-settings", keychain},
		{"unlock-keychain", "-p", "", keychain},
		{"import", file("id.p12"), "-k", keychain, "-P", "agnostic-ai", "-T", "/usr/bin/codesign"},
	} {
		if err := runSecurity(ctx, args...); err != nil {
			return nil, err
		}
	}
	return os.ReadFile(file("cert.der"))
}

// removeSignerScratch deletes the key folders an install that was killed
// while making the keychain left behind.
func removeSignerScratch(folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), companionSignerScratch) {
			if err := os.RemoveAll(filepath.Join(folder, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func keychainHolds(ctx context.Context, keychain, hash string) (bool, error) {
	if _, err := os.Stat(keychain); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	out, err := exec.CommandContext(ctx, "security", "find-certificate", "-a", "-Z", keychain).CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("security find-certificate %s: %w", keychain, withWhatItSaid(err, out))
	}
	for line := range strings.Lines(string(out)) {
		if strings.TrimSpace(line) == "SHA-1 hash: "+hash {
			return true, nil
		}
	}
	return false, nil
}

func runSecurity(ctx context.Context, args ...string) error {
	out, err := exec.CommandContext(ctx, "security", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("security %s: %w", args[0], withWhatItSaid(err, out))
	}
	return nil
}

func certHash(der []byte) string {
	sum := sha1.Sum(der) //nolint:gosec // see the import.
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}
