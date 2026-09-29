# Orchestration Control Plane

> **WARNING** Orchestration mode is not cheap it spawns multiple headless AI agents to plan, investigate, and execute tasks.
> If you care about money **DO NOT** use orchestration mode. Every headless agent runs with **FULL PERMISSIONS**.

Orchestration mode is a multi-agent control plane workspace that can launch multiple workflows in parallel where a single coordinator agent handles spawning, replacing and retiring other headless agents through
built-in MCP tools. The coordinator agent delegates sub-tasks to multiple worker agents so you don't have to manually stop and start sessions on your own. 
This allows for structured workflow instructions that can manage and orchestrate multiple headless AI agents.

- **Coordinator** A single headless agent that receives workflow instructions and manages the session.
- **Workers** Multiple headless agents who execute specific sub-tasks (coding, testing, reviewing, documenting)

<p align="center">
  <img src="./docs/assets/control-plane.png" width="1440" alt="search">
</p>

---

## Getting Started

### Configuration

The default orchestration settings use Claude Code with Opus 5.5 (`claude-opus-5-5`) and by far work the best. 
You can customize these settings in your config file, but they are optional. Perles uses the file given with `--config`/`-c`,
otherwise `.perles/config.yaml` in the current directory, otherwise `~/.config/perles/config.yaml`. The first time you run
`perles` in a beads project with no config file anywhere, it writes the default config to `.perles/config.yaml` (you can also create it with
`perles init`), and that local file then takes precedence over `~/.config/perles/config.yaml`.

```yaml
orchestration:
  coordinator_client: "claude"  # Options: claude, amp, codex, gemini, opencode, cursor
  worker_client: "claude"       # Options: claude, amp, codex, gemini, opencode, cursor
  
  # Provider-specific settings
  claude:
    model: "claude-opus-5-5"    # Default: claude-opus-5-5; aliases: opus, sonnet, haiku
  amp:
    model: "opus"               # Options: opus, sonnet
    mode: "smart"               # Options: free, rush, smart
  codex:
    model: "gpt-6-sol"          # Default: gpt-6-sol; any Codex model ID is passed through (e.g. gpt-6-luna)
  gemini:
    model: "gemini-3.8-flash"   # Default: gemini-3.8-flash
  opencode:
    model: "anthropic/claude-opus-5-5"  # Default: anthropic/claude-opus-5-5
  # cursor:
  #   model: "composer-1"       # Uses Cursor's default model if empty
```

If `coordinator_client` or `worker_client` is omitted, that role falls back to the legacy `orchestration.client` key, then to `claude`.

### Quick Start

1. Open Perles in your project directory.
2. Press `ctrl+o` in kanban mode to open the orchestration dashboard (configurable via `ui.keybindings.dashboard`).
3. Press `n` to open the New Workflow dialog, then select a workflow template and fill in any required fields.
   - A typical coding workflow is done over multiple workflows. You would start with the  **Research Proposal** to generate a proposal document. Then launch a **Research to Tasks** to break down the proposal document into beads epics and tasks. Then launch the **Cook** workflow with that epic to work through the entire epic's tasks.

---

Any workflow supports doing work inside a git worktree. When using a worktree you can specify a different base branch to create the worktree from along with an optional branch name. If you are just doing research or converting a research proposal into an epic and tasks you likely do not have to use a worktree since you are not changing any code. 
Worktree's are primarily meant for when you are running multiple "Cook" workflows on different epics in parallel.
 
<p align="center">
  <img src="./docs/assets/new-workflow.png" width="1440" alt="search">
</p>

---

## Layout

### Workflows Pane

Every workflow launched shows as a new workflow in the table which shows the status, epic id, working directory and last heartbeat status.

A workflow can be paused by pressing "x" which will stop all the running processes for the selected workflow and can be resumed with "s". 
New workflows are started by pressing "n" when the workflows table is in focus.

### Coordinator Pane

The headless AI agent process that plans and delegates work to the workers based on workflow instructions. You can communicate to the coordinator using the chat input.
In the workflows table, press `enter` on a workflow to focus its chat input, or `ctrl+w` to hide or show the pane.

The pane is split into tabs: `Coord` (coordinator), `Obs` (observer, only when `orchestration.observer_enabled` is true), `Msgs` (message log),
`CmdLog` (command log, only in debug mode with `perles -d`), and one tab per worker (`W1`, `W2`, ...). Use `ctrl+j`/`ctrl+k` to cycle tabs.

**What you see:**
- Status indicator showing coordinator state
- Token usage metrics (context consumption)
- Queue count when messages are pending
- Full conversation history

**Status indicators:**
| Icon | Meaning |
|------|---------|
| `●` (blue) | Working — actively processing |
| `○` (green) | Ready — waiting for input |
| `⏸` | Paused — workflow paused |
| `⚠` (yellow) | Stopped — needs attention |
| `✗` (red) | Retired or failed |

