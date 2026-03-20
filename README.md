# Ghostwriter

Teach AI coding agents to write like you.

Ghostwriter is a toolkit that lets you define your personal writing style and
have AI assistants use it when generating PR reviews, code comments, JIRA
updates, or any technical communication on your behalf. It works across
multiple AI coding tools through a shared style profile and an optional RAG
(Retrieval-Augmented Generation) layer powered by your real writing history.

## How It Works

```
style/profile.yaml          Your style traits (formality, tone, patterns)
style/examples/             10-15 curated writing samples
        |
        v
AGENTS.md / SKILL.md        Style instructions for AI tools
        |
        v
Any AI coding tool           OpenCode, Claude Code, Cursor, Gemini, etc.
        |
        +--> (optional) MCP RAG layer
                |
                v
            qdrant-find      Semantic search over 100s of your past comments
```

**Layer 1 (style instructions)** works standalone with zero infrastructure.
Copy the files, customize them, and your AI tool writes in your voice.

**Layer 2 (RAG)** adds a vector database of your past writing samples. When
generating text, the AI retrieves your most contextually relevant past
comments for even better style matching. This is optional but recommended.

## Tool Compatibility

| Tool | Instructions File | Skill Support | MCP/RAG |
|------|-------------------|---------------|---------|
| **OpenCode** | AGENTS.md | SKILL.md | Yes |
| **Claude Code** | CLAUDE.md (from AGENTS.md) | SKILL.md | Yes |
| **Cursor** | .cursor/rules/ | - | Yes |
| **Gemini CLI** | GEMINI.md (from AGENTS.md) | - | Yes |
| **Windsurf** | .windsurf/rules/ | SKILL.md | Yes |
| **Cline** | .clinerules | SKILL.md | Yes |
| **GitHub Copilot** | .github/copilot-instructions.md | - | - |

## Quickstart

### 1. Fork and clone

```bash
gh repo fork ghostwriter/ghostwriter --clone
cd ghostwriter
```

### 2. Define your style profile

```bash
cp style/profile.example.yaml style/profile.yaml
```

Edit `style/profile.yaml` to match your writing traits: formality level,
common phrases, formatting preferences, anti-patterns.

### 3. Add writing examples

Add 10-15 of your best writing samples to `style/examples/`. See
[style/examples/README.md](style/examples/README.md) for guidance on
what makes a good example.

### 4. Customize the instruction files

Edit these files, replacing the placeholder sections with your own style:

- `AGENTS.md` - universal instructions (works with most tools)
- `skills/ghostwriter/SKILL.md` - skill format (OpenCode, Claude Code, Cline, Windsurf)

Use your `profile.yaml` as a reference and paste your examples from
`style/examples/` into the `<example>` tags.

### 5. Run setup

```bash
./scripts/setup.sh
```

The wizard will:
- Check prerequisites (gh, python3, podman/docker, jq)
- Configure your GitHub username and orgs
- Collect your PR review history from GitHub
- Start Qdrant (vector database) via container
- Ingest your writing samples
- Install for your chosen AI tools

### 6. Test it

Restart your AI coding tool, then try:
- "Review this PR in my style"
- "Write a code review comment as me"
- "Draft feedback on this change in my voice"

## Repository Structure

```
ghostwriter/
├── AGENTS.md                    # Universal style instructions
├── style/
│   ├── profile.example.yaml    # Style profile template
│   ├── anti-patterns.md        # What NOT to do
│   └── examples/               # Your curated writing samples
├── skills/
│   └── ghostwriter/
│       └── SKILL.md            # Skill format for OpenCode/Claude/Cline/Windsurf
├── rules/                      # Tool-specific rule formats
│   ├── cursor.md               # For .cursor/rules/
│   ├── copilot-instructions.md # For .github/copilot-instructions.md
│   └── windsurf.md             # For .windsurf/rules/
├── mcp/                        # MCP config snippets per tool
│   ├── opencode.jsonc
│   ├── claude.jsonc
│   ├── gemini.jsonc
│   ├── cursor.jsonc
│   ├── windsurf.jsonc
│   └── cline.jsonc
├── rag/                        # RAG data pipeline
│   ├── compose.yaml            # Qdrant via Podman/Docker
│   ├── collect-github-reviews.sh
│   ├── ingest.py
│   └── requirements.txt
└── scripts/
    ├── setup.sh                # Interactive setup wizard
    └── install.sh              # Per-tool installer
```

## Customization Guide

### Editing your style profile

The `style/profile.yaml` file is a structured reference to help you think
through your writing traits. It is not consumed by any tool directly. Use
it as a guide when editing `AGENTS.md` and the `SKILL.md`.

Key sections to customize:

- **Voice & Tone**: formality, directness, empathy, teaching orientation
- **Language Patterns**: contractions, colloquialisms, softeners, connectors
- **Formatting**: dash usage, emoji, code references, bullet points
- **Opening Patterns**: how you start comments (casual, prefix, direct)
- **Anti-Patterns**: phrases and patterns you never use

### Adding writing examples

Examples are the single most important input. The AI learns your style
primarily from seeing how you actually write. See
[style/examples/README.md](style/examples/README.md) for detailed guidance.

### Refreshing data

Run these periodically to keep the RAG corpus current:

```bash
./rag/collect-github-reviews.sh
python3 rag/ingest.py --reset
```

### Adding data sources

The initial version supports GitHub PR reviews. To add more sources:

1. Write a collection script (similar to `collect-github-reviews.sh`)
2. Output to `rag/corpus/` in JSONL format with fields:
   `type`, `repo`, `body`, `created_at`
3. Run `ingest.py` to embed and store

## RAG Architecture

The optional RAG layer uses:

- **Qdrant** - local vector database (runs in a container)
- **FastEmbed** - local embedding model (BAAI/bge-small-en-v1.5, no API key needed)
- **mcp-server-qdrant** - MCP protocol bridge between Qdrant and AI tools

When an AI tool generates text in your style, it calls `qdrant-find` to
retrieve your most contextually similar past comments, then uses them as
additional style reference alongside the static instructions.

### Managing Qdrant

```bash
# Start (from rag/ directory)
podman-compose up -d          # or: docker compose up -d

# Stop
podman-compose down           # or: docker compose down

# Check status
curl http://127.0.0.1:6333/collections
```

### Dash normalization

By default, the ingestion script replaces em dashes, en dashes, and double
hyphens with commas in the embedded documents. This ensures the RAG examples
match a comma-based writing style. If you naturally use dashes, disable this:

```bash
python3 rag/ingest.py --reset --no-normalize-dashes
```

## Prerequisites

- **gh** - [GitHub CLI](https://cli.github.com/) (for collecting PR reviews)
- **python3** - Python 3.10+ (for ingestion)
- **podman** or **docker** - container runtime (for Qdrant)
- **jq** - JSON processor (for data collection)
- **mcp-server-qdrant** - `pip install mcp-server-qdrant` (for RAG layer)

## License

MIT
