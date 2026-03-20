package cli

import (
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/collector"
	"github.com/ghostwriter/ghostwriter/internal/config"
	"github.com/spf13/cobra"
)

func newCollectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect",
		Short: "Collect writing samples from external sources",
		Long:  `Collect your writing samples from supported sources like GitHub PR reviews.`,
	}

	cmd.AddCommand(newCollectGitHubCmd())

	return cmd
}

func newCollectGitHubCmd() *cobra.Command {
	var (
		username string
		orgs     []string
	)

	cmd := &cobra.Command{
		Use:   "github",
		Short: "Collect PR reviews from GitHub",
		Long: `Collect your PR review comments from GitHub using the GraphQL API.

Searches for PRs you've reviewed across your configured organizations and
extracts both review summaries and inline code comments.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// CLI flags override config values.
			if username == "" {
				username = cfg.GitHub.Username
			}
			if len(orgs) == 0 {
				orgs = cfg.GitHub.Orgs
			}

			if username == "" {
				return fmt.Errorf("GitHub username is required: use --username flag or run 'gw init'")
			}
			if len(orgs) == 0 {
				return fmt.Errorf("at least one GitHub org is required: use --orgs flag or run 'gw init'")
			}

			log.Info("collecting PR reviews from GitHub",
				"username", username,
				"orgs", orgs,
			)

			gc, err := collector.NewGitHubCollector(collector.GitHubCollectorOpts{
				Token:     cfg.GitHub.Token,
				Username:  username,
				Orgs:      orgs,
				BatchSize: cfg.GitHub.BatchSize,
				MaxPages:  cfg.GitHub.MaxPages,
				OutputDir: cfg.Corpus.Dir,
			})
			if err != nil {
				return err
			}

			result, err := gc.Collect(cmd.Context(), func(org string, page int, found int) {
				log.Info("collecting", "org", org, "page", page, "records", found)
			})
			if err != nil {
				return err
			}

			log.Info("collection complete",
				"total_records", result.TotalRecords,
				"unique_records", result.UniqueRecords,
				"output_file", result.OutputFile,
			)

			return nil
		},
	}

	cmd.Flags().StringVar(&username, "username", "", "GitHub username (overrides config)")
	cmd.Flags().StringSliceVar(&orgs, "orgs", nil, "GitHub orgs to search (overrides config)")

	return cmd
}
