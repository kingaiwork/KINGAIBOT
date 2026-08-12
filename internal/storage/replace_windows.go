//go:build windows

package storage

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	moveFileReplaceExisting = 0x1
	moveFileWriteThrough    = 0x8
)

var (
	kernel32    = syscall.NewLazyDLL("kernel32.dll")
	moveFileExW = kernel32.NewProc("MoveFileExW")
)

func replaceFile(src, dst string) error {
	srcp, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return err
	}
	dstp, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return err
	}
	r1, _, e1 := moveFileExW.Call(uintptr(unsafe.Pointer(srcp)), uintptr(unsafe.Pointer(dstp)), uintptr(moveFileReplaceExisting|moveFileWriteThrough))
	if r1 == 0 {
		if e1 != syscall.Errno(0) {
			return e1
		}
		return os.ErrInvalid
	}
	return nil
}

func syncDir(string) error { return nil }
