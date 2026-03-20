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

	// Fall back to direct container run.
	log.Debug("starting qdrant with direct run", "runtime", r.RuntimeName())
	args := []string{
		"run", "-d",
		"--name", "ghostwriter-qdrant",
		"-p", "6333:6333",
		"-p", "6334:6334",
		"-v", "qdrant_storage:/qdrant/storage:z",
		"docker.io/qdrant/qdrant:latest",
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

	cmd := exec.CommandContext(ctx, r.Command, "stop", "ghostwriter-qdrant")
	if err := cmd.Run(); err != nil {
		return err
	}
	cmd = exec.CommandContext(ctx, r.Command, "rm", "-f", "ghostwriter-qdrant")
	return cmd.Run()
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
