#!/usr/bin/env bash
#
# Install ghostwriter files into AI coding tool config directories.
#
# Usage:
#   ./scripts/install.sh --tool opencode
#   ./scripts/install.sh --tool claude
#   ./scripts/install.sh --tool cursor
#   ./scripts/install.sh --tool gemini
#   ./scripts/install.sh --tool windsurf
#   ./scripts/install.sh --tool cline
#   ./scripts/install.sh --tool all

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Colors
if [[ -t 1 ]]; then
    GREEN="\033[32m"
    YELLOW="\033[33m"
    RED="\033[31m"
    RESET="\033[0m"
else
    GREEN="" YELLOW="" RED="" RESET=""
fi

info()  { echo -e "${GREEN}[*]${RESET} $*"; }
warn()  { echo -e "${YELLOW}[!]${RESET} $*"; }
error() { echo -e "${RED}[x]${RESET} $*"; }

# Parse arguments
TOOL=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --tool) TOOL="$2"; shift 2 ;;
        *) error "Unknown argument: $1"; exit 1 ;;
    esac
done

if [[ -z "${TOOL}" ]]; then
    echo "Usage: $0 --tool <opencode|claude|cursor|gemini|windsurf|cline|all>"
    exit 1
fi

# ------------------------------------------------------------------
install_opencode() {
    info "Installing for OpenCode..."

    local skill_dir="${HOME}/.config/opencode/skills/ghostwriter"
    mkdir -p "${skill_dir}"
    cp "${REPO_ROOT}/skills/ghostwriter/SKILL.md" "${skill_dir}/SKILL.md"
    info "  Copied SKILL.md to ${skill_dir}/"

    echo ""
    info "  MCP config: merge the following into your ~/.config/opencode/opencode.json"
    echo "  (add under the \"mcp\" key):"
    echo ""
    cat "${REPO_ROOT}/mcp/opencode.jsonc" | grep -v "^//"
    echo ""
}

install_claude() {
    info "Installing for Claude Code..."

    local claude_dir="${HOME}/.claude"
    mkdir -p "${claude_dir}"

    if [[ -f "${claude_dir}/CLAUDE.md" ]]; then
        warn "  ${claude_dir}/CLAUDE.md already exists."
        read -rp "  Append ghostwriter instructions? [y/N]: " append_confirm
        if [[ "${append_confirm}" =~ ^[Yy] ]]; then
            echo "" >> "${claude_dir}/CLAUDE.md"
            echo "<!-- Ghostwriter style instructions -->" >> "${claude_dir}/CLAUDE.md"
            cat "${REPO_ROOT}/AGENTS.md" >> "${claude_dir}/CLAUDE.md"
            info "  Appended to ${claude_dir}/CLAUDE.md"
        else
            info "  Skipped. Copy manually from AGENTS.md."
        fi
    else
        cp "${REPO_ROOT}/AGENTS.md" "${claude_dir}/CLAUDE.md"
        info "  Copied AGENTS.md to ${claude_dir}/CLAUDE.md"
    fi

    # Also install the skill
    local skill_dir="${claude_dir}/skills/ghostwriter"
    mkdir -p "${skill_dir}"
    cp "${REPO_ROOT}/skills/ghostwriter/SKILL.md" "${skill_dir}/SKILL.md"
    info "  Copied SKILL.md to ${skill_dir}/"

    echo ""
    info "  MCP: run this command to add the writing-samples server:"
    echo "    claude mcp add writing-samples \\"
    echo "      -e QDRANT_URL=\"http://127.0.0.1:6333\" \\"
    echo "      -e COLLECTION_NAME=\"writing-samples\" \\"
    echo "      -e TOOL_FIND_DESCRIPTION=\"Search past writing samples for style reference.\" \\"
    echo "      -- mcp-server-qdrant"
    echo ""
}

