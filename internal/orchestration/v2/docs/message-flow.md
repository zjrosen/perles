# Message Flow and Processing Pipeline

This document describes the end-to-end message flow in the v2 orchestration system, from MCP tool calls through command processing to UI updates.

## High-Level Flow

```mermaid
flowchart TB
    subgraph Input["Input Layer"]
        MCP["MCP Tool Call<br/>(JSON-RPC)"]
        User["TUI Input"]
        Callback["Process Callback"]
    end
    
    subgraph Adapter["Adapter Layer"]
        V2A["V2Adapter<br/>Parse & Route"]
    end
    
    subgraph Processing["Processing Layer"]
        CP["CommandProcessor<br/>FIFO Queue"]
        MW["Middleware Chain<br/>Log, Dedup, Timeout"]
        HR["Handler"]
    end
    
    subgraph State["State Layer"]
        PR["ProcessRepo"]
        TR["TaskRepo"]
        QR["QueueRepo"]
        MR["MessageRepo"]
    end
    
    subgraph Output["Output Layer"]
        EB["EventBus"]
        TUI["TUI Subscriber"]
    end
    
    MCP --> V2A
    User --> V2A
    Callback --> CP
    
    V2A -->|Submit| CP
    V2A -.->|Read-only| PR
    
    CP --> MW
    MW --> HR
    HR --> PR
    HR --> TR
    HR --> QR
    HR --> MR
    HR -->|Events| EB
    HR -->|FollowUp| CP
    
    EB --> TUI
```

## Command Processing Pipeline

### FIFO Processor Architecture

```mermaid
flowchart LR
    subgraph Queue["Command Queue (1000 capacity)"]
        Q1["Cmd 1"]
        Q2["Cmd 2"]
        Q3["Cmd 3"]
        QN["..."]
    end
    
    subgraph Processor["Single-Threaded Processor"]
        Dequeue["Dequeue"]
        Validate["Validate"]
        Route["Route to Handler"]
        Execute["Execute"]
        Emit["Emit Events"]
        FollowUp["Submit FollowUps"]
    end
    
    Queue --> Dequeue
    Dequeue --> Validate
    Validate --> Route
    Route --> Execute
    Execute --> Emit
    Execute --> FollowUp
    FollowUp -.->|back to queue| Queue
```

### Execution Pipeline Detail

```mermaid
sequenceDiagram
    participant Client
    participant Queue as Command Queue
    participant Proc as Processor Loop
    participant MW as Middleware
    participant Handler
    participant Repo as Repositories
    participant EB as EventBus
    
    Client->>Queue: Submit(cmd)
    Note over Queue: FIFO ordering preserved
    
    Queue->>Proc: Dequeue cmd
    Proc->>Proc: cmd.Validate()
    alt Validation fails
        Proc->>EB: Emit CommandErrorEvent
        Proc->>Client: Return error
    end
    
    Proc->>MW: ChainMiddleware(handler)
    MW->>MW: Logging before
    MW->>MW: Dedup check
    MW->>Handler: Handle(ctx, cmd)
    
    Handler->>Repo: Read/Write state
    Handler-->>Proc: CommandResult
    
    Proc->>EB: Publish(result.Events)
    
    loop For each FollowUp
        Proc->>Queue: Submit(followUp)
    end
    
    MW->>MW: Logging after
    Proc->>Client: Return result
```

## Read vs Write Path (CQRS)

The v2 system separates read and write operations for performance:

```mermaid
flowchart TB
    subgraph Reads["Read Path (Fast)"]
        R1["HandleQueryWorkerState"]
        R2["HandleReadMessageLog"]
    end
    
    subgraph Writes["Write Path (Ordered)"]
        W1["HandleSpawnProcess"]
        W2["HandleAssignTask"]
        W3["HandleSendToWorker"]
    end
    
    subgraph Repos["Repositories"]
        PR["ProcessRepo"]
        TR["TaskRepo"]
        MR["MessageRepo"]
    end
    
    subgraph CP["CommandProcessor"]
        Queue["FIFO Queue"]
        Handler["Handlers"]
    end
    
    Reads -->|Direct| Repos
    Writes -->|Submit| CP
    CP --> Handler
    Handler --> Repos
```

