package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
)

func validateLockPath(path string) error {
	for _, item := range []string{filepath.Dir(path), path} {
		info, err := os.Lstat(item)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("lock path contains symbolic link: %s", item)
		}
	}
	return nil
}
