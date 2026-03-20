package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/config"
	"github.com/ghostwriter/ghostwriter/internal/container"
	"github.com/spf13/cobra"
)

func newQdrantCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "qdrant",
		Short: "Manage the Qdrant vector database",
		Long:  `Start, stop, and check the status of the Qdrant container.`,
	}

	cmd.AddCommand(
		newQdrantStartCmd(),
		newQdrantStopCmd(),
		newQdrantStatusCmd(),
	)

	return cmd
}

func newQdrantStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the Qdrant container",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			// Check if already running.
			if container.IsQdrantHealthy(cfg.Qdrant.URL) {
				log.Info("qdrant is already running", "url", cfg.Qdrant.URL)
				return nil
			}

			rt, err := container.Detect()
			if err != nil {
				return err
			}

			log.Info("starting qdrant", "runtime", rt.RuntimeName())

			repoRoot, err := findRepoRoot()
			if err != nil {
				return err
			}

			composeFile := filepath.Join(repoRoot, "rag", "compose.yaml")
			if err := rt.StartQdrant(cmd.Context(), composeFile); err != nil {
				return fmt.Errorf("failed to start qdrant: %w", err)
			}

			log.Info("waiting for qdrant to become healthy")
			if err := container.WaitForHealthy(cmd.Context(), cfg.Qdrant.URL, 30*time.Second); err != nil {
				return err
			}

			log.Info("qdrant is running", "url", cfg.Qdrant.URL)
			return nil
		},
	}
}

func newQdrantStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the Qdrant container",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := container.Detect()
			if err != nil {
				return err
			}

			repoRoot, err := findRepoRoot()
			if err != nil {
				return err
			}

			composeFile := filepath.Join(repoRoot, "rag", "compose.yaml")
			if err := rt.StopQdrant(cmd.Context(), composeFile); err != nil {
				return fmt.Errorf("failed to stop qdrant: %w", err)
			}

			log.Info("qdrant stopped")
			return nil
		},
	}
}

func newQdrantStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check Qdrant health and collection status",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			if container.IsQdrantHealthy(cfg.Qdrant.URL) {
				log.Info("qdrant is healthy", "url", cfg.Qdrant.URL)
			} else {
				log.Warn("qdrant is not responding", "url", cfg.Qdrant.URL)
				return nil
			}

			return nil
		},
	}
}
