//go:build windows

package vault

// syncDir does nothing on Windows, which has no equivalent of flushing a
// directory: FlushFileBuffers rejects a directory handle with "Access is
// denied", so the POSIX idiom of opening the directory and syncing it fails
// outright rather than degrading.
//
// The durability the sync provides elsewhere comes from the rename itself
// here. os.Rename calls MoveFileEx with MOVEFILE_REPLACE_EXISTING, which
// NTFS journals as a single metadata transaction, so a crash leaves either
// the old file or the new one.
func syncDir(string) error { return nil }
