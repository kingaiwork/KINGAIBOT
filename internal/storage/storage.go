package storage

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
)

var idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

func ValidateID(id string) error {
	if !idPattern.MatchString(id) || id == "." || id == ".." {
		return errors.New("invalid identifier")
	}
	return nil
}

func AtomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".kingagent-tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(mode); err != nil { _ = f.Close(); return err }
	if _, err = f.Write(data); err != nil { _ = f.Close(); return err }
	if err = f.Sync(); err != nil { _ = f.Close(); return err }
	if err = f.Close(); err != nil { return err }
	if err = replaceFile(tmp, path); err != nil { return err }
	return syncDir(dir)
}

func RandomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil { return "", errors.New("secure random generator unavailable") }
	return prefix + "_" + hex.EncodeToString(b), nil
}
