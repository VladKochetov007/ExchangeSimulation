//go:build !linux || !amd64

package multivenue

import "errors"

func renameRenderDirectoryNoReplace(sourcePath, destinationPath string) error {
	return errors.New("multivenue: no-replacement directory publication is unsupported on this platform")
}
