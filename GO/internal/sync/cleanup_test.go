package sync

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestDeprecatedFileDeletion(t *testing.T) {
	dir := t.TempDir()
	// Create .tmp subdir so Cleanup doesn't fail
	os.MkdirAll(filepath.Join(dir, ".tmp"), 0750)

	deprecated := []string{
		"Lync Installer.pkg",
		"MicrosoftTeams.pkg",
		"Teams_osx.pkg",
		"wdav-upgrade.pkg",
	}
	for _, name := range deprecated {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	count := Cleanup(dir, discardLogger)

	for _, name := range deprecated {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("deprecated file %q should have been deleted", name)
		}
	}
	if count < len(deprecated) {
		t.Errorf("count = %d, want at least %d", count, len(deprecated))
	}
}

func TestXmlCatFilesInRootDeleted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tmp"), 0750)

	files := []string{"app.xml", "app.cat", "OTHER.XML", "test.CAT"}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, discardLogger)

	for _, name := range files {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("root xml/cat file %q should have been deleted", name)
		}
	}
}

func TestCollateralSubdirNotDeleted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tmp"), 0750)

	collateralDir := filepath.Join(dir, "collateral")
	os.MkdirAll(collateralDir, 0750)
	// Place xml and cat files in collateral subdirectory
	for _, name := range []string{"app.xml", "app.cat"} {
		if err := os.WriteFile(filepath.Join(collateralDir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, discardLogger)

	// Files in collateral/ must survive (P4 fix validation)
	for _, name := range []string{"app.xml", "app.cat"} {
		if _, err := os.Stat(filepath.Join(collateralDir, name)); err != nil {
			t.Errorf("collateral file %q should NOT have been deleted (P4 fix)", name)
		}
	}
}

func TestBuildsTxtDeleted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tmp"), 0750)

	if err := os.WriteFile(filepath.Join(dir, "builds.txt"), []byte("16.90\n16.91"), 0644); err != nil {
		t.Fatal(err)
	}

	Cleanup(dir, discardLogger)

	if _, err := os.Stat(filepath.Join(dir, "builds.txt")); err == nil {
		t.Error("builds.txt should have been deleted")
	}
}

func TestNonMatchingFilesNotDeleted(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".tmp"), 0750)

	keep := []string{"update.pkg", "readme.txt", "image.png"}
	for _, name := range keep {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	Cleanup(dir, discardLogger)

	for _, name := range keep {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("non-matching file %q should NOT have been deleted", name)
		}
	}
}

func TestCleanupEmptyDir(t *testing.T) {
	dir := t.TempDir()
	// No panic or error expected on empty directory
	count := Cleanup(dir, discardLogger)
	if count != 0 {
		t.Errorf("count = %d, want 0 for empty dir", count)
	}
}
