#!/usr/bin/env bash
#
# Ghostwriter setup wizard
# Walks through initial configuration, data collection, and installation.
#
# Usage: ./scripts/setup.sh

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# Colors (if terminal supports them)
if [[ -t 1 ]]; then
    BOLD="\033[1m"
    DIM="\033[2m"
    GREEN="\033[32m"
    YELLOW="\033[33m"
    RED="\033[31m"
    RESET="\033[0m"
else
    BOLD="" DIM="" GREEN="" YELLOW="" RED="" RESET=""
fi

info()  { echo -e "${GREEN}[*]${RESET} $*"; }
warn()  { echo -e "${YELLOW}[!]${RESET} $*"; }
error() { echo -e "${RED}[x]${RESET} $*"; }
step()  { echo -e "\n${BOLD}=== $* ===${RESET}"; }

# ------------------------------------------------------------------
step "Ghostwriter Setup"
echo "This wizard will help you configure ghostwriter for your personal"
echo "writing style. It will:"
echo "  1. Check prerequisites"
echo "  2. Configure your .env file"
echo "  3. Collect your writing samples from GitHub"
echo "  4. Start Qdrant (vector database)"
echo "  5. Ingest your samples into Qdrant"
echo "  6. Install for your AI coding tools"
echo ""

# ------------------------------------------------------------------
step "1. Checking prerequisites"

missing=()

if command -v gh &>/dev/null; then
    info "GitHub CLI (gh): $(gh --version | head -1)"
else
    missing+=("gh (GitHub CLI) - https://cli.github.com/")
fi

if command -v python3 &>/dev/null; then
    info "Python: $(python3 --version)"
else
    missing+=("python3 - https://www.python.org/downloads/")
fi

if command -v jq &>/dev/null; then
    info "jq: $(jq --version)"
else
    missing+=("jq - https://jqlang.github.io/jq/download/")
fi

# Detect container runtime
CONTAINER_RT=""
COMPOSE_CMD=""
if command -v podman &>/dev/null; then
    CONTAINER_RT="podman"
    info "Container runtime: podman $(podman --version | awk '{print $NF}')"
    if command -v podman-compose &>/dev/null; then
        COMPOSE_CMD="podman-compose"
    elif podman compose version &>/dev/null 2>&1; then
        COMPOSE_CMD="podman compose"
    fi
elif command -v docker &>/dev/null; then
    CONTAINER_RT="docker"
    info "Container runtime: docker $(docker --version | awk '{print $3}' | tr -d ',')"
    if docker compose version &>/dev/null 2>&1; then
        COMPOSE_CMD="docker compose"
    elif command -v docker-compose &>/dev/null; then
        COMPOSE_CMD="docker-compose"
    fi
fi

if [[ -z "${CONTAINER_RT}" ]]; then
    missing+=("podman or docker - needed to run Qdrant")
fi

if [[ -z "${COMPOSE_CMD}" ]]; then
    warn "No compose command found. Will use '${CONTAINER_RT} run' instead."
fi

