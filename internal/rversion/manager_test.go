package rversion

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ulikunitz/xz"
)

// testArchiveName returns an asset name selectAssetForRelease will pick on the current GOOS; Linux uses LinuxFallbackSuffixes[0], always selected as a last-resort match on any CI runner.
func testArchiveName(version string) string {
	switch runtime.GOOS {
	case "darwin":
		return fmt.Sprintf("r-portable-%s-macos-arm64.tar.xz", version)
	case "windows":
		return fmt.Sprintf("r-portable-%s-windows-x86_64.zip", version)
	default:
		return fmt.Sprintf("r-portable-%s-%s-x86_64.tar.xz", version, LinuxFallbackSuffixes[0])
	}
}

func fakeReleasesServer(t *testing.T, archiveBytes []byte, archiveName string) *httptest.Server {
	t.Helper()

	hash := sha256.Sum256(archiveBytes)
	checksumLine := fmt.Sprintf("%s  %s\n", hex.EncodeToString(hash[:]), archiveName)

	mux := http.NewServeMux()
	mux.HandleFunc("/releases", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `[{
			"tag_name": "r-4.4.2",
			"assets": [
				{"name": %q, "browser_download_url": "%s/download/%s"},
				{"name": %q, "browser_download_url": "%s/download/%s.sha256"}
			]
		}]`, archiveName, "http://"+r.Host, archiveName, archiveName+".sha256", "http://"+r.Host, archiveName)
	})
	mux.HandleFunc("/download/"+archiveName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(archiveBytes)
	})
	mux.HandleFunc("/download/"+archiveName+".sha256", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(checksumLine))
	})

	return httptest.NewServer(mux)
}

func buildFakeArchive(t *testing.T) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		return buildFakeZip(t)
	}
	return buildFakeTarXz(t)
}

