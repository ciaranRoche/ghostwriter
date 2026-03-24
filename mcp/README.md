# MCP Configuration Snippets

These files contain ready-to-merge JSON snippets for configuring the
`mcp-server-qdrant` MCP server in each supported AI coding tool. This
server provides the RAG (Retrieval-Augmented Generation) layer that lets
tools search your past writing samples for style-relevant examples.

## Architecture

```
Your past writing samples (50-300+ docs)
        |
        v
  gw ingest (embed + store)
        |
        v
  Qdrant vector DB (local container, port 6333)
        |
        v
  mcp-server-qdrant (MCP protocol)
        |
        v
  Any MCP client: OpenCode / Claude Code / Cursor / Gemini / Windsurf / Cline
```

## Prerequisites

1. **Qdrant running**: `gw qdrant start`
2. **mcp-server-qdrant installed**: `pip install mcp-server-qdrant`
3. **Corpus ingested**: run `gw collect github` then `gw ingest`

## Per-Tool Setup

### OpenCode
Merge `opencode.jsonc` into your `~/.config/opencode/opencode.json` under
the `"mcp"` key.

### Claude Code
Merge `claude.jsonc` into `.mcp.json` in your project root, or run the
`claude mcp add` command shown in the file comments.

### Gemini CLI
Merge `gemini.jsonc` into `~/.gemini/settings.json` under the
`"mcpServers"` key.

### Cursor
Merge `cursor.jsonc` into `.cursor/mcp.json` in your project root or
`~/.cursor/mcp.json` globally.

### Windsurf
Merge `windsurf.jsonc` into `~/.codeium/windsurf/mcp_config.json`.

### Cline
Merge `cline.jsonc` into Cline's MCP settings file, or configure via the
Cline MCP panel in VS Code.

## Environment Variables

All snippets reference these environment variables (set in `.env`):

| Variable | Default | Description |
|----------|---------|-------------|
| `QDRANT_URL` | `http://127.0.0.1:6333` | Qdrant server URL |
| `COLLECTION_NAME` | `writing-samples` | Qdrant collection name |
| `TOOL_FIND_DESCRIPTION` | (see .env.example) | Description shown to the AI for the search tool |
| `TOOL_STORE_DESCRIPTION` | (see .env.example) | Description shown to the AI for the store tool |