#### Message Log (`Msgs` Tab)

The timeline of all inter-agent communication. Workers post to the message log when they finish their turns which nudges the coordinator to read and act.
Workers are automatically enforced to end their turn with an MCP tool call to post their message to the log, if they do not use a tool call the system will
intercept and remind them to do so.

**What you see:**
- A header for each message with its time, channel and sender (e.g., `14:02 [#tasks] worker-1`)
- The message content (thread replies are prefixed with `↳ reply:`)

#### Worker Panes (Tabs)

When workers are spawned they are shown as tabs in the coordinator pane which you can view the output of each individual AI agent process.

**What you see for each worker:**
- Tab label with a status indicator (same icons as coordinator) and short worker ID (e.g., `● W1`)
- Context token usage (e.g., `45k/200k`)
- Queue count when messages are pending (e.g., `[2 queued]`)
- Output content

#### Chat Input Bar

Text input area for sending messages to the coordinator or to a fabric channel.

**Visual feedback:**
- The bottom-right of the input shows your current message target: `DM: Coordinator`, `#general`, `#tasks`, `#planning`,
  and `#observer` when the observer is enabled. Press `Tab` while the coordinator pane is focused to cycle targets
  (use `ctrl+n` or `shift+tab` to move focus out of the pane).
- In a channel, type `@` to mention a participant: `@worker-N` or `@coordinator` notifies that agent, and `@here` notifies every agent that has joined the fabric.
- After you post in a channel, your next messages there reply in that thread, and the top-right shows the thread ID (e.g., `↩ a1b2c3`).
  Press `ctrl+t` to pick a different thread, or `esc` with an empty input to start a new one.
- When vim_mode is enabled the bottom-left shows which vim mode you are in for the text input.

### Epic Tree and Details

Every workflow is powered by a backing beads epic, this allows you to see progress being made of a workflow and view the details of each task of the epic. 

The "Cook" workflow is the only special workflow which uses an existing epic for its work versus the other workflows which create an epic on their own.

---

## Workflow Templates

Workflow templates are pre-defined "recipes" for common orchestration patterns they are DAGs configured via yaml files which are
then converted into a beads epic and tasks.

### Built-in Templates

| Template | Description |
|----------|-------------|
| **Cook** | Sequential task execution with code review |
| **Research to Tasks** | Convert an existing research/proposal document into a beads epic and tasks with multi-perspective review |
| **Technical Debate** | Structured multi-perspective debate (moderator, affirmative, negative, neutral analyst) |
| **Mediated Investigation** | Structured investigation with mediator |
| **Research Proposal** | Collaborative proposal development |
| **Quick Plan** | Rapid planning and task breakdown |

### Community Templates

Extra community workflows ship with perles (currently `joke-contest`) but are disabled by default. Enable them by ID in your config
and restart perles; they then appear in the new workflow picker:

```yaml
orchestration:
  community_workflows:
    - "joke-contest"   # "workflow/joke-contest" also works
```

A user template with the same `key` overrides a community or built-in template.

Run `perles workflows` to check what's loaded: its `Dashboard Workflows` section lists every template the new workflow picker will show,
labeled `[built-in]`, `[community]` or `[user]`.

### Creating Custom Templates

Create your own templates in `~/.perles/workflows/{your_workflow_name}` and they will be loaded into the workflow picker automatically the next time perles starts.

Templates consist of a `template.yaml` file that specifies the DAG for the epic and its tasks and custom arguments that can be used in templates. 
And individual task markdown files that are referenced in the yaml file.

A template given as a bare filename is looked up in `~/.perles/workflows/{your_workflow_name}/`, then `~/.perles/workflows/`,
then perles' built-in shared templates (`v1-epic-instructions.md`, the default coordinator prompt, and `v1-human-review.md`, used by
human review nodes), so you don't need to copy the shared templates. A file of the same name in your directories overrides the built-in copy.
Paths containing `/` are relative to `~/.perles/` (e.g. `workflows/v1-human-review.md`), falling back to the same path in the built-in templates.

Every referenced markdown file must exist in one of those places. An invalid workflow (missing template, bad YAML, invalid assignee, etc.) is skipped
and your other workflows still load; perles writes a `skipping invalid workflow` warning with the error to the debug log (run `perles -d` and check `debug.log`).

#### Template YAML Fields

**Registration Fields (Top-Level)**

