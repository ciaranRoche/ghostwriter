package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/log"
	"github.com/ghostwriter/ghostwriter/internal/installer"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var tool string

	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install style files for AI coding tools",
		Long: `Install ghostwriter style instructions and MCP configuration for your AI tools.

Copies the appropriate files (AGENTS.md, SKILL.md, rules, MCP config) to the
correct locations for each tool.

Supported tools: opencode, claude, cursor, gemini, windsurf, cline, all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			repoRoot, err := findRepoRoot()
			if err != nil {
				return fmt.Errorf("unable to find ghostwriter repo root: %w", err)
			}

			if tool == "all" {
				results, err := installer.InstallAll(repoRoot)
				if err != nil {
					return err
				}
				for name, result := range results {
					printInstallResult(name, result)
				}
				return nil
			}

			t, err := installer.ToolByName(tool)
			if err != nil {
				return err
			}

			result, err := t.InstallFunc(repoRoot)
			if err != nil {
				return err
			}

			printInstallResult(t.Name, result)
			return nil
		},
	}

	cmd.Flags().StringVar(&tool, "tool", "", "Tool to install for (opencode, claude, cursor, gemini, windsurf, cline, all)")
	_ = cmd.MarkFlagRequired("tool")

	return cmd
}

func printInstallResult(name string, result *installer.InstallResult) {
	log.Info("installed", "tool", name)
	for _, f := range result.FilesCopied {
		log.Info("  copied", "file", f)
	}
	for _, step := range result.ManualSteps {
		fmt.Printf("\n  Manual step required:\n  %s\n", step)
	}
}

// findRepoRoot attempts to find the ghostwriter repo root by looking for AGENTS.md
// in the current directory and parent directories.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(fmt.Sprintf("%s/AGENTS.md", dir)); err == nil {
			if _, err := os.Stat(fmt.Sprintf("%s/skills/ghostwriter/SKILL.md", dir)); err == nil {
				return dir, nil
			}
		}

		parent := fmt.Sprintf("%s/..", dir)
		parent, err = resolveDir(parent)
		if err != nil || parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("ghostwriter repo root not found (looked for AGENTS.md + skills/ghostwriter/SKILL.md)")
}

func resolveDir(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return abs, nil
}