install_cursor() {
    info "Installing for Cursor..."

    local rules_dir="${HOME}/.cursor/rules"
    mkdir -p "${rules_dir}"
    cp "${REPO_ROOT}/rules/cursor.md" "${rules_dir}/ghostwriter.md"
    info "  Copied rules to ${rules_dir}/ghostwriter.md"

    echo ""
    info "  MCP config: merge the following into ~/.cursor/mcp.json"
    echo "  (add under the \"mcpServers\" key):"
    echo ""
    cat "${REPO_ROOT}/mcp/cursor.jsonc" | grep -v "^//"
    echo ""
}

install_gemini() {
    info "Installing for Gemini CLI..."

    local gemini_dir="${HOME}/.gemini"
    mkdir -p "${gemini_dir}"

    if [[ -f "${gemini_dir}/GEMINI.md" ]]; then
        warn "  ${gemini_dir}/GEMINI.md already exists."
        read -rp "  Append ghostwriter instructions? [y/N]: " append_confirm
        if [[ "${append_confirm}" =~ ^[Yy] ]]; then
            echo "" >> "${gemini_dir}/GEMINI.md"
            echo "<!-- Ghostwriter style instructions -->" >> "${gemini_dir}/GEMINI.md"
            cat "${REPO_ROOT}/AGENTS.md" >> "${gemini_dir}/GEMINI.md"
            info "  Appended to ${gemini_dir}/GEMINI.md"
        else
            info "  Skipped. Copy manually from AGENTS.md."
        fi
    else
        cp "${REPO_ROOT}/AGENTS.md" "${gemini_dir}/GEMINI.md"
        info "  Copied AGENTS.md to ${gemini_dir}/GEMINI.md"
    fi

    echo ""
    info "  MCP config: merge the following into ~/.gemini/settings.json"
    echo "  (add under the \"mcpServers\" key):"
    echo ""
    cat "${REPO_ROOT}/mcp/gemini.jsonc" | grep -v "^//"
    echo ""
}

install_windsurf() {
    info "Installing for Windsurf..."

    local rules_dir="${HOME}/.windsurf/rules"
    mkdir -p "${rules_dir}"
    cp "${REPO_ROOT}/rules/windsurf.md" "${rules_dir}/ghostwriter.md"
    info "  Copied rules to ${rules_dir}/ghostwriter.md"

    # Also install skill
    local skill_dir="${HOME}/.windsurf/skills/ghostwriter"
    mkdir -p "${skill_dir}"
    cp "${REPO_ROOT}/skills/ghostwriter/SKILL.md" "${skill_dir}/SKILL.md"
    info "  Copied SKILL.md to ${skill_dir}/"

    echo ""
    info "  MCP config: merge the following into ~/.codeium/windsurf/mcp_config.json"
    echo "  (add under the \"mcpServers\" key):"
    echo ""
    cat "${REPO_ROOT}/mcp/windsurf.jsonc" | grep -v "^//"
    echo ""
}

install_cline() {
    info "Installing for Cline..."

    # Cline uses .agents/skills/ convention
    local skill_dir="${HOME}/.agents/skills/ghostwriter"
    mkdir -p "${skill_dir}"
    cp "${REPO_ROOT}/skills/ghostwriter/SKILL.md" "${skill_dir}/SKILL.md"
    info "  Copied SKILL.md to ${skill_dir}/"

    echo ""
    info "  MCP: configure via the Cline MCP panel in VS Code."
    echo "  Or merge the following into the Cline MCP settings JSON:"
    echo ""
    cat "${REPO_ROOT}/mcp/cline.jsonc" | grep -v "^//"
    echo ""
}

# ------------------------------------------------------------------
case "${TOOL}" in
    opencode) install_opencode ;;
    claude)   install_claude ;;
    cursor)   install_cursor ;;
    gemini)   install_gemini ;;
    windsurf) install_windsurf ;;
    cline)    install_cline ;;
    all)
        install_opencode
        install_claude
        install_cursor
        install_gemini
        install_windsurf
        install_cline
        ;;
    *)
        error "Unknown tool: ${TOOL}"
        echo "Supported: opencode, claude, cursor, gemini, windsurf, cline, all"
        exit 1
        ;;
esac

info "Installation complete. Restart your AI tool to pick up the changes."
