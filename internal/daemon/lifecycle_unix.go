//go:build !windows

package daemon

import (
	"os/exec"
	"strconv"
	"strings"
)

func processMemoryBytes(pid int) uint64 {
	if pid == 0 {
		return 0
	}
	command := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	output, err := command.Output()
	if err != nil {
		return 0
	}
	value, _ := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	return value * 1024
}