**Rationale:**
- Reads don't mutate state → no ordering needed
- Writes must be serialized → FIFO queue guarantees consistency
- Read latency is critical for UI responsiveness

## Message Queue Pattern

The system uses a **queue-or-deliver** pattern for worker messages:

```mermaid
flowchart TD
    Send["SendToProcess"]
    
    Send --> GetStatus{Worker Status?}
    
    GetStatus -->|Working| Enqueue["1. Enqueue message"]
    Enqueue --> EmitQueue["2. Emit QueueChanged"]
    
    GetStatus -->|Ready| EnqueueReady["1. Enqueue message"]
    EnqueueReady --> CreateFollowUp["2. Create DeliverProcessQueued"]
    CreateFollowUp --> Return["3. Return FollowUp"]
    
    GetStatus -->|Other| Error["Return error"]
```

### Queue Drain on Turn Complete

```mermaid
sequenceDiagram
    participant AI as AI Process
    participant Proc as Process
    participant CP as CommandProcessor
    participant TE as TurnEnforcer
    participant Queue as MessageQueue
    participant EB as EventBus
    
    AI->>Proc: Turn Complete
    Proc->>CP: ProcessTurnComplete
    
    alt Worker missing required tool call
        CP->>TE: CheckTurnCompletion
        TE-->>CP: Missing tools (retries < 2)
        CP->>Queue: Enqueue reminder (SenderSystem)
        CP->>CP: Return DeliverProcessQueued
        Note over CP: Retry count preserved
    else Compliant or max retries exceeded
        CP->>CP: Working → Ready
        CP->>Queue: Check queue
        
        alt Queue not empty
            CP->>CP: Return DeliverProcessQueued
            CP->>Queue: Dequeue message
            Queue-->>CP: Message content
            CP->>AI: Deliver message
            CP->>CP: Ready → Working
        else Queue empty
            CP->>EB: Emit ProcessReady
        end
    end
```

## Event Emission and Subscription

### Event Types

```mermaid
flowchart LR
    subgraph Sources["Event Sources"]
        H["Handlers"]
        P["Process Event Loop"]
        E["Error Handler"]
    end
    
    subgraph Events["Event Types"]
        PS["ProcessSpawned"]
        PO["ProcessOutput"]
        PSC["ProcessStatusChange"]
        PTU["ProcessTokenUsage"]
        PE["ProcessError"]
        PQC["ProcessQueueChanged"]
        PR["ProcessReady"]
        PW["ProcessWorking"]
    end
    
    subgraph Bus["EventBus"]
        EB["pubsub.Broker[any]"]
    end
    
    subgraph Subs["Subscribers"]
        TUI["TUI"]
        LOG["Logger"]
        MET["Metrics"]
    end
    
    Sources --> Events
    Events --> EB
    EB --> Subs
```

### Pub/Sub Pattern

```go
// Subscribe with context for auto-cleanup
eventCh := eventBus.Subscribe(ctx)

// Non-blocking publish
eventBus.Publish(pubsub.UpdatedEvent, processEvent)

// Receive in goroutine
for evt := range eventCh {
    switch e := evt.Payload.(type) {
    case events.ProcessEvent:
        // Handle process event
    case processor.CommandErrorEvent:
        // Handle command error
    }
}
```

### ContinuousListener (Bubble Tea)

For TUI integration, use `ContinuousListener` to maintain subscription across the update loop:

```go
type Model struct {
    listener *pubsub.ContinuousListener[any]
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
    switch msg := msg.(type) {
    case pubsub.Event[any]:
        // Handle event
        return m, m.listener.Listen()  // Always continue listening!
    }
    return m, nil
}
```

## Complete Example: Task Assignment Flow

