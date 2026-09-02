//go:build linux && amd64

package multivenue

import (
	"syscall"
	"unsafe"
)

const (
	renderRenameAt2Syscall = uintptr(316)
	renderRenameNoReplace  = uintptr(1)
)

func renameRenderDirectoryNoReplace(sourcePath, destinationPath string) error {
	source, err := syscall.BytePtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := syscall.BytePtrFromString(destinationPath)
	if err != nil {
		return err
	}
	// AT_FDCWD is -100; this representation preserves the signed value in a uintptr.
	currentDirectory := ^uintptr(99)
	_, _, errno := syscall.Syscall6(
		renderRenameAt2Syscall,
		currentDirectory,
		uintptr(unsafe.Pointer(source)),
		currentDirectory,
		uintptr(unsafe.Pointer(destination)),
		renderRenameNoReplace,
		0,
	)
	if errno != 0 {
		return errno
	}
	return nil
}
