//go:build unix

package mcptools

import "os"

func insecureWritePermissions(info os.FileInfo) bool {
	return info.Mode().Perm()&0o022 != 0
}

func executableRegularFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func privateRegularFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0o077 == 0
}
