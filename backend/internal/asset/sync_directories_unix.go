//go:build !windows

package asset

import (
	"os"
	"path/filepath"
)

// Flush the rename and newly created object directories before committing metadata.
func syncResourceDirectories(root, dir string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	for {
		file, err := os.Open(dir)
		if err != nil {
			return err
		}
		syncErr := file.Sync()
		closeErr := file.Close()
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if dir == absRoot {
			return nil
		}
		dir = filepath.Dir(dir)
	}
}
