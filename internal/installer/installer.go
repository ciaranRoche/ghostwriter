// Package installer handles copying ghostwriter style files to the correct
// locations for each supported AI coding tool.
package installer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"
)

// Tool represents a supported AI coding tool.
type Tool struct {
	Name        string
	Description string
	// InstallFunc performs the installation for this tool.
	InstallFunc func(repoRoot string) (*InstallResult, error)
}

// InstallResult holds the result of a tool installation.
type InstallResult struct {
	// FilesCopied is the list of files that were copied.
	FilesCopied []string
	// ManualSteps lists any steps the user needs to do manually.
	ManualSteps []string
}

// SupportedTools returns the list of all supported tools.
func SupportedTools() []Tool {
	return []Tool{
		{Name: "opencode", Description: "OpenCode (SKILL.md + MCP)", InstallFunc: installOpenCode},
		{Name: "claude", Description: "Claude Code (AGENTS.md + MCP)", InstallFunc: installClaude},
		{Name: "cursor", Description: "Cursor (.cursor/rules/ + MCP)", InstallFunc: installCursor},
		{Name: "gemini", Description: "Gemini CLI (GEMINI.md + MCP)", InstallFunc: installGemini},
		{Name: "windsurf", Description: "Windsurf (.windsurf/rules/ + MCP)", InstallFunc: installWindsurf},
		{Name: "cline", Description: "Cline (SKILL.md + MCP)", InstallFunc: installCline},
	}
}

