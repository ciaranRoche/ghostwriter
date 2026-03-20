package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/collector"
	"github.com/ghostwriter/ghostwriter/internal/config"
	"github.com/ghostwriter/ghostwriter/internal/container"
	"github.com/ghostwriter/ghostwriter/internal/embedder"
	"github.com/ghostwriter/ghostwriter/internal/installer"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Interactive setup wizard",
		Long: `Run the interactive setup wizard to configure ghostwriter.

This walks you through:
  1. Detecting prerequisites (container runtime)
  2. Configuring your GitHub username and orgs
  3. Collecting your PR review writing samples
  4. Starting Qdrant and ingesting samples
  5. Installing style files for your AI tools`,
		RunE: runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	// Welcome.
	fmt.Println()
	fmt.Println("  Ghostwriter Setup")
	fmt.Println("  Teach AI to write in your voice.")
	fmt.Println()

	// Step 1: Detect prerequisites.
	log.Info("checking prerequisites")

	rt, err := container.Detect()
	if err != nil {
		return fmt.Errorf("prerequisite check failed: %w", err)
	}
	log.Info("container runtime detected", "runtime", rt.RuntimeName())

	if rt.HasCompose() {
		log.Info("compose available", "command", strings.Join(rt.ComposeCommand, " "))
	} else {
		log.Warn("compose not found, will use direct container run")
	}

	// Step 2: Configure GitHub settings.
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	username := cfg.GitHub.Username
	orgsStr := strings.Join(cfg.GitHub.Orgs, ", ")

	// Try auto-detection if not already configured.
	if username == "" {
		if detected, err := collector.DetectUsername(); err == nil {
			username = detected
		}
	}
	if orgsStr == "" {
		if detected, err := collector.DetectOrgs(); err == nil {
			orgsStr = strings.Join(detected, ", ")
		}
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GitHub username").
				Description("Your GitHub username for collecting PR reviews.").
				Value(&username).
				Placeholder("your-username"),
			huh.NewInput().
				Title("GitHub organizations").
				Description("Comma-separated list of orgs to search for your reviews.").
				Value(&orgsStr).
				Placeholder("org1, org2"),
		),
	)

	if err := form.Run(); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	// Parse orgs from comma-separated string.
	var orgs []string
	for _, org := range strings.Split(orgsStr, ",") {
		org = strings.TrimSpace(org)
		if org != "" {
			orgs = append(orgs, org)
		}
	}

	cfg.GitHub.Username = username
	cfg.GitHub.Orgs = orgs

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}
	log.Info("configuration saved")

	// Step 3: Collect writing samples.
	var doCollect bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Collect PR reviews from GitHub?").
				Description(fmt.Sprintf("This will search for reviews by %s across %s.", username, strings.Join(orgs, ", "))).
				Value(&doCollect).
				Affirmative("Yes").
				Negative("Skip"),
		),
	)

	if err := confirmForm.Run(); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	if doCollect {
		log.Info("collecting PR reviews from GitHub")

		gc, err := collector.NewGitHubCollector(collector.GitHubCollectorOpts{
			Token:     cfg.GitHub.Token,
			Username:  username,
			Orgs:      orgs,
			BatchSize: cfg.GitHub.BatchSize,
			MaxPages:  cfg.GitHub.MaxPages,
			OutputDir: cfg.Corpus.Dir,
		})
		if err != nil {
			return fmt.Errorf("failed to create collector: %w", err)
		}

		result, err := gc.Collect(cmd.Context(), func(org string, page int, found int) {
			log.Info("collecting", "org", org, "page", page, "records", found)
		})
		if err != nil {
			return fmt.Errorf("collection failed: %w", err)
		}

		log.Info("collection complete",
			"total_records", result.TotalRecords,
			"unique_records", result.UniqueRecords,
		)
	} else {
		log.Info("skipping collection, run 'gw collect github' later")
	}

	// Step 4: Start Qdrant.
	if !container.IsQdrantHealthy(cfg.Qdrant.URL) {
		log.Info("starting qdrant")

		repoRoot, err := findRepoRoot()
		if err != nil {
			return fmt.Errorf("unable to find repo root: %w", err)
		}

		composeFile := filepath.Join(repoRoot, "rag", "compose.yaml")
		if err := rt.StartQdrant(cmd.Context(), composeFile); err != nil {
			return fmt.Errorf("failed to start qdrant: %w", err)
		}

		log.Info("waiting for qdrant to become healthy")
		if err := container.WaitForHealthy(cmd.Context(), cfg.Qdrant.URL, 30*time.Second); err != nil {
			return fmt.Errorf("qdrant failed to start: %w", err)
		}
	}

	log.Info("qdrant is running", "url", cfg.Qdrant.URL)

	// Step 5: Ingest samples.
	if doCollect {
		corpusFile := filepath.Join(cfg.Corpus.Dir, "reviews.jsonl")
		records, err := loadCorpus(corpusFile)
		if err != nil {
			log.Warn("skipping ingestion", "reason", err.Error())
		} else {
			log.Info("ingesting samples into qdrant", "records", len(records))

			e := embedder.NewEmbedder(embedder.EmbedderOpts{
				QdrantURL:       cfg.Qdrant.URL,
				CollectionName:  cfg.Qdrant.CollectionName,
				EmbeddingModel:  cfg.Qdrant.EmbeddingModel,
				NormalizeDashes: cfg.Style.NormalizeDashes,
			})

			ingestResult, err := e.Ingest(cmd.Context(), records, true, func(batch, total int) {
				log.Info("upserting", "progress", fmt.Sprintf("%d/%d", batch, total))
			})
			if err != nil {
				return fmt.Errorf("ingestion failed: %w", err)
			}

			log.Info("ingestion complete",
				"ingested", ingestResult.IngestedCount,
			)
		}
	}

	// Step 6: Install for AI tools.
	tools := installer.SupportedTools()
	toolNames := make([]string, len(tools))
	for i, t := range tools {
		toolNames[i] = t.Name
	}

	var selectedTools []string
	toolForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Install for which AI tools?").
				Description("Select the tools you want to configure with ghostwriter.").
				Options(
					huh.NewOption("OpenCode", "opencode"),
					huh.NewOption("Claude Code", "claude"),
					huh.NewOption("Cursor", "cursor"),
					huh.NewOption("Gemini CLI", "gemini"),
					huh.NewOption("Windsurf", "windsurf"),
					huh.NewOption("Cline", "cline"),
				).
				Value(&selectedTools),
		),
	)

	if err := toolForm.Run(); err != nil {
		return fmt.Errorf("form cancelled: %w", err)
	}

	if len(selectedTools) > 0 {
		repoRoot, err := findRepoRoot()
		if err != nil {
			return fmt.Errorf("unable to find repo root: %w", err)
		}

		for _, toolName := range selectedTools {
			t, err := installer.ToolByName(toolName)
			if err != nil {
				log.Warn("unknown tool, skipping", "tool", toolName)
				continue
			}

			result, err := t.InstallFunc(repoRoot)
			if err != nil {
				log.Warn("failed to install", "tool", toolName, "error", err)
				continue
			}

			printInstallResult(toolName, result)
		}
	}

	// Done.
	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Println()
	fmt.Println("  Next steps:")
	fmt.Println("  1. Customize your style in AGENTS.md and skills/ghostwriter/SKILL.md")
	fmt.Println("  2. Add 10-15 writing examples to style/examples/")
	fmt.Println("  3. Restart your AI tool and try: \"Review this PR in my style\"")
	fmt.Println()

	return nil
}
