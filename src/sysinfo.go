package main

import (
	"os/exec"
	"strings"
)

func getCPUUsageInfo() (string, error) {
	// Using top -bn1 | grep "Cpu(s)" for Linux
	out, err := exec.Command("sh", "-c", "top -bn1 | grep \"Cpu(s)\"").Output()
	if err != nil {
		// Fallback for macOS: top -l 1 -n 0 | grep "CPU usage"
		out, err = exec.Command("sh", "-c", "top -l 1 -n 0 | grep \"CPU usage\"").Output()
		if err != nil {
			return "CPU usage info not available", nil
		}
	}
	return strings.TrimSpace(string(out)), nil
}

func getRAMUsageInfo() (string, error) {
	// Using free -h to get human-readable memory usage
	out, err := exec.Command("free", "-h").Output()
	if err != nil {
		// Fallback for macOS if free -h is not available
		out, err = exec.Command("top", "-l", "1", "-s", "0", "-n", "0").Output()
		if err != nil {
			return "RAM usage info not available", nil
		}
		// Basic extraction for macOS top output
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PhysMem:") {
				return line, nil
			}
		}
		return "RAM usage info not available", nil
	}
	return strings.TrimSpace(string(out)), nil
}

func getDiskSpaceInfo() (string, error) {
	// Using df -h to get human-readable disk space info on all mounted filesystems
	out, err := exec.Command("df", "-h").Output()
	if err != nil {
		return "Disk space info not available", nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return "No disk space information available.", nil
	}

	// Filter and format the output to be cleaner
	var result []string
	result = append(result, lines[0]) // Header

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		// Skip temporary or virtual filesystems if they are too many
		if strings.HasPrefix(line, "tmpfs") || strings.HasPrefix(line, "devtmpfs") || strings.HasPrefix(line, "udev") {
			continue
		}
		result = append(result, line)
	}

	return strings.Join(result, "\n"), nil
}
