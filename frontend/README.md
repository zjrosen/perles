# Perles Session Viewer

A React-based viewer for Perles orchestration session files.

## Quick Start

```bash
# Install dependencies
npm install

# Start the API server (in one terminal)
npm run server

# Start the frontend dev server (in another terminal)
npm run dev
```

Then open http://localhost:3000 and enter a session path like:
```
/Users/zack/.perles/sessions/perles/2026-01-28/058cd149-0eb1-4935-b98e-3bab36b520bd
```

## Features

- **Overview**: Session metadata, token usage, worker info, fabric activity stats
- **Fabric Events**: Channel creation, messages, replies, acks - with filtering
- **Coordinator**: View coordinator message log with tool calls highlighted
- **Workers**: Switch between workers and view their message logs
- **MCP Requests**: All MCP tool calls with decoded request/response JSON

## Architecture

- **Frontend**: React + TypeScript + Vite (port 3000)
- **Backend**: Express.js API server (port 3001)

The API server reads session files from disk and returns structured JSON.
The frontend proxies `/api` requests to the backend server.

## Session File Structure

```
session-dir/
├── metadata.json       # Session metadata (status, workers, tokens)
├── fabric.jsonl        # Fabric messaging events
├── mcp_requests.jsonl  # MCP tool calls (base64 encoded)
├── messages.jsonl      # Inter-agent messages
├── coordinator/
│   ├── messages.jsonl  # Coordinator conversation log
│   └── raw.jsonl       # Raw API responses
└── workers/
    ├── worker-1/
    │   ├── messages.jsonl
    │   └── raw.jsonl
    └── ...
```
