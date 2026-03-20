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

### 1. Install the CLI

```bash
# From source
gh repo fork ghostwriter/ghostwriter --clone
cd ghostwriter
make install

# Or build locally
make build
./bin/gw --help
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
gw init
```

The interactive wizard will:
- Detect your container runtime (podman/docker)
- Configure your GitHub username and orgs (auto-detected from `gh`)
- Collect your PR review history from GitHub
- Start Qdrant (vector database) via container
- Ingest your writing samples
- Install for your chosen AI tools

### 6. Test it

Restart your AI coding tool, then try:
- "Review this PR in my style"
- "Write a code review comment as me"
- "Draft feedback on this change in my voice"

## CLI Reference

```
gw init                         Interactive setup wizard
gw collect github               Collect PR reviews from GitHub
gw ingest                       Embed and upsert samples into Qdrant
gw install --tool <name|all>    Install style files for AI tools
gw qdrant start|stop|status     Manage the Qdrant container
gw config show|set|path         View and update configuration
gw version                      Print version info
```

### `gw collect github`

Collects your PR review comments from GitHub using the GraphQL API.

```bash
gw collect github                          # uses config values
gw collect github --username myuser        # override username
gw collect github --orgs org1,org2         # override orgs
```

### `gw ingest`

Processes collected writing samples and stores them in Qdrant.

```bash
gw ingest                                  # append to existing collection
gw ingest --reset                          # delete and recreate collection
gw ingest --no-normalize-dashes            # disable dash-to-comma normalization
gw ingest --qdrant-url http://host:6333    # override Qdrant URL
gw ingest --collection my-samples          # override collection name
```

### `gw install`

Installs ghostwriter style files for your AI tools.

```bash
gw install --tool opencode     # install for OpenCode
gw install --tool claude       # install for Claude Code
gw install --tool cursor       # install for Cursor
gw install --tool gemini       # install for Gemini CLI
gw install --tool windsurf     # install for Windsurf
gw install --tool cline        # install for Cline
gw install --tool all          # install for all tools
```

### `gw config`

```bash
gw config show                             # display current config
gw config set github.username myuser       # set a value
gw config set github.orgs "org1,org2"      # set orgs
gw config path                             # print config file path
```

Configuration is stored at `~/.config/ghostwriter/config.yaml` and can be
overridden with environment variables prefixed with `GW_` (e.g.,
`GW_GITHUB_USERNAME`, `GW_QDRANT_URL`). Legacy environment variable names
(`GITHUB_USERNAME`, `QDRANT_URL`, etc.) are also supported.

## Repository Structure

```
ghostwriter/
├── cmd/gw/                      # CLI entry point
├── internal/
│   ├── cli/                     # Cobra command definitions
│   ├── config/                  # Viper-based configuration
│   ├── collector/               # Data collection (GitHub, extensible)
│   ├── embedder/                # Qdrant ingestion pipeline
│   ├── installer/               # Per-tool file installation
│   ├── container/               # Container runtime abstraction
│   └── tui/                     # Interactive form definitions
├── AGENTS.md                    # Universal style instructions
├── style/
│   ├── profile.example.yaml    # Style profile template
│   ├── anti-patterns.md        # What NOT to do
│   └── examples/               # Your curated writing samples
├── skills/
│   └── ghostwriter/
│       └── SKILL.md            # Skill format for OpenCode/Claude/Cline/Windsurf
├── rules/                      # Tool-specific rule formats
│   ├── cursor.md
│   ├── copilot-instructions.md
│   └── windsurf.md
├── mcp/                        # MCP config snippets per tool
├── rag/
│   └── compose.yaml            # Qdrant via Podman/Docker
├── Makefile                    # Build, install, lint, test targets
└── go.mod
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
gw collect github
gw ingest --reset
```

### Adding data sources

The `collect` command is extensible. Currently it supports GitHub PR reviews.
Future collectors (JIRA, Slack, etc.) will be added as subcommands under
`gw collect`. To add a custom source, output JSONL to `rag/corpus/` with
fields: `type`, `repo`, `body`, `created_at`, then run `gw ingest`.

## RAG Architecture

The optional RAG layer uses:

- **Qdrant** - local vector database (runs in a container)
- **mcp-server-qdrant** - MCP protocol bridge between Qdrant and AI tools

When an AI tool generates text in your style, it calls `qdrant-find` to
retrieve your most contextually similar past comments, then uses them as
additional style reference alongside the static instructions.

### Managing Qdrant

```bash
gw qdrant start       # start the container
gw qdrant stop        # stop the container
gw qdrant status      # check health
```

### Dash normalization

By default, the ingestion step replaces em dashes, en dashes, and double
hyphens with commas in the embedded documents. This ensures the RAG examples
match a comma-based writing style. If you naturally use dashes, disable this:

```bash
gw ingest --reset --no-normalize-dashes
```

Or set it permanently:

```bash
gw config set style.normalize_dashes false
```

## Prerequisites

- **Go 1.21+** - for building the CLI
- **podman** or **docker** - container runtime (for Qdrant)
- **gh** (optional) - [GitHub CLI](https://cli.github.com/) for auth token fallback
- **mcp-server-qdrant** - `pip install mcp-server-qdrant` (for RAG layer)

## License

MIT
