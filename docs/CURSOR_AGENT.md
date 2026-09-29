# Cursor Agent Integration

Perles supports [Cursor Agent CLI](https://cursor.com) as an orchestration provider. The cursor provider enables Cursor to serve as coordinator, worker, or observer in multi-agent workflows.

## Configuration

Set `cursor` as the client in your Perles config:

```yaml
orchestration:
  coordinator_client: cursor
  worker_client: cursor
  observer_enabled: true       # optional, defaults to false
  observer_client: cursor      # optional, defaults to claude
  cursor:
    model: composer-1           # any model Cursor supports
```

You can mix providers — for example, use `cursor` for the coordinator and `claude` for workers.

## How It Works

### Headless Mode

Cursor Agent runs in headless mode with structured JSON output:

```
cursor-agent --print --output-format stream-json --model <model> --force --approve-mcps "prompt"
```

Key flags:
- `--print` — non-interactive mode for automation
- `--output-format stream-json` — structured JSONL for parsing
- `--model` — model selection (e.g., `composer-1`)
- `--resume <id>` — resume an existing session
- `--force` — allow file modifications without confirmation
- `--approve-mcps` — auto-approve MCP servers (added automatically)

### MCP Configuration (File-Based)

Cursor CLI does not accept `--mcp-config` as a command-line flag. It reads MCP server configuration from `.cursor/mcp.json` in its working directory.

Before spawning `cursor-agent`, Perles:

1. Generates the MCP config JSON pointing to the orchestration HTTP server
2. Writes it to `{workDir}/.cursor/mcp.json`
3. Merges with any existing user-defined servers in that file
4. Passes `--approve-mcps` so Cursor connects without interactive approval

The generated config looks like:

```json
{
  "mcpServers": {
    "perles-orchestrator": {
      "url": "http://localhost:<port>/mcp"
    }
  }
}
```

Workers and the observer get their own MCP entries pointing to role-specific endpoints. Each worker uses a unique server name (e.g., `perles-worker-1`, `perles-worker-2`) so multiple workers can coexist in the same file. The observer uses `perles-observer`.

### System Prompt

Cursor does not support `--append-system-prompt`. The system prompt is prepended to the main prompt with a blank line separator — the same approach OpenCode uses.

### Session Resumption

Cursor supports session resumption via `--resume <session-id>`. The session ID is extracted from the `init` event in the stream-json output. Every follow-up message to an existing process (user input, inter-agent messages, health-check nudges) re-spawns `cursor-agent` with `--resume <session-id>`. Resuming a paused workflow continues the same sessions; for a workflow restored from disk, the session IDs saved in its session metadata are restored first.

Context-exhaustion recovery does not resume. An exhausted coordinator is replaced with a fresh session and a handoff prompt, and an exhausted observer is replaced with a fresh session that recovers from its notes file. An exhausted worker is marked failed so the coordinator can replace it.

## CLI Requirements

Perles looks for `cursor-agent` in these locations, in order, and then falls back to `PATH`:

- `~/.local/bin/cursor-agent`
- `/opt/homebrew/bin/cursor-agent` (Apple Silicon Mac)
- `/usr/local/bin/cursor-agent` (Intel Mac / Linux)

Install from: https://cursor.com/install

## Integration Tests

Perles includes optional integration tests that exercise the real Cursor Agent CLI.
These are build-tagged and skipped automatically if `cursor-agent` is not installed
or not configured.

Run with:

```bash
go test -tags=cursor_integration ./internal/orchestration/client/providers/cursor
```

Notes:
- Tests use the `composer-1` model.
- They are intended for local/dev environments, not CI, unless Cursor is installed.

## Known Limitations

### No Token Usage Reporting

Cursor's stream-json output does not include token counts or cost data. The Tokens and Cost fields in the Perles session view show zero. This is a limitation of the Cursor CLI output format — assistant events lack `message.usage`, and result events lack `total_cost_usd` and `modelUsage`.

### No Tool Filtering

Cursor CLI does not support `--allowed-tools` or `--disallowed-tools`. All tools available to the agent remain enabled. The `DisallowedTools` field in `client.Config` (used to block `AskUserQuestion` in headless mode) is silently ignored.

### Shared MCP Config File

Claude and Amp pass MCP config via `--mcp-config`, Codex via `-c mcp_servers.*`, and OpenCode via the `OPENCODE_CONFIG_CONTENT` environment variable. Like Gemini (which writes `{workDir}/.gemini/settings.json`), Cursor reads MCP config only from a file in the working directory: `.cursor/mcp.json`. This creates a constraint: all processes in a workflow share the same working directory, so they share a single config file.

#### How the current solution works

Each process writes its MCP server entry to `{workDir}/.cursor/mcp.json` before spawning. The file uses unique server names per process:

- Coordinator: `perles-orchestrator` → `http://localhost:<port>/mcp`
- Worker 1: `perles-worker-1` → `http://localhost:<port>/worker/worker-1`
- Worker 2: `perles-worker-2` → `http://localhost:<port>/worker/worker-2`
- Observer: `perles-observer` → `http://localhost:<port>/observer`

Writes use a read-merge-write pattern: read any existing file, add/overwrite our entries, write back. This preserves user-defined MCP servers already in the file (if the existing file is not valid JSON, it is replaced). A process's entry is rewritten before every spawn, including each `--resume`. Each `cursor-agent` process reads the file at startup and, because Perles passes `--approve-mcps`, connects to every server listed in it.

Perles does not enforce role separation, since the MCP endpoints don't check which process is calling. A Cursor worker can therefore use other workers' tools and, when the coordinator also runs on Cursor, coordinator tools via `perles-orchestrator` (likewise observer tools via `perles-observer` when the observer runs on Cursor). Only the prompts keep each agent on its own role's tools.

When running in **worktree mode**, the working directory is the workflow's git worktree (newly created or an existing one you selected), so `.cursor/mcp.json` is written there instead of the main project directory. Perles does not delete worktrees when a workflow ends, so the file stays until you remove the worktree yourself.

Workflows that run at the same time in the same directory (for example, two workflows started with **No Worktree**) also share the file. Their entries have the same names (`perles-orchestrator`, `perles-worker-1`, ...), so each workflow's writes overwrite the other's, and a Cursor process can end up connected to the other workflow's servers. Give each workflow its own worktree to keep the files separate.

Perles never deletes or git-ignores `.cursor/mcp.json`, and writes only add or update entries. Entries from earlier runs (for example a `perles-worker-5` left by a workflow that had more workers) are never removed, so later `cursor-agent` runs also try to connect to those dead servers. Add `.cursor/mcp.json` (or `.cursor/`) to your repo's `.gitignore` so it doesn't show up in `git status` or get committed by agents, and delete the file to clear stale entries. If your repo already tracks `.cursor/mcp.json`, Perles modifies that tracked file, so review diffs before committing.

#### Alternatives explored and rejected

**Per-process HOME directory override.** The idea: create a temporary directory per process, write `.cursor/mcp.json` there, and set `HOME` to the temp directory so cursor reads `~/.cursor/mcp.json` from the isolated location. The process's cwd would remain the real project directory, so file operations would work normally. This failed because cursor-agent's authentication is tied to the real home directory through a mechanism that doesn't follow symlinks or file copies — overriding HOME produces "Authentication required" errors. Even symlinking all files from the real `~/.cursor/` (except `mcp.json`) into the temp directory didn't help; auth appears to depend on the actual HOME path, likely via a platform-specific secret store.

**Per-worker working directories.** Give each worker its own subdirectory so each gets a separate `.cursor/mcp.json`. Rejected because cursor-agent needs the project root as its cwd to read and write code files. Subdirectories break file operations.

**CURSOR_API_KEY with HOME override.** Cursor supports a `CURSOR_API_KEY` environment variable as an alternative to interactive login. Combined with a per-process HOME, this would provide full isolation. Not pursued because it requires users to configure an API key, adding friction that other providers don't impose.
