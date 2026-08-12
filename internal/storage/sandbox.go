package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxSandboxPathBytes = 8192
const maxSandboxPathComponents = 128

// OpenAllowedFile opens an untrusted path only when it is lexically beneath one
// of the configured roots. The actual filesystem walk is then performed by
// os.Root, which prevents symlink and .. traversal from escaping that root.
func OpenAllowedFile(path string, roots []string) (*os.File, error) {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	f, openErr := root.Open(rel)
	closeErr := root.Close()
	if openErr != nil {
		return nil, openErr
	}
	if closeErr != nil {
		_ = f.Close()
		return nil, closeErr
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, errors.New("only regular files may be read")
	}
	return f, nil
}

// AtomicWriteAllowedFile writes an untrusted path beneath an allowed root.
// The temporary file and final rename are both performed through the same
// os.Root handle so a concurrent symlink swap cannot redirect the operation
// outside the configured root.
func AtomicWriteAllowedFile(path string, roots []string, data []byte, mode os.FileMode) error {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return err
	}
	if mode&^os.FileMode(0o777) != 0 {
		return errors.New("unsupported file mode")
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()

	parent := filepath.Dir(rel)
	if parent != "." {
		if err := root.MkdirAll(parent, 0o700); err != nil {
			return err
		}
	}

	tmpID, err := RandomID("tmp")
	if err != nil {
		return err
	}
	tmpName := ".kingaibot-" + tmpID
	tmpRel := tmpName
	if parent != "." {
		tmpRel = filepath.Join(parent, tmpName)
	}
	f, err := root.OpenFile(tmpRel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer root.Remove(tmpRel)

	if err := writeAll(f, data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := root.Rename(tmpRel, rel); err != nil {
		return err
	}
	return nil
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func matchAllowedRoot(path string, roots []string) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", errors.New("path required")
	}
	if len(path) > maxSandboxPathBytes {
		return "", "", errors.New("path too long")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", err
	}
	absPath = filepath.Clean(absPath)
	for _, configuredRoot := range roots {
		if strings.TrimSpace(configuredRoot) == "" {
			continue
		}
		rootPath, err := filepath.Abs(configuredRoot)
		if err != nil {
			continue
		}
		rootPath = filepath.Clean(rootPath)
		rel, err := filepath.Rel(rootPath, absPath)
		if err != nil || rel == "." || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if len(rel) > maxSandboxPathBytes || pathComponents(rel) > maxSandboxPathComponents {
			return "", "", errors.New("path complexity limit exceeded")
		}
		return rootPath, rel, nil
	}
	return "", "", fmt.Errorf("path outside allowed roots")
}

func pathComponents(p string) int {
	p = filepath.Clean(p)
	if p == "." || p == "" {
		return 0
	}
	return len(strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }))
}
