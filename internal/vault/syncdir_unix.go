//go:build !windows

package vault

import "os"

// syncDir flushes the directory entry so that the rename in writeTo survives
// a power loss, not just the file contents. Without it the kernel may have
// the new file on the platter but not the directory entry pointing at it.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
