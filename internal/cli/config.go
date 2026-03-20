package cli

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage ghostwriter configuration",
		Long:  `View and update ghostwriter configuration values.`,
	}

	cmd.AddCommand(
		newConfigShowCmd(),
		newConfigSetCmd(),
		newConfigPathCmd(),
	)

	return cmd
}

func newConfigShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}

			fmt.Println("github:")
			fmt.Printf("  username:   %s\n", cfg.GitHub.Username)
			fmt.Printf("  orgs:       %s\n", strings.Join(cfg.GitHub.Orgs, ", "))
			fmt.Printf("  batch_size: %d\n", cfg.GitHub.BatchSize)
			fmt.Printf("  max_pages:  %d\n", cfg.GitHub.MaxPages)
			if cfg.GitHub.Token != "" {
				fmt.Println("  token:      [set]")
			} else {
				fmt.Println("  token:      [not set, will use gh CLI]")
			}

			fmt.Println("qdrant:")
			fmt.Printf("  url:             %s\n", cfg.Qdrant.URL)
			fmt.Printf("  collection_name: %s\n", cfg.Qdrant.CollectionName)
			fmt.Printf("  embedding_model: %s\n", cfg.Qdrant.EmbeddingModel)

			fmt.Println("corpus:")
			fmt.Printf("  dir: %s\n", cfg.Corpus.Dir)

			fmt.Println("style:")
			fmt.Printf("  normalize_dashes: %t\n", cfg.Style.NormalizeDashes)

			configFile := viper.ConfigFileUsed()
			if configFile != "" {
				fmt.Printf("\nconfig file: %s\n", configFile)
			} else {
				fmt.Println("\nconfig file: [none found, using defaults]")
			}

			return nil
		},
	}
}

func newConfigSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value and save it to disk.

Examples:
  gw config set github.username myuser
  gw config set github.orgs "org1,org2"
  gw config set qdrant.url http://localhost:6333
  gw config set style.normalize_dashes false`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			value := args[1]

			cfg, err := config.Load()
			if err != nil {
				return err
			}

			switch key {
			case "github.username":
				cfg.GitHub.Username = value
			case "github.orgs":
				cfg.GitHub.Orgs = strings.Split(value, ",")
				for i, org := range cfg.GitHub.Orgs {
					cfg.GitHub.Orgs[i] = strings.TrimSpace(org)
				}
			case "github.token":
				cfg.GitHub.Token = value
			case "qdrant.url":
				cfg.Qdrant.URL = value
			case "qdrant.collection_name":
				cfg.Qdrant.CollectionName = value
			case "qdrant.embedding_model":
				cfg.Qdrant.EmbeddingModel = value
			case "corpus.dir":
				cfg.Corpus.Dir = value
			case "style.normalize_dashes":
				cfg.Style.NormalizeDashes = value == "true"
			default:
				return fmt.Errorf("unknown config key: %s", key)
			}

			if err := config.Save(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			log.Info("config updated", "key", key, "value", value)
			return nil
		},
	}
}

func newConfigPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.ConfigPath()
			if err != nil {
				return err
			}
			fmt.Println(path)
			return nil
		},
	}
}
