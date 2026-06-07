# TinkerView — guidance for Claude

**Read `handoff.md` (repo root, gitignored) first** — it's the source of truth for
project state, ops, deploy, and backlog.

## MCP tool routing (two servers, namespaced — no collision)
- **`mcp__graphify__*`** (`query_graph`, `get_node`, `get_neighbors`, `shortest_path`,
  + PR-impact tools): use for questions about **this repo's structure** — symbols,
  dependencies, call paths, "what calls X / what breaks if I change Y". Backed by a
  per-repo knowledge graph at `graphify-out/graph.json` (gitignored, served via a
  local-scope MCP). Rebuild after code changes:
  - `graphify update .` — AST-only, fast, **free** (no LLM).
  - `graphify extract . --backend=claude-cli` — full semantic rebuild on the **Claude
    Max subscription** (no `ANTHROPIC_API_KEY`; set `GRAPHIFY_CLAUDE_CLI_MODEL=sonnet`).
- **`mcp__openbrain__*`** (`search_thoughts`, `capture_thought`): use to recall/store
  **cross-session, cross-project** decisions, gotchas, and rationale.

## Deploy (summary — details in handoff.md)
Redeploy = rsync the repo to `root@tower.local:/mnt/user/appdata/webcamserver`
(**must exclude tower-specific configs** — see handoff.md) then rebuild the dashboard.
