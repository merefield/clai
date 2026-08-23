//go:build windows

package mcptools

import "os"

// Windows does not expose ACL ownership through os.FileMode permission bits.
// Keep the regular-file checks and let CreateProcess enforce executability.
func insecureWritePermissions(os.FileInfo) bool {
	return false
}

func executableRegularFile(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}

func privateRegularFile(info os.FileInfo) bool {
	return info.Mode().IsRegular()
}