| Field | Type | Required | Description                                                                                                                                                        |
|-------|------|----------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `namespace` | string | Yes      | Always `"workflow"` for workflow templates                                                                                                                         |
| `key` | string | Yes      | Unique identifier for the workflow (e.g., `"joke-contest"`)                                                                                                        |
| `version` | string | Yes      | Version identifier (e.g., `"v1"`)                                                                                                                                  |
| `name` | string | Yes      | Human-readable name shown in the workflow picker                                                                                                                   |
| `description` | string | Yes      | Description of what the workflow does                                                                                                                              |
| `epic_template` | string | No       | Filename of the markdown template for the epic description (omit for epic-driven workflows like Cook)                                                              |
| `system_prompt` | string | No       | Leave this empty most of the time: the coordinator then uses the built-in `v1-epic-instructions.md` prompt and takes its instructions from the epic_template. Set it only to override the coordinator's system prompt; the file must exist. |
| `path` | string | No       | Path prefix for artifact inputs/outputs (example: `".spec"`)                                                                                                       |
| `labels` | list | No       | Tags for filtering (e.g., `["category:meta", "lang:go"]`)                                                                                                          |
| `arguments` | list | No       | User-configurable parameters (see Arguments table)                                                                                                                 |
| `nodes` | list | Conditional | DAG of workflow tasks (see Nodes table). Required unless the workflow is epic-driven (a single `epic_id` argument, like Cook) or sets `system_prompt` |

**Argument Fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Unique identifier, accessed in templates as `{{.Args.key}}` |
| `label` | string | Yes | Human-readable label for the form field |
| `description` | string | No | Help text/placeholder for the form field |
| `type` | string | Yes | Input type: `text`, `number`, `textarea`, `select`, `multi-select`, or `epic-search` (searchable epic picker) |
| `required` | bool | No | Whether the argument must be filled (default: `false`) |
| `default` | string | No | Default value for the field |
| `options` | list | Conditional | Required for `select` and `multi-select` types |

**Node Fields (DAG Tasks)**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Unique identifier for the node within the workflow |
| `name` | string | Yes | Display name for the task |
| `template` | string | Yes | Filename of the markdown template for this task |
| `assignee` | string | No | Worker role (e.g., `"worker-1"`, `"human"` for human review gates) |
| `after` | list | No | Node keys this node depends on (runs after these complete) |
| `inputs` | list | No | Artifacts consumed by this node (see Artifacts table) |
| `outputs` | list | No | Artifacts produced by this node (see Artifacts table) |

**Artifact Fields (Inputs/Outputs)**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Stable identifier for template access (e.g., `{{.Inputs.plan}}`) |
| `file` | string | Yes | Filename, may contain template syntax (e.g., `"{{.Date}}-report.md"`) |

`~/.perles/workflows/{your_workflow_name}/template.yaml`
```yaml
registry:
  - namespace: "workflow"
    key: "joke-contest"
    version: "v1"
    name: "Joke Contest"
    description: "Test workflow where two workers write jokes in parallel, then a third worker judges and picks a winner"
    epic_template: "v1-joke-contest-epic.md"
    # Optional system prompt override most of the time you should leave this omitted.
    # If you set it, the file must exist.
    # system_prompt: "my-coordinator-prompt.md"
    # Optional prefix path to the inputs / outputs
    path: ""
    labels:
      - "category:meta"

    # ═══════════════════════════════════════════════════════════════════════
    # ARGUMENTS: User-configurable parameters
    # ═══════════════════════════════════════════════════════════════════════
    arguments:
      - key: "theme"
        label: "Joke Theme"
        description: "Optional theme or topic for the jokes (e.g., programming, animals, food)"
        type: "text"
        required: false

    # ═══════════════════════════════════════════════════════════════════════
    # NODES: Workflow tasks (DAG structure)
    # ═══════════════════════════════════════════════════════════════════════
    # Phase 1: Two jokers write jokes in parallel (no dependencies)
    # Phase 2: Judge reads both jokes and picks a winner
    # ═══════════════════════════════════════════════════════════════════════
    nodes:
      # Phase 1: Parallel joke writing
      - key: "joke-1"
        name: "Joker 1 - Write Joke"
        template: "v1-joke-contest-joke-1.md"
        assignee: "worker-1"
        # No after - starts immediately

      - key: "joke-2"
        name: "Joker 2 - Write Joke"
        template: "v1-joke-contest-joke-2.md"
        assignee: "worker-2"
        # No after - runs in parallel with joke-1

      # Phase 2: Human review gate
      - key: "review"
        name: "Human Review"
        template: "v1-human-review.md"  # Built-in shared template, no need to copy it
        assignee: "human"
        after:
          - "joke-1"
          - "joke-2"

      # Phase 3: Judge picks winner
      - key: "judge"
        name: "Judge - Pick Winner"
        template: "v1-joke-contest-judge.md"
        assignee: "worker-3"
        after:
          - "review"
```

