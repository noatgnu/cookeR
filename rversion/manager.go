// Package rversion downloads, verifies, and installs portable R runtimes published by github.com/noatgnu/r-portable.
package rversion

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

const defaultReleasesURL = "https://api.github.com/repos/noatgnu/r-portable/releases"

// Manager installs and manages portable R versions under a single install directory.
type Manager struct {
	InstallDir  string
	ReleasesURL string
	Client      *http.Client
}

// New returns a Manager rooted at installDir, using the default r-portable releases API.
func New(installDir string) *Manager {
	return &Manager{
		InstallDir:  installDir,
		ReleasesURL: defaultReleasesURL,
		Client:      http.DefaultClient,
	}
}

// Release describes one available R version for the current platform.
type Release struct {
	Version       string `json:"version"`
	AssetURL      string `json:"assetUrl"`
	ChecksumURL   string `json:"checksumUrl"`
	AssetFileName string `json:"assetFileName"`
}

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Linux assets are per-distro (e.g. "r-portable-4.5.2-ubuntu-22.04-x86_64.tar.xz"); macOS and Windows each have a single build.
var (
	rVersionPattern     = `[0-9]+(?:\.[0-9]+)+`
	linuxAssetPattern   = regexp.MustCompile(`^r-portable-(` + rVersionPattern + `)-([a-z]+-[0-9]+(?:\.[0-9]+)*)-x86_64\.tar\.xz$`)
	darwinAssetPattern  = regexp.MustCompile(`^r-portable-(` + rVersionPattern + `)-macos-arm64\.tar\.xz$`)
	windowsAssetPattern = regexp.MustCompile(`^r-portable-(` + rVersionPattern + `)-windows-x86_64\.zip$`)
)

// LinuxFallbackSuffixes orders Linux asset suffixes oldest-glibc-first, the most broadly compatible fallback when there's no exact or family match.
var LinuxFallbackSuffixes = []string{"almalinux-8", "debian-11", "ubuntu-22.04", "almalinux-9", "debian-12", "ubuntu-24.04"}

// linuxDistroFamilies maps an /etc/os-release ID to same-family r-portable asset suffixes, newest first.
var linuxDistroFamilies = map[string][]string{
	"ubuntu":    {"ubuntu-24.04", "ubuntu-22.04"},
	"debian":    {"debian-12", "debian-11"},
	"rhel":      {"almalinux-9", "almalinux-8"},
	"rocky":     {"almalinux-9", "almalinux-8"},
	"almalinux": {"almalinux-9", "almalinux-8"},
	"centos":    {"almalinux-9", "almalinux-8"},
	"fedora":    {"almalinux-9", "almalinux-8"},
}

// linuxDistroInfo holds the fields read from /etc/os-release that identify the running distro.
type linuxDistroInfo struct {
	id        string
	versionID string
}

// detectLinuxDistro reads /etc/os-release for the running system's ID and VERSION_ID.
func detectLinuxDistro() (linuxDistroInfo, error) {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return linuxDistroInfo{}, err
	}
	defer f.Close()

	var info linuxDistroInfo
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "ID="):
			info.id = strings.Trim(strings.TrimPrefix(line, "ID="), `"`)
		case strings.HasPrefix(line, "VERSION_ID="):
			info.versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}
	return info, scanner.Err()
}

