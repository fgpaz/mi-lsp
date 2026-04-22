//go:build !windows

package daemon

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fgpaz/mi-lsp/internal/model"
)

func processMemoryBytes(pid int) uint64 {
	return processStats(pid).WorkingSetBytes
}

func processStats(pid int) model.DaemonProcessStats {
	stats := model.DaemonProcessStats{PID: pid}
	if pid == 0 {
		return stats
	}
	if stat, ok := linuxProcessStats(pid); ok {
		return stat
	}
	command := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	output, err := command.Output()
	if err != nil {
		return stats
	}
	value, _ := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
	stats.WorkingSetBytes = value * 1024
	return stats
}

func linuxProcessStats(pid int) (model.DaemonProcessStats, bool) {
	stats := model.DaemonProcessStats{PID: pid}
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")
	file, err := os.Open(statusPath)
	if err != nil {
		return stats, false
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "VmRSS:"):
			stats.WorkingSetBytes = parseStatusKB(line) * 1024
		case strings.HasPrefix(line, "VmData:"):
			stats.PrivateBytes = parseStatusKB(line) * 1024
		case strings.HasPrefix(line, "Threads:"):
			stats.ThreadCount = parseStatusUint(line)
		}
	}
	fdEntries, err := os.ReadDir(filepath.Join("/proc", strconv.Itoa(pid), "fd"))
	if err == nil {
		stats.HandleCount = uint64(len(fdEntries))
	}
	return stats, scanner.Err() == nil
}

func parseStatusKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	value, _ := strconv.ParseUint(fields[1], 10, 64)
	return value
}

func parseStatusUint(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	value, _ := strconv.ParseUint(fields[1], 10, 64)
	return value
}
