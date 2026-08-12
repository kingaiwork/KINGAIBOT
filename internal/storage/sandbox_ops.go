package storage

import (
	"errors"
	"os"
	"time"
)

const maxDirectoryEntries = 1024

type EntryInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
	IsDir   bool      `json:"is_dir"`
}

func StatAllowedPath(path string, roots []string) (EntryInfo, error) {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return EntryInfo{}, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return EntryInfo{}, err
	}
	defer root.Close()
	info, err := root.Stat(rel)
	if err != nil {
		return EntryInfo{}, err
	}
	return entryInfo(info), nil
}

func ListAllowedDir(path string, roots []string) ([]EntryInfo, error) {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(rel)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("path is not a directory")
	}
	entries, err := f.ReadDir(maxDirectoryEntries + 1)
	if err != nil {
		return nil, err
	}
	if len(entries) > maxDirectoryEntries {
		return nil, errors.New("directory exceeds 1024 entry limit")
	}
	out := make([]EntryInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		out = append(out, entryInfo(info))
	}
	return out, nil
}

func MkdirAllowed(path string, roots []string) error {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.MkdirAll(rel, 0o700)
}

// RemoveAllowed removes one file or one empty directory. Recursive deletion is
// intentionally not exposed as an agent tool in this release.
func RemoveAllowed(path string, roots []string) error {
	rootPath, rel, err := matchAllowedRoot(path, roots)
	if err != nil {
		return err
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Remove(rel)
}

func entryInfo(info os.FileInfo) EntryInfo {
	return EntryInfo{
		Name:    info.Name(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().UTC(),
		IsDir:   info.IsDir(),
	}
}