// linuxAssetSuffix picks the best available r-portable Linux asset suffix: exact match, then same-family, then broadest fallback.
func linuxAssetSuffix(info linuxDistroInfo, available map[string]bool) (string, error) {
	if exact := info.id + "-" + info.versionID; available[exact] {
		return exact, nil
	}
	for _, candidate := range linuxDistroFamilies[info.id] {
		if available[candidate] {
			return candidate, nil
		}
	}
	for _, candidate := range LinuxFallbackSuffixes {
		if available[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no compatible r-portable Linux build found for %s %s", info.id, info.versionID)
}

// ListAvailableRVersions queries r-portable's GitHub releases for the asset matching the current platform/arch.
func (m *Manager) ListAvailableRVersions() ([]Release, error) {
	req, err := http.NewRequest(http.MethodGet, m.ReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := m.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status fetching releases: %s", resp.Status)
	}

	var releases []ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse releases response: %w", err)
	}

	var found []Release
	for _, r := range releases {
		asset, ok, err := selectAssetForRelease(r)
		if err != nil {
			continue
		}
		if !ok {
			continue
		}
		found = append(found, asset)
	}

	return found, nil
}

// selectAssetForRelease picks the one asset (and its checksum sidecar) matching the current platform, applying Linux distro fallback where needed.
func selectAssetForRelease(r ghRelease) (Release, bool, error) {
	assetsByName := make(map[string]string, len(r.Assets))
	for _, a := range r.Assets {
		assetsByName[a.Name] = a.BrowserDownloadURL
	}

	toRelease := func(name, version string) (Release, bool, error) {
		checksumURL, ok := assetsByName[name+".sha256"]
		if !ok {
			return Release{}, false, nil
		}
		return Release{
			Version:       version,
			AssetURL:      assetsByName[name],
			ChecksumURL:   checksumURL,
			AssetFileName: name,
		}, true, nil
	}

	switch runtime.GOOS {
	case "darwin":
		for _, a := range r.Assets {
			if match := darwinAssetPattern.FindStringSubmatch(a.Name); match != nil {
				return toRelease(a.Name, match[1])
			}
		}
		return Release{}, false, nil

	case "windows":
		for _, a := range r.Assets {
			if match := windowsAssetPattern.FindStringSubmatch(a.Name); match != nil {
				return toRelease(a.Name, match[1])
			}
		}
		return Release{}, false, nil

	default:
		var version string
		bySuffix := make(map[string]string) // distro suffix -> asset name
		for _, a := range r.Assets {
			match := linuxAssetPattern.FindStringSubmatch(a.Name)
			if match == nil {
				continue
			}
			version = match[1]
			bySuffix[match[2]] = a.Name
		}
		if len(bySuffix) == 0 {
			return Release{}, false, nil
		}

		info, err := detectLinuxDistro()
		if err != nil {
			return Release{}, false, fmt.Errorf("failed to detect Linux distro: %w", err)
		}
		available := make(map[string]bool, len(bySuffix))
		for suffix := range bySuffix {
			available[suffix] = true
		}
		suffix, err := linuxAssetSuffix(info, available)
		if err != nil {
			return Release{}, false, err
		}
		return toRelease(bySuffix[suffix], version)
	}
}

// InstallRVersion downloads, verifies, and extracts the given release into InstallDir/<version>.
func (m *Manager) InstallRVersion(release Release, progress func(string)) error {
	if progress == nil {
		progress = func(string) {}
	}

	destDir := filepath.Join(m.InstallDir, release.Version)
	if _, err := os.Stat(destDir); err == nil {
		return fmt.Errorf("R %s is already installed at %s", release.Version, destDir)
	}

	if err := os.MkdirAll(m.InstallDir, 0755); err != nil {
		return err
	}

	// Staged inside InstallDir, not the OS temp dir: os.Rename can't cross drive letters/volumes on Windows, and CI runners commonly put the workspace and the OS temp dir on different drives.
	tmpDir, err := os.MkdirTemp(m.InstallDir, "tmp-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, release.AssetFileName)

	progress(fmt.Sprintf("Downloading %s...", release.AssetFileName))
	if err := m.downloadFile(release.AssetURL, archivePath); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}

	progress("Verifying checksum...")
	expectedHash, err := m.downloadChecksum(release.ChecksumURL, release.AssetFileName)
	if err != nil {
		return fmt.Errorf("failed to fetch checksum: %w", err)
	}
	actualHash, err := calculateSHA256(archivePath)
	if err != nil {
		return fmt.Errorf("failed to hash downloaded archive: %w", err)
	}
	if !strings.EqualFold(expectedHash, actualHash) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedHash, actualHash)
	}

	progress("Extracting...")
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(extractDir, 0755); err != nil {
		return err
	}
	if strings.HasSuffix(release.AssetFileName, ".zip") {
		if err := extractZip(archivePath, extractDir); err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}
	} else {
		if err := extractTarXz(archivePath, extractDir); err != nil {
			return fmt.Errorf("extraction failed: %w", err)
		}
	}

	if err := os.Rename(extractDir, destDir); err != nil {
		return fmt.Errorf("failed to move extracted R into place: %w", err)
	}

	progress(fmt.Sprintf("R %s installed to %s", release.Version, destDir))
	return nil
}

// ListInstalledRVersions returns the version directory names present under InstallDir.
func (m *Manager) ListInstalledRVersions() ([]string, error) {
	entries, err := os.ReadDir(m.InstallDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var versions []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), "tmp-install-") {
			versions = append(versions, e.Name())
		}
	}
	return versions, nil
}

// GetRPath returns the path to the Rscript binary for an installed version.
func (m *Manager) GetRPath(version string) (string, error) {
	binName := "Rscript"
	if runtime.GOOS == "windows" {
		binName = "Rscript.exe"
	}
	path := filepath.Join(m.InstallDir, version, "bin", binName)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("R %s is not installed (expected %s): %w", version, path, err)
	}
	return path, nil
}

// UninstallRVersion removes an installed R version's directory.
func (m *Manager) UninstallRVersion(version string) error {
	dir := filepath.Join(m.InstallDir, version)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("R %s is not installed: %w", version, err)
	}
	return os.RemoveAll(dir)
}

func (m *Manager) downloadFile(url, destPath string) error {
	resp, err := m.Client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// downloadChecksum fetches a "<hash>  <filename>" formatted sha256 sidecar and returns the hash.
func (m *Manager) downloadChecksum(url, expectedFileName string) (string, error) {
	resp, err := m.Client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	if !scanner.Scan() {
		return "", fmt.Errorf("empty checksum file")
	}
	fields := strings.Fields(scanner.Text())
	if len(fields) < 1 || len(fields[0]) != 64 {
		return "", fmt.Errorf("malformed checksum line: %q", scanner.Text())
	}
	return fields[0], nil
}

func calculateSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	buf := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func extractTarXz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	xzReader, err := xz.NewReader(bufio.NewReaderSize(f, 1024*1024))
	if err != nil {
		return err
	}

	tr := tar.NewReader(xzReader)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			os.Remove(target)
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		}
	}
	return nil
}

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			rc.Close()
			return err
		}
		out.Close()
		rc.Close()
	}
	return nil
}
