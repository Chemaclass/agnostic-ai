package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testReleaseArchive(t *testing.T, binary []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(binary))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binary); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestInstallReleaseBinary_ReplacesVerifiedBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("running binaries cannot be replaced on Windows")
	}
	version := "0.58.0"
	newBinary := []byte("#!/bin/sh\nprintf 'agnostic-ai version 0.58.0\\n'\n")
	archive := testReleaseArchive(t, newBinary)
	asset := fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v" + version + "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%x  %s\n", sum, asset)
		case "/v" + version + "/" + asset:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), binaryName)
	if err := os.WriteFile(path, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installReleaseBinary(path, version, server.URL, server.Client()); err != nil {
		t.Fatalf("install release binary: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Errorf("installed binary does not match release archive")
	}
}

func TestInstallReleaseBinary_LeavesOriginalOnVerificationFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("running binaries cannot be replaced on Windows")
	}
	version := "0.58.0"
	asset := fmt.Sprintf("%s_%s_%s.tar.gz", binaryName, runtime.GOOS, runtime.GOARCH)
	archive := testReleaseArchive(t, []byte("#!/bin/sh\nprintf 'agnostic-ai version 0.57.0\\n'\n"))
	sum := sha256.Sum256(archive)
	cases := []struct {
		name      string
		checksums string
		wantError string
	}{
		{"checksum mismatch", fmt.Sprintf("%064x  %s\n", 0, asset), "checksum mismatch"},
		{"missing checksum", "", "is missing"},
		{"version mismatch", fmt.Sprintf("%x  %s\n", sum, asset), "expected"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v" + version + "/checksums.txt":
					_, _ = io.WriteString(w, tc.checksums)
				case "/v" + version + "/" + asset:
					_, _ = w.Write(archive)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			path := filepath.Join(t.TempDir(), binaryName)
			old := []byte("old binary")
			if err := os.WriteFile(path, old, 0o755); err != nil {
				t.Fatal(err)
			}
			err := installReleaseBinary(path, version, server.URL, server.Client())
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("error = %v, want %q", err, tc.wantError)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, old) {
				t.Errorf("installed binary changed after failed verification")
			}
		})
	}
}
