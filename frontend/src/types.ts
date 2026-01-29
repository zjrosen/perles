export interface SessionMetadata {
  session_id: string;
  start_time: string;
  status: string;
  session_dir: string;
  coordinator_session_ref: string;
  resumable: boolean;
  workers: WorkerMeta[];
  client_type: string;
  token_usage: {
    total_input_tokens: number;
    total_output_tokens: number;
    total_cost_usd: number;
  };
  application_name: string;
  work_dir: string;
  date_partition: string;
}

export interface WorkerMeta {
  id: string;
  spawned_at: string;
  headless_session_ref: string;
  work_dir: string;
}

export interface FabricEvent {
  version: number;
  timestamp: string;
  event: {
    type: string;
    timestamp: string;
    channel_id?: string;
    parent_id?: string;
    agent_id?: string;
    thread?: {
      id: string;
      type: string;
      created_at: string;
      created_by: string;
      content?: string;
      kind?: string;
      slug?: string;
      title?: string;
      purpose?: string;
      mentions?: string[];
      seq: number;
    };
    subscription?: {
      channel_id: string;
      agent_id: string;
      mode: string;
    };
    mentions?: string[];
  };
}

export interface McpRequest {
  timestamp: string;
  type: string;
  method: string;
  tool_name: string;
  request_json: string;
  response_json: string;
  duration: number;
  worker_id?: string;
}

export interface AgentMessage {
  role: string;
  content: string;
  is_tool_call?: boolean;
  ts: string;
}

export interface Command {
  command_id: string;
  command_type: string;
  source: string;
  success: boolean;
  duration_ms: number;
  timestamp: string;
  payload: Record<string, unknown>;
  result_data?: Record<string, unknown>;
}

export interface Session {
  path: string;
  metadata: SessionMetadata | null;
  fabric: FabricEvent[];
  mcpRequests: McpRequest[];
  commands: Command[];
  messages: unknown[];
  coordinator: {
    messages: AgentMessage[];
    raw: unknown[];
  };
  workers: {
    [key: string]: {
      messages: AgentMessage[];
      raw: unknown[];
      accountabilitySummary?: string;
    };
  };
  accountabilitySummary: string | null;
}
