# Perles Session Viewer

A React-based viewer for Perles orchestration session files.

## Quick Start

The frontend is served by the Perles Go binary. To develop:

```bash
# 1. From the repo root, start a backend on Vite's default proxy port (19999)
make daemon            # builds and runs ./perles daemon -p 19999
# Or run the TUI from a beads project: perles -p 19999, then press ctrl+o to
# open the dashboard (the API server starts when you enter the dashboard)

# 2. In another terminal, from frontend/
npm install
npm run dev            # http://localhost:3000, proxies /api to localhost:19999
```

Either way, perles needs a working beads or beads_rust project (see the
[README requirements](../README.md#requirements)). The daemon uses
`BEADS_DIR` if set, then `beads_dir` from the config, then the `.beads/`
directory where it runs (the repo root for `make daemon`).

If the backend runs on a different port, start the dev server with
`VITE_API_PORT=<port> npm run dev`. The daemon prints
`Perles daemon started on port <port>`, and the dashboard shows the port in
its title (`Workflows · API ::<port>`). Open http://localhost:3000 and pick a
session from the list.

For production, the frontend is embedded in the Go binary and served
automatically when you run Perles in dashboard mode (or `perles daemon`).
In the dashboard, press `o` on a workflow to open its session in your browser.

## Building for the Go Binary

The Go binary embeds `frontend/dist/`. Run `npm run build` here (or
`make build-frontend` / `make build` / `make run-all` from the repo root) to
regenerate it. `make build-go`, `make run` and `make debug` reuse the
committed `dist/` and will not pick up frontend changes. `dist/` is committed
so the Go build (including `go install`) works without Node.js.

## Testing

- `npm run test:run` runs the Vitest unit tests (`npm test` for watch mode).
- `npm run test:e2e` runs the Playwright tests (run `npx playwright install`
  once first; `npm run test:e2e:ui` opens the Playwright UI).

CI only type-checks and builds the frontend (via `make build`); it does not
run these tests. Use the Node version in the repo-root `.nvmrc`.

## Features

- **Overview**: Session metadata, token usage, worker info, fabric activity stats
- **Fabric**: Channel creation, messages, replies, acks - with filtering;
  post messages and replies (with @mention autocomplete), view attached
  artifact files, and add reactions (running or paused workflows only)
- **Commands**: Command processor log (`commands.jsonl`)
- **Coordinator**: View coordinator message log with tool calls highlighted
- **Workers**: Switch between workers and view their message logs
- **Observer**: Observer messages and notes (shown only when an observer ran)
- **MCP Requests**: All MCP tool calls with decoded request/response JSON

## Architecture

- **Frontend**: React + TypeScript + Vite
- **Backend**: Go API server (embedded in Perles binary)

The Go server reads session files from disk and returns structured JSON. It
lists and loads sessions only under `~/.perles/sessions` (it does not read a
custom `orchestration.session_storage.base_dir`). Fabric posts go to running
or paused workflows through the control plane; for other sessions they are
appended to the session's `fabric.jsonl`.
In development, Vite proxies `/api` requests to the Go backend.

## Session File Structure

```
session-dir/
├── metadata.json       # Session metadata (status, workers, tokens)
├── fabric.jsonl        # Fabric messaging events
├── mcp_requests.jsonl  # MCP tool calls (base64 encoded)
├── messages.jsonl      # Inter-agent messages
├── commands.jsonl      # Command processor log
├── summary.md          # Post-session summary
├── coordinator/
│   ├── messages.jsonl  # Coordinator conversation log
│   └── raw.jsonl       # Raw API responses
├── observer/
│   ├── messages.jsonl  # Observer conversation log
│   └── observer_notes.md
└── workers/
    ├── worker-1/
    │   ├── messages.jsonl
    │   ├── raw.jsonl
    │   └── accountability_summary.md
    └── ...
```
