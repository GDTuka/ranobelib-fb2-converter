//go:build !windows

package converter

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
