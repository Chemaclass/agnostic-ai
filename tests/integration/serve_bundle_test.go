package integration

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestServe_InstalledBinaryContainsTheWebInterface(t *testing.T) {
	built := handoffBuildCLI(t)
	data, err := os.ReadFile(built)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "release.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	header := &zip.FileHeader{Name: filepath.Base(built), Method: zip.Store}
	header.SetMode(0o755)
	file, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	release, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = release.Close() }()
	packed, err := release.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = packed.Close() }()
	installed := filepath.Join(t.TempDir(), filepath.Base(built))
	extracted, err := os.OpenFile(installed, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(extracted, packed); err != nil {
		t.Fatal(err)
	}
	if err := extracted.Close(); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	writeProjectFile(t, project, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: []\n")
	t.Setenv("PATH", "")
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_NO_UPDATE_CHECK", "1")
	client := &http.Client{Timeout: 15 * time.Second}
	get := func(t *testing.T, url string, status int, contains string) string {
		t.Helper()
		resp, err := client.Get(url)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != status || !strings.Contains(string(data), contains) {
			t.Fatalf("%s: status %d, want %d containing %q", url, resp.StatusCode, status, contains)
		}
		return string(data)
	}
	t.Run("bundled", func(t *testing.T) {
		url := startInstalledServe(t, installed, project)
		html := get(t, url, http.StatusOK, `<div id="root"></div>`)
		assets := regexp.MustCompile(`(?:src|href)="([^" ]+\.(?:js|css))"`).FindAllStringSubmatch(html, -1)
		if len(assets) < 2 {
			t.Fatal("bundled client has no JavaScript and CSS assets")
		}
		for _, asset := range assets {
			body := get(t, strings.TrimRight(url, "/")+asset[1], http.StatusOK, "")
			if strings.Contains(body, "<!doctype html>") || body == "" {
				t.Errorf("asset %s did not return its contents", asset[1])
			}
		}
		get(t, url+"assets/missing.js", http.StatusNotFound, "")
		get(t, url+"api/missing", http.StatusNotFound, `"error"`)
		var overview struct{ SchemaVersion string }
		if err := json.Unmarshal([]byte(get(t, url+"api/views/overview", http.StatusOK, "")), &overview); err != nil || overview.SchemaVersion != "overview/v1" {
			t.Fatalf("overview = %+v, error %v", overview, err)
		}
	})
	t.Run("api only", func(t *testing.T) {
		url := startInstalledServe(t, installed, project, "--api-only")
		get(t, url, http.StatusNotFound, "no client")
		get(t, url+"api/health", http.StatusOK, `"ok":true`)
	})
	t.Run("override", func(t *testing.T) {
		directory := t.TempDir()
		writeProjectFile(t, directory, "index.html", "custom client")
		url := startInstalledServe(t, installed, project, "--ui", directory)
		get(t, url, http.StatusOK, "custom client")
	})
}

func startInstalledServe(t *testing.T, binary, project string, flags ...string) string {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binary, append([]string{"serve", "--addr", "127.0.0.1:0", "--dir", project}, flags...)...)
	cmd.Dir = t.TempDir()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	t.Cleanup(func() { cancel(); <-done })
	urls := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		pattern := regexp.MustCompile(`http://127\.0\.0\.1:\d+/`)
		for scanner.Scan() {
			if url := pattern.FindString(scanner.Text()); url != "" {
				urls <- url
				return
			}
		}
		close(urls)
	}()
	select {
	case url := <-urls:
		if url == "" {
			t.Fatal("server exited before reporting its URL")
		}
		return url
	case <-time.After(15 * time.Second):
		t.Fatal("server did not report its URL")
		return ""
	}
}
