//go:build !windows

package shellintegration

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func parentExecutable(ctx context.Context) string {
	if path, err := os.Readlink("/proc/" + strconv.Itoa(os.Getppid()) + "/exe"); err == nil {
		return path
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(os.Getppid()), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
