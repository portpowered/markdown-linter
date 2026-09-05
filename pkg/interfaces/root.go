package interfaces

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckPathRoot rejects paths outside root, including symlink escapes. Missing
// targets are checked through their nearest existing ancestor. An empty root
// leaves filesystem policy to the direct library caller.
func CheckPathRoot(root, target string) error {
	if root == "" {
		return nil
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	for {
		resolved, resolveErr := filepath.EvalSymlinks(target)
		if resolveErr == nil {
			relative, err := filepath.Rel(root, resolved)
			if err != nil {
				return err
			}
			if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("path escapes document root")
			}
			return nil
		}
		if !os.IsNotExist(resolveErr) {
			return resolveErr
		}
		parent := filepath.Dir(target)
		if parent == target {
			return resolveErr
		}
		target = parent
	}
}