if [[ ${#missing[@]} -gt 0 ]]; then
    error "Missing prerequisites:"
    for m in "${missing[@]}"; do
        echo "  - ${m}"
    done
    echo ""
    echo "Install the missing tools and re-run this script."
    exit 1
fi

info "All prerequisites met."

# ------------------------------------------------------------------
step "2. Configuring .env"

if [[ -f "${REPO_ROOT}/.env" ]]; then
    info ".env file already exists. Loading existing values."
    set -a
    # shellcheck disable=SC1091
    source "${REPO_ROOT}/.env"
    set +a
else
    cp "${REPO_ROOT}/.env.example" "${REPO_ROOT}/.env"
    info "Created .env from .env.example"
fi

# Prompt for GitHub username
current_user="${GITHUB_USERNAME:-}"
if [[ -z "${current_user}" || "${current_user}" == "your-github-username" ]]; then
    # Try to detect from gh CLI
    detected_user=$(gh api user --jq '.login' 2>/dev/null || true)
    if [[ -n "${detected_user}" ]]; then
        read -rp "GitHub username [${detected_user}]: " input_user
        current_user="${input_user:-${detected_user}}"
    else
        read -rp "GitHub username: " current_user
    fi
fi
info "Username: ${current_user}"

# Prompt for GitHub orgs
current_orgs="${GITHUB_ORGS:-}"
if [[ -z "${current_orgs}" || "${current_orgs}" == "your-org-1,your-org-2" ]]; then
    # Try to detect orgs from gh CLI
    detected_orgs=$(gh api user/orgs --jq '.[].login' 2>/dev/null | paste -sd, || true)
    if [[ -n "${detected_orgs}" ]]; then
        echo "Detected orgs: ${detected_orgs}"
        read -rp "GitHub orgs (comma-separated) [${detected_orgs}]: " input_orgs
        current_orgs="${input_orgs:-${detected_orgs}}"
    else
        read -rp "GitHub orgs (comma-separated): " current_orgs
    fi
fi
info "Orgs: ${current_orgs}"

# Update .env file
sed -i "s|^GITHUB_USERNAME=.*|GITHUB_USERNAME=${current_user}|" "${REPO_ROOT}/.env"
sed -i "s|^GITHUB_ORGS=.*|GITHUB_ORGS=${current_orgs}|" "${REPO_ROOT}/.env"

# ------------------------------------------------------------------
step "3. Collecting writing samples from GitHub"

echo "This will pull your PR review comments from the configured orgs."
read -rp "Proceed? [Y/n]: " collect_confirm
collect_confirm="${collect_confirm:-Y}"

if [[ "${collect_confirm}" =~ ^[Yy] ]]; then
    bash "${REPO_ROOT}/rag/collect-github-reviews.sh" "${current_orgs}" "${current_user}"
else
    warn "Skipping collection. Run manually later:"
    echo "  ./rag/collect-github-reviews.sh"
fi

# ------------------------------------------------------------------
step "4. Starting Qdrant"

if curl -sf http://127.0.0.1:6333/healthz &>/dev/null; then
    info "Qdrant is already running on port 6333."
else
    echo "Starting Qdrant via ${CONTAINER_RT}..."

    if [[ -n "${COMPOSE_CMD}" ]]; then
        ${COMPOSE_CMD} -f "${REPO_ROOT}/rag/compose.yaml" up -d
    else
        ${CONTAINER_RT} run -d \
            --name ghostwriter-qdrant \
            -p 6333:6333 \
            -p 6334:6334 \
            -v qdrant_storage:/qdrant/storage:Z \
            docker.io/qdrant/qdrant:latest
    fi

    # Wait for Qdrant to be ready
    echo "Waiting for Qdrant to start..."
    for i in $(seq 1 15); do
        if curl -sf http://127.0.0.1:6333/healthz &>/dev/null; then
            break
        fi
        sleep 1
    done

    if curl -sf http://127.0.0.1:6333/healthz &>/dev/null; then
        info "Qdrant is running."
    else
        error "Qdrant failed to start. Check container logs:"
        echo "  ${CONTAINER_RT} logs ghostwriter-qdrant"
        exit 1
    fi
fi

# ------------------------------------------------------------------
step "5. Ingesting samples into Qdrant"

# Install Python dependencies
echo "Installing Python dependencies..."
pip3 install -q -r "${REPO_ROOT}/rag/requirements.txt" 2>&1 | tail -1

# Check if corpus exists
corpus_file="${REPO_ROOT}/rag/corpus/reviews.jsonl"
if [[ ! -f "${corpus_file}" ]] || [[ ! -s "${corpus_file}" ]]; then
    warn "No corpus file found. Skipping ingestion."
    echo "Run the collection script first, then re-run setup."
else
    python3 "${REPO_ROOT}/rag/ingest.py" --reset
    info "Ingestion complete."
fi

# ------------------------------------------------------------------
step "6. Installing for AI tools"

echo "Which tools do you want to install ghostwriter for?"
echo ""
echo "  1) opencode    - OpenCode (SKILL.md + MCP)"
echo "  2) claude      - Claude Code (AGENTS.md + MCP)"
echo "  3) cursor      - Cursor (.cursor/rules/ + MCP)"
echo "  4) gemini      - Gemini CLI (GEMINI.md + MCP)"
echo "  5) windsurf    - Windsurf (.windsurf/rules/ + MCP)"
echo "  6) cline       - Cline (SKILL.md + MCP)"
echo "  7) all         - All detected tools"
echo "  8) skip        - Skip installation (do it manually later)"
echo ""
read -rp "Choice [8]: " tool_choice
tool_choice="${tool_choice:-8}"

case "${tool_choice}" in
    1) bash "${REPO_ROOT}/scripts/install.sh" --tool opencode ;;
    2) bash "${REPO_ROOT}/scripts/install.sh" --tool claude ;;
    3) bash "${REPO_ROOT}/scripts/install.sh" --tool cursor ;;
    4) bash "${REPO_ROOT}/scripts/install.sh" --tool gemini ;;
    5) bash "${REPO_ROOT}/scripts/install.sh" --tool windsurf ;;
    6) bash "${REPO_ROOT}/scripts/install.sh" --tool cline ;;
    7) bash "${REPO_ROOT}/scripts/install.sh" --tool all ;;
    8|"skip")
        info "Skipping tool installation."
        echo "Run manually later: ./scripts/install.sh --tool <tool>"
        ;;
    *) warn "Unknown choice. Skipping installation." ;;
esac

# ------------------------------------------------------------------
step "Setup Complete"

echo ""
echo "Next steps:"
echo ""
echo "  1. Customize your style:"
echo "     - Copy style/profile.example.yaml to style/profile.yaml"
echo "     - Edit AGENTS.md and skills/ghostwriter/SKILL.md"
echo "     - Add writing examples to style/examples/"
echo ""
echo "  2. Test it:"
echo "     - Restart your AI coding tool"
echo "     - Ask: \"Review this PR in my style\""
echo "     - Ask: \"Write a code review comment as me\""
echo ""
echo "  3. Refresh data periodically:"
echo "     - ./rag/collect-github-reviews.sh"
echo "     - python3 rag/ingest.py --reset"
echo ""
info "Ghostwriter is ready."
