#!/usr/bin/env bash
#
# Collects PR review comments from GitHub for the ghostwriter corpus.
# Pulls both review summary comments and inline code comments.
#
# Usage:
#   ./collect-github-reviews.sh                    # reads from .env
#   ./collect-github-reviews.sh <org> <username>   # explicit args
#
# Environment variables (or set in .env at repo root):
#   GITHUB_USERNAME  - GitHub username to collect reviews for
#   GITHUB_ORGS      - Comma-separated list of GitHub orgs to search
#
# Output: rag/corpus/reviews.jsonl

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Load .env if it exists
if [[ -f "${REPO_ROOT}/.env" ]]; then
    # shellcheck disable=SC1091
    set -a
    source "${REPO_ROOT}/.env"
    set +a
fi

# Resolve configuration
if [[ $# -ge 2 ]]; then
    ORGS_INPUT="$1"
    USERNAME="$2"
elif [[ $# -eq 1 ]]; then
    ORGS_INPUT="$1"
    USERNAME="${GITHUB_USERNAME:-}"
else
    ORGS_INPUT="${GITHUB_ORGS:-}"
    USERNAME="${GITHUB_USERNAME:-}"
fi

if [[ -z "${USERNAME}" ]]; then
    echo "Error: GitHub username not set."
    echo "Set GITHUB_USERNAME in .env or pass as second argument."
    exit 1
fi

if [[ -z "${ORGS_INPUT}" ]]; then
    echo "Error: GitHub orgs not set."
    echo "Set GITHUB_ORGS in .env or pass as first argument."
    exit 1
fi

# Check prerequisites
if ! command -v gh &>/dev/null; then
    echo "Error: GitHub CLI (gh) is not installed."
    echo "Install it: https://cli.github.com/"
    exit 1
fi

if ! command -v jq &>/dev/null; then
    echo "Error: jq is not installed."
    echo "Install it: https://jqlang.github.io/jq/download/"
    exit 1
fi

OUTPUT_DIR="${SCRIPT_DIR}/corpus"
OUTPUT_FILE="${OUTPUT_DIR}/reviews.jsonl"
BATCH_SIZE=20
MAX_PAGES=10

mkdir -p "${OUTPUT_DIR}"

# Clear previous output
> "${OUTPUT_FILE}"

# Split comma-separated orgs
IFS=',' read -ra ORGS <<< "${ORGS_INPUT}"

echo "Collecting PR reviews by ${USERNAME}..."
echo "Orgs: ${ORGS_INPUT}"
echo "Output: ${OUTPUT_FILE}"
echo ""

grand_total=0

for ORG in "${ORGS[@]}"; do
    # Trim whitespace
    ORG="$(echo "${ORG}" | xargs)"
    echo "=== Org: ${ORG} ==="

    total_comments=0
    has_next="true"
    cursor=""

    for ((page=1; page<=MAX_PAGES; page++)); do
        if [[ "${has_next}" != "true" ]]; then
            break
        fi

        # Build the after clause for pagination
        after_clause=""
        if [[ -n "${cursor}" ]]; then
            after_clause=", after: \"${cursor}\""
        fi

        echo "  Fetching page ${page}..."

        result=$(gh api graphql -f query="
        {
          search(query: \"is:pr reviewed-by:${USERNAME} org:${ORG} sort:updated-desc\", type: ISSUE, first: ${BATCH_SIZE}${after_clause}) {
            pageInfo {
              hasNextPage
              endCursor
            }
            nodes {
              ... on PullRequest {
                number
                title
                repository { nameWithOwner }
                createdAt
                reviews(author: \"${USERNAME}\", first: 10) {
                  nodes {
                    body
                    state
                    createdAt
                    comments(first: 20) {
                      nodes {
                        body
                        path
                        createdAt
                      }
                    }
                  }
                }
              }
            }
          }
        }
        ")

        # Extract pagination info
        has_next=$(echo "${result}" | jq -r '.data.search.pageInfo.hasNextPage')
        cursor=$(echo "${result}" | jq -r '.data.search.pageInfo.endCursor')

        # Extract review summaries (non-empty, non-lgtm-only bodies)
        page_comments=$(echo "${result}" | jq -r --arg user "${USERNAME}" '
          .data.search.nodes[] |
          . as $pr |
          .reviews.nodes[] |
          # Review summary comments
          (
            select(.body != null and .body != "" and (.body | test("^\\s*/lgtm\\s*$"; "i") | not)) |
            {
              type: "review_summary",
              repo: $pr.repository.nameWithOwner,
              pr_number: $pr.number,
              pr_title: $pr.title,
              body: .body,
              state: .state,
              file_path: null,
              created_at: .createdAt
            }
          ),
          # Inline code comments
          (
            .comments.nodes[] |
            select(.body != null and .body != "" and (.body | length > 20)) |
            {
              type: "inline_comment",
              repo: $pr.repository.nameWithOwner,
              pr_number: $pr.number,
              pr_title: $pr.title,
              body: .body,
              state: null,
              file_path: .path,
              created_at: .createdAt
            }
          )
        ' 2>/dev/null)

        if [[ -n "${page_comments}" ]]; then
            count=$(echo "${page_comments}" | jq -s 'length')
            echo "${page_comments}" >> "${OUTPUT_FILE}"
            total_comments=$((total_comments + count))
            echo "    Found ${count} comments (total for org: ${total_comments})"
        else
            echo "    No comments on this page"
        fi

        # Small delay to avoid rate limiting
        sleep 0.5
    done

    grand_total=$((grand_total + total_comments))
    echo "  Org total: ${total_comments}"
    echo ""
done

# Deduplicate by body content and output as proper JSONL
if [[ -f "${OUTPUT_FILE}" ]]; then
    tmp_file=$(mktemp)
    jq -s 'unique_by(.body) | sort_by(.created_at) | reverse | .[]' "${OUTPUT_FILE}" | jq -c '.' > "${tmp_file}" 2>/dev/null || true
    mv "${tmp_file}" "${OUTPUT_FILE}"
    final_count=$(jq -s 'length' "${OUTPUT_FILE}" 2>/dev/null || echo "0")
    echo "Done! ${final_count} unique comments saved to ${OUTPUT_FILE}"
else
    echo "Done! No comments found."
fi
