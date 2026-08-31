package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	original := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read pipe: %v", err)
	}
	return buf.String()
}

func TestCmdR_Dispatch(t *testing.T) {
	if err := cmdR(nil); err == nil {
		t.Error("expected usage error with no subcommand, got nil")
	}
	if err := cmdR([]string{"frobnicate"}); err == nil {
		t.Error("expected error for unknown r subcommand, got nil")
	}
}

func TestCmdRInstall_UsageError(t *testing.T) {
	if err := cmdRInstall(nil); err == nil {
		t.Error("expected usage error with no version, got nil")
	}
	if err := cmdRInstall([]string{"4.4.2", "extra"}); err == nil {
		t.Error("expected usage error with extra args, got nil")
	}
}

func TestCmdRUninstall_UsageError(t *testing.T) {
	if err := cmdRUninstall(nil); err == nil {
		t.Error("expected usage error with no version, got nil")
	}
}

func TestCmdRPath_UsageError(t *testing.T) {
	if err := cmdRPath(nil); err == nil {
		t.Error("expected usage error with no version, got nil")
	}
}

func TestCmdEnv_Dispatch(t *testing.T) {
	if err := cmdEnv(nil); err == nil {
		t.Error("expected usage error with no subcommand, got nil")
	}
	if err := cmdEnv([]string{"create", "/tmp/foo"}); err == nil {
		t.Error("expected 'not yet implemented' error, got nil")
	}
	if err := cmdEnv([]string{"frobnicate"}); err == nil {
		t.Error("expected error for unknown env subcommand, got nil")
	}
}

func TestCmdLib_Dispatch(t *testing.T) {
	if err := cmdLib(nil); err == nil {
		t.Error("expected usage error with no subcommand, got nil")
	}
	if err := cmdLib([]string{"install", "somepkg"}); err == nil {
		t.Error("expected 'not yet implemented' error, got nil")
	}
	if err := cmdLib([]string{"list"}); err == nil {
		t.Error("expected 'not yet implemented' error, got nil")
	}
	if err := cmdLib([]string{"frobnicate"}); err == nil {
		t.Error("expected error for unknown lib subcommand, got nil")
	}
}

func TestResolveInstallDir_RespectsEnvOverride(t *testing.T) {
	t.Setenv("COOKER_INSTALL_DIR", "/tmp/cooker-test-dir")
	dir, err := resolveInstallDir()
	if err != nil {
		t.Fatalf("resolveInstallDir error: %v", err)
	}
	if dir != "/tmp/cooker-test-dir" {
		t.Errorf("expected COOKER_INSTALL_DIR to be respected, got %q", dir)
	}
}

// TestCmdDoctor_Runs hits the real r-portable GitHub releases API (empty list is fine, network access is required).
func TestCmdDoctor_Runs(t *testing.T) {
	t.Setenv("COOKER_INSTALL_DIR", t.TempDir())
	output := captureStdout(t, func() {
		if err := cmdDoctor(); err != nil {
			t.Fatalf("cmdDoctor error: %v", err)
		}
	})
	if !strings.Contains(output, "cookeR Doctor") {
		t.Errorf("expected doctor output to mention 'cookeR Doctor', got: %q", output)
	}
}
