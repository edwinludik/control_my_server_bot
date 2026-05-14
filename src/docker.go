package main

import (
	"fmt"
	"os/exec"
	"strings"
)

type DockerContainer struct {
	ID     string
	Image  string
	Names  string
	Status string
	State  string // running, exited, etc.
}

func getDockerContainers() ([]DockerContainer, error) {
	// Format: ID|Image|Names|Status|State
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.ID}}|{{.Image}}|{{.Names}}|{{.Status}}|{{.State}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var containers []DockerContainer
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 5 {
			continue
		}
		containers = append(containers, DockerContainer{
			ID:     parts[0],
			Image:  parts[1],
			Names:  parts[2],
			Status: parts[3],
			State:  parts[4],
		})
	}
	return containers, nil
}

func getDockerContainer(nameOrID string) (*DockerContainer, error) {
	cmd := exec.Command("docker", "inspect", nameOrID, "--format", "{{.ID}}|{{.Config.Image}}|{{.Name}}|{{.Status}}|{{.State.Status}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	if len(parts) != 5 {
		return nil, fmt.Errorf("invalid output from docker inspect")
	}

	// Remove leading slash from name
	name := strings.TrimPrefix(parts[2], "/")

	return &DockerContainer{
		ID:     parts[0][:12], // Short ID
		Image:  parts[1],
		Names:  name,
		Status: parts[3],
		State:  parts[4],
	}, nil
}

func getDockerLogs(containerID string, lines int) (string, error) {
	cmd := exec.Command("docker", "logs", "--tail", fmt.Sprintf("%d", lines), containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func dockerPull(image string) (string, error) {
	cmd := exec.Command("docker", "pull", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}