// ToolByName returns a tool by its name.
func ToolByName(name string) (*Tool, error) {
	for _, t := range SupportedTools() {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("unknown tool: %s", name)
}

// InstallAll installs for all supported tools.
func InstallAll(repoRoot string) (map[string]*InstallResult, error) {
	results := make(map[string]*InstallResult)
	for _, tool := range SupportedTools() {
		result, err := tool.InstallFunc(repoRoot)
		if err != nil {
			log.Warn("failed to install for tool", "tool", tool.Name, "error", err)
			continue
		}
		results[tool.Name] = result
	}
	return results, nil
}

func installOpenCode(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy SKILL.md.
	src := filepath.Join(repoRoot, "skills", "ghostwriter", "SKILL.md")
	dst := filepath.Join(home, ".config", "opencode", "skills", "ghostwriter", "SKILL.md")
	if err := copyFile(src, dst); err != nil {
		return nil, fmt.Errorf("failed to copy SKILL.md: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, dst)

	// MCP config needs manual merge.
	mcpSnippet, err := readMCPConfig(filepath.Join(repoRoot, "mcp", "opencode.jsonc"))
	if err == nil && mcpSnippet != "" {
		result.ManualSteps = append(result.ManualSteps,
			fmt.Sprintf("Merge the following MCP config into ~/.config/opencode/opencode.json:\n%s", mcpSnippet))
	}

	return result, nil
}

func installClaude(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy AGENTS.md as CLAUDE.md.
	src := filepath.Join(repoRoot, "AGENTS.md")
	dst := filepath.Join(home, ".claude", "CLAUDE.md")

	if fileExists(dst) {
		// Append to existing file.
		if err := appendToFile(dst, src, "<!-- Ghostwriter style instructions -->"); err != nil {
			return nil, fmt.Errorf("failed to append to CLAUDE.md: %w", err)
		}
		result.FilesCopied = append(result.FilesCopied, dst+" (appended)")
	} else {
		if err := copyFile(src, dst); err != nil {
			return nil, fmt.Errorf("failed to copy CLAUDE.md: %w", err)
		}
		result.FilesCopied = append(result.FilesCopied, dst)
	}

	// Copy SKILL.md.
	skillSrc := filepath.Join(repoRoot, "skills", "ghostwriter", "SKILL.md")
	skillDst := filepath.Join(home, ".claude", "skills", "ghostwriter", "SKILL.md")
	if err := copyFile(skillSrc, skillDst); err != nil {
		return nil, fmt.Errorf("failed to copy SKILL.md: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, skillDst)

	result.ManualSteps = append(result.ManualSteps,
		"Run: claude mcp add ghostwriter-qdrant -- uvx mcp-server-qdrant --qdrant-url http://127.0.0.1:6333 --collection-name writing-samples")

	return result, nil
}

func installCursor(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy rules file.
	src := filepath.Join(repoRoot, "rules", "cursor.md")
	dst := filepath.Join(home, ".cursor", "rules", "ghostwriter.md")
	if err := copyFile(src, dst); err != nil {
		return nil, fmt.Errorf("failed to copy cursor rules: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, dst)

	// MCP config needs manual merge.
	mcpSnippet, err := readMCPConfig(filepath.Join(repoRoot, "mcp", "cursor.jsonc"))
	if err == nil && mcpSnippet != "" {
		result.ManualSteps = append(result.ManualSteps,
			fmt.Sprintf("Merge the following MCP config into ~/.cursor/mcp.json:\n%s", mcpSnippet))
	}

	return result, nil
}

func installGemini(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy AGENTS.md as GEMINI.md.
	src := filepath.Join(repoRoot, "AGENTS.md")
	dst := filepath.Join(home, ".gemini", "GEMINI.md")

	if fileExists(dst) {
		if err := appendToFile(dst, src, "<!-- Ghostwriter style instructions -->"); err != nil {
			return nil, fmt.Errorf("failed to append to GEMINI.md: %w", err)
		}
		result.FilesCopied = append(result.FilesCopied, dst+" (appended)")
	} else {
		if err := copyFile(src, dst); err != nil {
			return nil, fmt.Errorf("failed to copy GEMINI.md: %w", err)
		}
		result.FilesCopied = append(result.FilesCopied, dst)
	}

	// MCP config needs manual merge.
	mcpSnippet, err := readMCPConfig(filepath.Join(repoRoot, "mcp", "gemini.jsonc"))
	if err == nil && mcpSnippet != "" {
		result.ManualSteps = append(result.ManualSteps,
			fmt.Sprintf("Merge the following MCP config into ~/.gemini/settings.json:\n%s", mcpSnippet))
	}

	return result, nil
}

func installWindsurf(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy rules file.
	rulesSrc := filepath.Join(repoRoot, "rules", "windsurf.md")
	rulesDst := filepath.Join(home, ".windsurf", "rules", "ghostwriter.md")
	if err := copyFile(rulesSrc, rulesDst); err != nil {
		return nil, fmt.Errorf("failed to copy windsurf rules: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, rulesDst)

	// Copy SKILL.md.
	skillSrc := filepath.Join(repoRoot, "skills", "ghostwriter", "SKILL.md")
	skillDst := filepath.Join(home, ".windsurf", "skills", "ghostwriter", "SKILL.md")
	if err := copyFile(skillSrc, skillDst); err != nil {
		return nil, fmt.Errorf("failed to copy SKILL.md: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, skillDst)

	// MCP config needs manual merge.
	mcpSnippet, err := readMCPConfig(filepath.Join(repoRoot, "mcp", "windsurf.jsonc"))
	if err == nil && mcpSnippet != "" {
		result.ManualSteps = append(result.ManualSteps,
			fmt.Sprintf("Merge the following MCP config into ~/.codeium/windsurf/mcp_config.json:\n%s", mcpSnippet))
	}

	return result, nil
}

func installCline(repoRoot string) (*InstallResult, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	result := &InstallResult{}

	// Copy SKILL.md.
	src := filepath.Join(repoRoot, "skills", "ghostwriter", "SKILL.md")
	dst := filepath.Join(home, ".agents", "skills", "ghostwriter", "SKILL.md")
	if err := copyFile(src, dst); err != nil {
		return nil, fmt.Errorf("failed to copy SKILL.md: %w", err)
	}
	result.FilesCopied = append(result.FilesCopied, dst)

	// MCP config needs manual merge.
	mcpSnippet, err := readMCPConfig(filepath.Join(repoRoot, "mcp", "cline.jsonc"))
	if err == nil && mcpSnippet != "" {
		result.ManualSteps = append(result.ManualSteps,
			fmt.Sprintf("Merge the following MCP config for Cline:\n%s", mcpSnippet))
	}

	return result, nil
}

// copyFile copies a file from src to dst, creating parent directories as needed.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// appendToFile appends the contents of src to an existing dst file with a marker.
func appendToFile(dst, src, marker string) error {
	srcContent, err := os.ReadFile(src)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(dst, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	content := fmt.Sprintf("\n\n%s\n%s", marker, string(srcContent))
	_, err = f.WriteString(content)
	return err
}

// readMCPConfig reads an MCP config file and strips comment lines.
func readMCPConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "//") {
			lines = append(lines, line)
		}
	}

	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

// fileExists returns true if a file exists and is not a directory.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
