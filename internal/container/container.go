// Package container provides an abstraction for managing container runtimes
// (podman/docker) and compose tooling for running Qdrant.
package container

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// Runtime represents a detected container runtime.
type Runtime struct {
	// Command is the container runtime command (e.g., "podman" or "docker").
	Command string

	// ComposeCommand is the compose command parts (e.g., ["podman", "compose"] or ["docker", "compose"]).
	// Empty if compose is not available.
	ComposeCommand []string
}

// Detect finds an available container runtime on the system.
// It prefers podman over docker.
func Detect() (*Runtime, error) {
	rt := &Runtime{}

	// Check for podman first, then docker.
	if path, err := exec.LookPath("podman"); err == nil {
		rt.Command = path
		rt.ComposeCommand = detectCompose("podman")
	} else if path, err := exec.LookPath("docker"); err == nil {
		rt.Command = path
		rt.ComposeCommand = detectCompose("docker")
	} else {
		return nil, fmt.Errorf("no container runtime found: install podman or docker")
	}

	return rt, nil
}

// detectCompose finds the compose command for a given runtime.
func detectCompose(runtime string) []string {
	// Check for standalone compose command (e.g., podman-compose, docker-compose).
	standaloneName := runtime + "-compose"
	if _, err := exec.LookPath(standaloneName); err == nil {
		return []string{standaloneName}
	}

	// Check for subcommand compose (e.g., podman compose, docker compose).
	cmd := exec.Command(runtime, "compose", "version")
	if err := cmd.Run(); err == nil {
		return []string{runtime, "compose"}
	}

	return nil
}

// HasCompose returns true if a compose command is available.
func (r *Runtime) HasCompose() bool {
	return len(r.ComposeCommand) > 0
}

// RuntimeName returns a friendly name for the detected runtime.
func (r *Runtime) RuntimeName() string {
	if strings.Contains(r.Command, "podman") {
		return "podman"
	}
	return "docker"
}

// qdrantImage is the container image used for Qdrant.
const qdrantImage = "docker.io/qdrant/qdrant"

// qdrantContainerName is the preferred name for containers started by the CLI.
const qdrantContainerName = "ghostwriter-qdrant"

// findQdrantContainers discovers Qdrant containers by image name.
// When allStates is true, it includes stopped/exited containers.
// This handles containers started outside the CLI (e.g., with a different name).
func (r *Runtime) findQdrantContainers(ctx context.Context, allStates bool) []string {
	args := []string{"ps",
		"--filter", "ancestor=" + qdrantImage,
		"--format", "{{.Names}}"}
	if allStates {
		args = append(args, "-a")
	}

	cmd := exec.CommandContext(ctx, r.Command, args...)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

// removeStaleContainers removes any stopped Qdrant containers that would
// block a new container from starting with the same name.
func (r *Runtime) removeStaleContainers(ctx context.Context) {
	running := r.findQdrantContainers(ctx, false)
	all := r.findQdrantContainers(ctx, true)

	runningSet := make(map[string]bool, len(running))
	for _, name := range running {
		runningSet[name] = true
	}

	for _, name := range all {
		if !runningSet[name] {
			log.Debug("removing stale qdrant container", "name", name)
			cmd := exec.CommandContext(ctx, r.Command, "rm", "-f", name)
			if err := cmd.Run(); err != nil {
				log.Debug("failed to remove stale container", "name", name, "error", err)
			}
		}
	}
}

// StartQdrant starts the Qdrant container using compose or a direct run command.
func (r *Runtime) StartQdrant(ctx context.Context, composeFile string) error {
	if r.HasCompose() {
		args := append(r.ComposeCommand[1:], "-f", composeFile, "up", "-d")
		cmd := exec.CommandContext(ctx, r.ComposeCommand[0], args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		log.Debug("starting qdrant with compose", "command", cmd.String())
		return cmd.Run()
	}

	// Check if a Qdrant container is already running (possibly under a different name).
	if existing := r.findQdrantContainers(ctx, false); len(existing) > 0 {
		log.Info("qdrant container already running", "name", existing[0])
		return nil
	}

	// Remove any stopped containers that would block a new one with the same name.
	r.removeStaleContainers(ctx)

	log.Debug("starting qdrant with direct run", "runtime", r.RuntimeName())
	args := []string{
		"run", "-d",
		"--name", qdrantContainerName,
		"-p", "6333:6333",
		"-p", "6334:6334",
		"-v", "qdrant_storage:/qdrant/storage:z",
		qdrantImage + ":latest",
	}
	cmd := exec.CommandContext(ctx, r.Command, args...)
	return cmd.Run()
}

// StopQdrant stops the Qdrant container.
func (r *Runtime) StopQdrant(ctx context.Context, composeFile string) error {
	if r.HasCompose() {
		args := append(r.ComposeCommand[1:], "-f", composeFile, "down")
		cmd := exec.CommandContext(ctx, r.ComposeCommand[0], args...)
		return cmd.Run()
	}

	// Find running Qdrant containers by image, not by hardcoded name.
	containers := r.findQdrantContainers(ctx, false)
	if len(containers) == 0 {
		log.Warn("no running qdrant container found")
		return nil
	}

	for _, name := range containers {
		log.Info("stopping qdrant container", "name", name)
		cmd := exec.CommandContext(ctx, r.Command, "stop", name)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to stop container %s: %w", name, err)
		}
		cmd = exec.CommandContext(ctx, r.Command, "rm", "-f", name)
		if err := cmd.Run(); err != nil {
			log.Warn("failed to remove container", "name", name, "error", err)
		}
	}

	return nil
}

// WaitForHealthy polls the Qdrant health endpoint until it responds or the timeout is reached.
func WaitForHealthy(ctx context.Context, qdrantURL string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := strings.TrimRight(qdrantURL, "/") + "/healthz"
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("qdrant did not become healthy within %s", timeout)
}

// IsQdrantHealthy checks if Qdrant is currently responding.
func IsQdrantHealthy(qdrantURL string) bool {
	healthURL := strings.TrimRight(qdrantURL, "/") + "/healthz"
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(healthURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