```mermaid
sequenceDiagram
    participant Coord as Coordinator AI
    participant MCP as MCP Server
    participant V2A as V2Adapter
    participant CP as CommandProcessor
    participant ATH as AssignTaskHandler
    participant PR as ProcessRepo
    participant TR as TaskRepo
    participant QR as QueueRepo
    participant BD as BDExecutor
    participant EB as EventBus
    participant TUI as TUI
    participant W1 as Worker AI
    
    Coord->>MCP: assign_task(worker_id, task_id)
    MCP->>V2A: HandleAssignTask(args)
    V2A->>V2A: Parse JSON, validate
    V2A->>CP: Submit(AssignTaskCommand)
    
    CP->>ATH: Handle(ctx, cmd)
    
    ATH->>PR: Get(workerID)
    PR-->>ATH: Worker (Ready, Idle)
    
    ATH->>ATH: Validate preconditions
    ATH->>PR: Save(worker with Implementing phase)
    ATH->>TR: Save(TaskAssignment)
    ATH->>QR: Enqueue(task prompt)
    ATH->>BD: UpdateTaskStatus("in_progress")
    
    ATH-->>CP: Result{Events, FollowUp: DeliverQueued}
    
    CP->>EB: Publish(ProcessPhaseChange)
    EB->>TUI: ProcessEvent
    TUI->>TUI: Update UI
    
    CP->>CP: Submit(DeliverProcessQueued)
    CP->>QR: Dequeue
    CP->>W1: Deliver task prompt
    
    CP->>EB: Publish(ProcessWorking)
    EB->>TUI: ProcessEvent
    TUI->>TUI: Show working status
```

## Error Handling Flow

```mermaid
flowchart TD
    subgraph Input["Command Input"]
        Cmd["Command"]
    end
    
    subgraph Validation["Validation Layer"]
        Val{Validate?}
    end
    
    subgraph Routing["Handler Routing"]
        Route{Handler Found?}
    end
    
    subgraph Execution["Handler Execution"]
        Exec{Execute OK?}
    end
    
    subgraph Errors["Error Handling"]
        ValErr["ValidationError"]
        RouteErr["ErrUnknownCommandType"]
        ExecErr["Handler Error"]
        ErrEvt["CommandErrorEvent"]
    end
    
    subgraph Output["Output"]
        Success["CommandResult{Success: true}"]
        Failure["CommandResult{Success: false}"]
    end
    
    Cmd --> Val
    Val -->|Yes| Route
    Val -->|No| ValErr
    
    Route -->|Yes| Exec
    Route -->|No| RouteErr
    
    Exec -->|Yes| Success
    Exec -->|No| ExecErr
    
    ValErr --> ErrEvt
    RouteErr --> ErrEvt
    ExecErr --> ErrEvt
    
    ErrEvt --> Failure
```

## Middleware Chain

```mermaid
flowchart TB
    subgraph Chain["Middleware Chain (outer to inner)"]
        L["LoggingMiddleware<br/>Logs command execution"]
        D["DeduplicationMiddleware<br/>Prevents duplicate processing"]
        T["TimeoutMiddleware<br/>Warns on slow handlers"]
        H["Handler<br/>Business logic"]
    end
    
    Request --> L
    L --> D
    D --> T
    T --> H
    H --> T
    T --> D
    D --> L
    L --> Response
```

### Deduplication

```go
// SHA256 hash of command content (excludes ID, timestamp)
func computeContentHash(cmd Command) string {
    // Hash type + type-specific fields
    // Commands can implement ContentHash() for custom logic
}

// Cache with TTL (default 5s)
type DeduplicationMiddleware struct {
    cache sync.Map       // contentHash → expiry
    ttl   time.Duration  // 5 seconds
}
```

## Message Repository Integration

The MessageRepository provides an inter-agent communication log:

```mermaid
flowchart TB
    subgraph Agents["Agents"]
        Coord["Coordinator"]
        W1["Worker 1"]
        W2["Worker 2"]
    end
    
    subgraph MR["MessageRepository"]
        Log["Message Log<br/>(append-only)"]
        ReadTrack["Read Tracking<br/>(per agent)"]
        Broker["Pub/Sub Broker"]
    end
    
    subgraph Broadcast["Broadcast Semantics"]
        Note["All agents see all messages<br/>To field is metadata only"]
    end
    
    Coord -->|Append| Log
    W1 -->|Append| Log
    W2 -->|Append| Log
    
    Log -->|Entries| Coord
    Log -->|Entries| W1
    Log -->|Entries| W2
    
    Log -->|Publish| Broker
    Broker -->|Subscribe| TUI
```

### Message Entry

```go
type Message struct {
    ID        string
    Timestamp time.Time
    From      string      // COORDINATOR, WORKER.1, USER
    To        string      // ALL, COORDINATOR, WORKER.2
    Content   string
    Type      MessageType // info, request, response, completion, error
    ReadBy    []string    // Track which agents have read
}
```
