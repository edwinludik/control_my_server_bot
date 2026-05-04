package main

import (
	"fmt"
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

func getSystemShutdownRequester() string {
	// 1. Check if a system shutdown or reboot is active via systemctl list-jobs
	// On systemd systems, a shutdown/reboot involves active jobs like reboot.target or poweroff.target.
	out, err := exec.Command("systemctl", "list-jobs").Output()
	if err != nil {
		return ""
	}

	output := string(out)
	isShuttingDown := strings.Contains(output, "reboot.target") ||
		strings.Contains(output, "poweroff.target") ||
		strings.Contains(output, "halt.target") ||
		strings.Contains(output, "shutdown.target")

	if !isShuttingDown {
		return ""
	}

	// 2. Try to find who requested the shutdown from journalctl
	// systemd-logind usually logs: "System is rebooting (requested by user root/0)."
	// We check the last 20 entries of systemd-logind.
	out, err = exec.Command("journalctl", "-u", "systemd-logind", "-n", "20", "--no-pager").Output()
	if err == nil {
		lines := strings.Split(string(out), "\n")
		// Iterate backwards to find the most recent request
		for i := len(lines) - 1; i >= 0; i-- {
			line := lines[i]
			if strings.Contains(line, "System is") && strings.Contains(line, "requested by user") {
				start := strings.Index(line, "requested by user")
				if start != -1 {
					return strings.TrimSuffix(line[start:], ".")
				}
			}
		}
	}

	// 3. Fallback: check who is currently logged in to provide context
	out, err = exec.Command("who").Output()
	if err == nil && len(out) > 0 {
		users := strings.Split(strings.TrimSpace(string(out)), "\n")
		var userList []string
		for _, u := range users {
			fields := strings.Fields(u)
			if len(fields) > 0 {
				userList = append(userList, fields[0])
			}
		}
		if len(userList) > 0 {
			return fmt.Sprintf("System shutdown in progress (Logged in users: %s)", strings.Join(userList, ", "))
		}
	}

	return "System shutdown in progress"
}
