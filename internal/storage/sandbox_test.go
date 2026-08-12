package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestOpenAllowedFileReadsRegularFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "file.txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := OpenAllowedFile(path, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
}

func TestOpenAllowedFileRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outside)
	if _, err := OpenAllowedFile(outside, []string{root}); err == nil {
		t.Fatal("expected outside-root path rejection")
	}
}

func TestOpenAllowedFileRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation permissions vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAllowedFile(link, []string{root}); err == nil {
		t.Fatal("expected os.Root to reject symlink escape")
	}
}

func TestAtomicWriteAllowedFileNested(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "a", "b", "result.txt")
	if err := AtomicWriteAllowedFile(target, []string{root}, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestAtomicWriteAllowedFileOverwritesExisting(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "result.txt")
	if err := AtomicWriteAllowedFile(target, []string{root}, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWriteAllowedFile(target, []string{root}, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "second" {
		t.Fatalf("overwrite got %q", got)
	}
}

func TestAtomicWriteAllowedFileRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation permissions vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(link, "pwned.txt")
	if err := AtomicWriteAllowedFile(target, []string{root}, []byte("blocked"), 0o600); err == nil {
		t.Fatal("expected os.Root to reject write through escaping symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "pwned.txt")); !os.IsNotExist(err) {
		t.Fatalf("outside file unexpectedly created: %v", err)
	}
}
