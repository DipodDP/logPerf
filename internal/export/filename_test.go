package export

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testDate = time.Date(2026, 2, 18, 14, 32, 7, 0, time.UTC)

func TestDateSuffix(t *testing.T) {
	got := DateSuffix(testDate)
	want := "18.02.2026"
	if got != want {
		t.Errorf("DateSuffix() = %q, want %q", got, want)
	}
}

func TestBuildPath_Simple(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "measurement")
	got := BuildPath(base, "", ".csv", testDate)
	want := filepath.Join(dir, "measurement_18.02.2026.csv")
	if got != want {
		t.Errorf("BuildPath() = %q, want %q", got, want)
	}
}

// BuildPath returns the same path even if the file already exists (append mode).
func TestBuildPath_ExistingFileReturnssamePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "results")

	first := filepath.Join(dir, "results_18.02.2026.csv")
	if err := os.WriteFile(first, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}

	got := BuildPath(base, "", ".csv", testDate)
	if got != first {
		t.Errorf("BuildPath() with existing file = %q, want %q (same path for append)", got, first)
	}
}

func TestBuildPath_WithSuffix(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "measurement")
	got := BuildPath(base, "_log", ".csv", testDate)
	want := filepath.Join(dir, "measurement_log_18.02.2026.csv")
	if got != want {
		t.Errorf("BuildPath(_log) = %q, want %q", got, want)
	}
}

func TestBuildPath_TXT(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "results")
	got := BuildPath(base, "", ".txt", testDate)
	want := filepath.Join(dir, "results_18.02.2026.txt")
	if got != want {
		t.Errorf("BuildPath(.txt) = %q, want %q", got, want)
	}
}

func TestBuildLogPath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "results")
	got := BuildLogPath(base, "_log", ".csv")
	want := filepath.Join(dir, "results_log.csv")
	if got != want {
		t.Errorf("BuildLogPath() = %q, want %q", got, want)
	}
}

func TestBuildLogPath_NoSuffix(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "results")
	got := BuildLogPath(base, "", ".csv")
	want := filepath.Join(dir, "results.csv")
	if got != want {
		t.Errorf("BuildLogPath(no suffix) = %q, want %q", got, want)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "file.csv")
	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir() error: %v", err)
	}
	info, err := os.Stat(filepath.Join(dir, "sub", "dir"))
	if err != nil {
		t.Fatalf("directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected a directory")
	}
}

func TestEnsureDir_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.csv")
	// dir already exists — EnsureDir should be a no-op
	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir() on existing dir error: %v", err)
	}
}