func buildFakeTarXz(t *testing.T) []byte {
	t.Helper()

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	content := []byte("#!/bin/sh\necho fake-Rscript\n")
	if err := tw.WriteHeader(&tar.Header{Name: "bin/Rscript", Mode: 0755, Size: int64(len(content))}); err != nil {
		t.Fatalf("tar header error: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("tar write error: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error: %v", err)
	}

	var xzBuf bytes.Buffer
	xw, err := xz.NewWriter(&xzBuf)
	if err != nil {
		t.Fatalf("xz writer error: %v", err)
	}
	if _, err := xw.Write(tarBuf.Bytes()); err != nil {
		t.Fatalf("xz write error: %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close error: %v", err)
	}

	return xzBuf.Bytes()
}

func buildFakeZip(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("bin/Rscript.exe")
	if err != nil {
		t.Fatalf("zip create error: %v", err)
	}
	if _, err := w.Write([]byte("fake-Rscript")); err != nil {
		t.Fatalf("zip write error: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close error: %v", err)
	}
	return buf.Bytes()
}

func TestListAvailableRVersions(t *testing.T) {
	archiveName := testArchiveName("4.4.2")
	server := fakeReleasesServer(t, []byte("dummy"), archiveName)
	defer server.Close()

	m := New(t.TempDir())
	m.ReleasesURL = server.URL + "/releases"

	releases, err := m.ListAvailableRVersions()
	if err != nil {
		t.Fatalf("ListAvailableRVersions error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d: %+v", len(releases), releases)
	}
	if releases[0].Version != "4.4.2" {
		t.Errorf("expected version 4.4.2, got %q", releases[0].Version)
	}
}

func TestListAvailableRVersions_IgnoresOtherPlatforms(t *testing.T) {
	archiveName := "r-portable-4.4.2-some-other-platform.tar.xz"
	server := fakeReleasesServer(t, []byte("dummy"), archiveName)
	defer server.Close()

	m := New(t.TempDir())
	m.ReleasesURL = server.URL + "/releases"

	releases, err := m.ListAvailableRVersions()
	if err != nil {
		t.Fatalf("ListAvailableRVersions error: %v", err)
	}
	if len(releases) != 0 {
		t.Fatalf("expected 0 releases for a foreign platform asset, got %d: %+v", len(releases), releases)
	}
}

func TestInstallRVersion_Success(t *testing.T) {
	archiveName := testArchiveName("4.4.2")
	archiveBytes := buildFakeArchive(t)
	server := fakeReleasesServer(t, archiveBytes, archiveName)
	defer server.Close()

	installDir := t.TempDir()
	m := New(installDir)
	m.ReleasesURL = server.URL + "/releases"

	releases, err := m.ListAvailableRVersions()
	if err != nil {
		t.Fatalf("ListAvailableRVersions error: %v", err)
	}
	if len(releases) != 1 {
		t.Fatalf("expected 1 release, got %d", len(releases))
	}

	var progressMessages []string
	if err := m.InstallRVersion(releases[0], func(msg string) { progressMessages = append(progressMessages, msg) }); err != nil {
		t.Fatalf("InstallRVersion error: %v", err)
	}
	if len(progressMessages) == 0 {
		t.Error("expected progress messages during install")
	}

	installed, err := m.ListInstalledRVersions()
	if err != nil {
		t.Fatalf("ListInstalledRVersions error: %v", err)
	}
	if len(installed) != 1 || installed[0] != "4.4.2" {
		t.Fatalf("expected [4.4.2] installed, got %v", installed)
	}

	rscriptPath, err := m.GetRPath("4.4.2")
	if err != nil {
		t.Fatalf("GetRPath error: %v", err)
	}
	if _, err := os.Stat(rscriptPath); err != nil {
		t.Errorf("expected Rscript to exist at %s: %v", rscriptPath, err)
	}

	if err := m.UninstallRVersion("4.4.2"); err != nil {
		t.Fatalf("UninstallRVersion error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "4.4.2")); !os.IsNotExist(err) {
		t.Error("expected version directory to be removed after uninstall")
	}
}

func TestInstallRVersion_ChecksumMismatchFails(t *testing.T) {
	archiveName := testArchiveName("4.4.2")
	archiveBytes := buildFakeArchive(t)
	server := fakeReleasesServer(t, archiveBytes, archiveName)
	defer server.Close()

	m := New(t.TempDir())
	m.ReleasesURL = server.URL + "/releases"

	releases, err := m.ListAvailableRVersions()
	if err != nil {
		t.Fatalf("ListAvailableRVersions error: %v", err)
	}

	// Corrupt the checksum URL so it points at a checksum for different content.
	releases[0].ChecksumURL = server.URL + "/download/" + archiveName // serves the archive bytes, not a checksum -- guaranteed mismatch/parse failure

	if err := m.InstallRVersion(releases[0], nil); err == nil {
		t.Error("expected an error when checksum verification fails, got nil")
	}
}

func TestGetRPath_NotInstalled(t *testing.T) {
	m := New(t.TempDir())
	if _, err := m.GetRPath("9.9.9"); err == nil {
		t.Error("expected error for a version that isn't installed, got nil")
	}
}

func TestUninstallRVersion_NotInstalled(t *testing.T) {
	m := New(t.TempDir())
	if err := m.UninstallRVersion("9.9.9"); err == nil {
		t.Error("expected error uninstalling a version that isn't installed, got nil")
	}
}

func TestLinuxAssetSuffix_ExactMatch(t *testing.T) {
	available := map[string]bool{"ubuntu-22.04": true, "debian-11": true}
	got, err := linuxAssetSuffix(linuxDistroInfo{id: "ubuntu", versionID: "22.04"}, available)
	if err != nil {
		t.Fatalf("linuxAssetSuffix error: %v", err)
	}
	if got != "ubuntu-22.04" {
		t.Errorf("expected exact match ubuntu-22.04, got %q", got)
	}
}

func TestLinuxAssetSuffix_FamilyFallback(t *testing.T) {
	// Ubuntu 26.04 doesn't exist in the matrix yet; should fall back to the newest ubuntu build.
	available := map[string]bool{"ubuntu-24.04": true, "ubuntu-22.04": true}
	got, err := linuxAssetSuffix(linuxDistroInfo{id: "ubuntu", versionID: "26.04"}, available)
	if err != nil {
		t.Fatalf("linuxAssetSuffix error: %v", err)
	}
	if got != "ubuntu-24.04" {
		t.Errorf("expected family fallback ubuntu-24.04, got %q", got)
	}
}

func TestLinuxAssetSuffix_LastResortFallback(t *testing.T) {
	// An unlisted distro family (e.g. openSUSE) falls back to the broadest-compatible build.
	available := map[string]bool{"almalinux-8": true, "debian-11": true}
	got, err := linuxAssetSuffix(linuxDistroInfo{id: "opensuse", versionID: "15.5"}, available)
	if err != nil {
		t.Fatalf("linuxAssetSuffix error: %v", err)
	}
	if got != "almalinux-8" {
		t.Errorf("expected last-resort fallback almalinux-8, got %q", got)
	}
}

func TestLinuxAssetSuffix_NoneAvailable(t *testing.T) {
	if _, err := linuxAssetSuffix(linuxDistroInfo{id: "ubuntu", versionID: "22.04"}, map[string]bool{}); err == nil {
		t.Error("expected an error when no candidate suffix is available, got nil")
	}
}
