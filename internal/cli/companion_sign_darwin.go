//go:build darwin

package cli

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // codesign names a certificate by its SHA-1 hash.
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
	hash, keychain, err := companionSigner(ctx, folder)
	if err != nil {
		return fmt.Errorf("make this Mac's signing certificate: %w", err)
	}
	// The keychain has an empty password; it locks itself after a while.
	// The partition list lets codesign use the key without a dialog, which
	// a session with no screen, such as CI, cannot answer.
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
			return withWhatItSaid(err, out)
		}
		return nil
	})
}

// withKeychainSearched runs sign with keychain on the user's keychain
// search list, then puts the list back. Some macOS versions find a signing
// identity only in a listed keychain, whatever codesign --keychain says.
func withKeychainSearched(ctx context.Context, keychain string, sign func() error) error {
	out, err := exec.CommandContext(ctx, "security", "list-keychains", "-d", "user").Output()
	if err != nil {
		return fmt.Errorf("security list-keychains: %w", err)
	}
	var kept []string
	for line := range strings.Lines(string(out)) {
		if path := strings.Trim(strings.TrimSpace(line), `"`); path != "" && path != keychain {
			kept = append(kept, path)
		}
	}
	setList := func(ctx context.Context, paths ...string) error {
		return runSecurity(ctx, append([]string{"list-keychains", "-d", "user", "-s"}, paths...)...)
	}
	if err := setList(ctx, append(kept, keychain)...); err != nil {
		return err
	}
	signErr := sign()
	return errors.Join(signErr, setList(context.WithoutCancel(ctx), kept...))
}

func companionRequirement(certHash string) string {
	return fmt.Sprintf(`identifier %q and certificate leaf = H"%s"`, companionBundleID, certHash)
}

// companionSigner returns the signing certificate's SHA-1 hash and its
// keychain, making both on first use. The certificate file is written
// last, so a keychain without it is from an install that stopped midway.
// A keychain that does not hold the recorded certificate is made again.
func companionSigner(ctx context.Context, folder string) (hash, keychain string, err error) {
	keychain, err = filepath.Abs(filepath.Join(folder, companionSignerKeychain))
	if err != nil {
		return "", "", err
	}
	if err := removeSignerScratch(folder); err != nil {
		return "", "", err
	}
	certFile := filepath.Join(folder, companionSignerCert)
	if der, readErr := os.ReadFile(certFile); readErr == nil {
		hash := certHash(der)
		holds, err := keychainHolds(ctx, keychain, hash)
		if err != nil {
			return "", "", err
		}
		if holds {
			return hash, keychain, nil
		}
	}
	for _, stale := range []string{certFile, keychain} {
		if err := os.Remove(stale); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", "", err
		}
	}
	der, err := makeCompanionSigner(ctx, folder, keychain)
	if err != nil {
		_ = os.Remove(keychain)
		return "", "", err
	}
	if err := os.WriteFile(certFile, der, 0o600); err != nil {
		return "", "", err
	}
	return certHash(der), keychain, nil
}

func makeCompanionSigner(ctx context.Context, folder, keychain string) ([]byte, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: companionSignerCN},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(100, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	scratch, err := os.MkdirTemp(folder, companionSignerScratch)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	keyFile := filepath.Join(scratch, "key.pem")
	certPEM := filepath.Join(scratch, "cert.pem")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		return nil, err
	}
	for _, args := range [][]string{
		{"create-keychain", "-p", "", keychain},
		{"import", keyFile, "-k", keychain, "-f", "openssl", "-t", "priv", "-T", "/usr/bin/codesign"},
		{"import", certPEM, "-k", keychain},
	} {
		if err := runSecurity(ctx, args...); err != nil {
			return nil, err
		}
	}
	return der, nil
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

// keychainHolds reports whether keychain holds the certificate with hash.
// An interrupted check is an error, so the caller keeps a good keychain.
func keychainHolds(ctx context.Context, keychain, hash string) (bool, error) {
	if _, err := os.Stat(keychain); err != nil {
		return false, nil
	}
	out, err := exec.CommandContext(ctx, "security", "find-certificate", "-a", "-Z", keychain).Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, ctxErr
	}
	return err == nil && strings.Contains(string(out), "SHA-1 hash: "+hash), nil
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
