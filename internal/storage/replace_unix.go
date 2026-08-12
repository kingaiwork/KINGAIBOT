//go:build !windows

package storage

import "os"

func replaceFile(src, dst string) error { return os.Rename(src, dst) }

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