Example markdown file referencing arguments from the `template.yaml`
`~/.perles/workflows/{your_workflow_name}/v1-joke-contest-epic.md`
```markdown
# Joke Contest: {{.Name}}

You are the **Coordinator** for a joke contest workflow. Your job is to orchestrate 3 workers through a fun competition. You do not wait for the user you start immediately you are operating in headless mode.

## Context

{{- if .Args.theme}}
- **Theme:** {{.Args.theme}}
{{- else}}
- **Theme:** Any topic (no theme specified)
{{- end}}

## Your Workers

| Worker | Role | Responsibilities | Phase |
|--------|------|------------------|-------|
| worker-1 | Joker 1 | Write a joke and add it as a comment to their task | 1 |
| worker-2 | Joker 2 | Write a joke and add it as a comment to their task | 1 |
| human | Reviewer | Review jokes before judging (optional gate) | 2 |
| worker-3 | Judge | Read both jokes from task comments, pick a winner | 3 |

**NOTE:** You (the Coordinator) are NOT a worker. Start executing immediately.

## Quality Standards

- Jokes should be original and appropriate
- The judge should provide reasoning for their choice
- All jokes must be added as bd task comments (not just mentioned in chat)

## Success Criteria

A successful joke contest should have:
- Two jokes submitted as task comments (one from each joker)
- A clear winner announced by the judge
- The winning joke quoted in the judge's comment
```

---

### Slash Commands

Slash commands let you control processes directly to spawn, stop, retire, or replace them. You generally do not have to use these
but if a worker does get stuck for any reason you can stop them directly using slash commands. You can ask the coordinator
to do this as well. Type them in the chat input; any other `/...` text is sent to the coordinator as a normal message.

| Command                           | Action |
|-----------------------------------|--------|
| `/stop <process-id> [--force]`    | Stop a worker or the coordinator. The process is stopped, not retired, so it can be resumed. It is asked to exit and killed if it hasn't exited after 5 seconds; `--force` kills it immediately (a worker that is committing is only stopped with `--force`) |
| `/spawn`                          | Spawn a new worker |
| `/retire <worker-id> [reason]`    | Gracefully retire a worker (the coordinator cannot be retired) |
| `/replace <process-id> [reason]`  | Replace a worker, the coordinator (`/replace coordinator`) or the observer with a fresh process |

---

## Session Management

### Sound Configuration

Perles plays audio feedback for various orchestration events. All sounds are enabled by default, including events you leave out of
your config. Set `enabled: false` on an event to silence it, or use `override_sounds` to replace the built-in sound.

**Available Sound Events**

| Event | Description                                        |
|-------|----------------------------------------------------|
| `review_verdict_approve` | Plays when a review is approved in a cook workflow |
| `review_verdict_deny` | Plays when a review is denied in a cook workflow   |
| `user_notification` | Plays when the coordinator needs your attention. There is no built-in sound for this event, so it is silent unless you set `override_sounds` |
| `worker_out_of_context` | Plays when a worker runs out of context            |
| `coordinator_out_of_context` | Plays when the coordinator runs out of context |
| `observer_out_of_context` | Plays when the observer runs out of context |
| `workflow_complete` | Plays when a workflow completes                    |

**Configuration Example**

```yaml
sound:
  events:
    review_verdict_approve:
      enabled: true
      override_sounds:
        - "~/.perles/sounds/checkpoint.wav"
        - "~/.perles/sounds/proceed.wav"  # Multiple sounds = random selection
    review_verdict_deny:
      enabled: true
      override_sounds:
        - "~/.perles/sounds/denied.wav"
    workflow_complete:
      enabled: true
      override_sounds:
        - "~/.perles/sounds/complete.wav"
```

**Event Configuration Fields**

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Whether to play sounds for this event. An event you list without `enabled: true` is silenced, so set it even when you only want `override_sounds` |
| `override_sounds` | list | Custom sound file paths (WAV format). If multiple are provided, one is randomly selected |

Override sound paths may start with `~/`. Each file must be a `.wav` file, must exist, must be inside `~/.perles/sounds/`
(after following symlinks), and must be 1MB or smaller. Otherwise perles exits at startup with an `invalid sound configuration` error.
If an override file is removed while perles is running, the event's built-in sound plays instead.

---

### Session Storage

Every orchestration session data is stored centrally in your home directory, by default in `~/.perles/sessions/`

```
~/.perles/sessions/
├── sessions.json                    # Global session index
└── {application-name}/
    ├── sessions.json                # Per-application index
    └── 2026-01-12/
        └── {session-uuid}/
            ├── metadata.json
            ├── coordinator/
            ├── workers/
            └── messages.jsonl
```

`{application-name}` is `orchestration.session_storage.application_name` if set, otherwise the repository name from the git `origin` remote,
falling back to the working directory name (`perles daemon` skips the git lookup and uses the directory name). You can change the
location and the application name in config:

```yaml
orchestration:
  session_storage:
    base_dir: ~/.perles/sessions      # Must be absolute or start with ~/
    application_name: my-project      # Overrides {application-name}
```
