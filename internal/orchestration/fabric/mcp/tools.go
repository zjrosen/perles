// Package mcp provides MCP tool definitions for the Fabric messaging layer.
package mcp

// Tool defines an MCP tool that can be called.
// This is a local copy to avoid import cycles with orchestration/mcp.
// JSON field names match MCP protocol spec (camelCase).
type Tool struct {
	Name         string        `json:"name"`
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description"`
	InputSchema  *InputSchema  `json:"inputSchema"`            //nolint:tagliatelle // MCP protocol uses camelCase
	OutputSchema *OutputSchema `json:"outputSchema,omitempty"` //nolint:tagliatelle // MCP protocol uses camelCase
}

// OutputSchema defines the JSON Schema for tool output.
type OutputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
	Items      *PropertySchema            `json:"items,omitempty"`
}

// InputSchema defines the JSON Schema for tool input.
type InputSchema struct {
	Type       string                     `json:"type"`
	Properties map[string]*PropertySchema `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
}

// PropertySchema defines a single property in a schema.
type PropertySchema struct {
	Type        string                     `json:"type"`
	Description string                     `json:"description,omitempty"`
	Properties  map[string]*PropertySchema `json:"properties,omitempty"`
	Items       *PropertySchema            `json:"items,omitempty"`
	Required    []string                   `json:"required,omitempty"`
	Enum        []string                   `json:"enum,omitempty"`
}

// FabricTools returns the MCP tool definitions for Fabric messaging.
func FabricTools() []Tool {
	return []Tool{
		ToolFabricInbox,
		ToolFabricSend,
		ToolFabricReply,
		ToolFabricAck,
		ToolFabricSubscribe,
		ToolFabricUnsubscribe,
		ToolFabricAttach,
		ToolFabricHistory,
		ToolFabricReadThread,
	}
}

// ToolFabricInbox gets unacked messages for the current agent grouped by channel.
var ToolFabricInbox = Tool{
	Name:        "fabric_inbox",
	Description: "Get unread messages for the current agent. Returns messages grouped by channel with unacked counts. Use this to check what needs your attention.",
	InputSchema: &InputSchema{
		Type:       "object",
		Properties: map[string]*PropertySchema{},
		Required:   []string{},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channels": {
				Type:        "array",
				Description: "Channels with unread messages",
				Items: &PropertySchema{
					Type: "object",
					Properties: map[string]*PropertySchema{
						"channel_id":   {Type: "string", Description: "Channel ID"},
						"channel_slug": {Type: "string", Description: "Channel slug (e.g., 'tasks', 'general')"},
						"unacked":      {Type: "number", Description: "Number of unread messages"},
						"messages": {
							Type:        "array",
							Description: "Unread messages in this channel",
							Items: &PropertySchema{
								Type: "object",
								Properties: map[string]*PropertySchema{
									"id":         {Type: "string", Description: "Message ID"},
									"content":    {Type: "string", Description: "Message content"},
									"created_by": {Type: "string", Description: "Sender ID"},
									"created_at": {Type: "string", Description: "Timestamp"},
									"mentions":   {Type: "array", Description: "Mentioned agent IDs"},
								},
							},
						},
					},
				},
			},
			"total_unacked": {Type: "number", Description: "Total unread messages across all channels"},
		},
		Required: []string{"channels", "total_unacked"},
	},
}

// ToolFabricSend posts a new message to a channel.
var ToolFabricSend = Tool{
	Name:        "fabric_send",
	Description: "Send a new message to a channel. Use @mentions to notify specific agents (e.g., '@worker-1', '@coordinator').",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel": {
				Type:        "string",
				Description: "Channel slug: 'tasks', 'planning', 'general', 'system', or 'observer'",
				Enum:        []string{"tasks", "planning", "general", "system", "observer"},
			},
			"content": {
				Type:        "string",
				Description: "Message content. Include @mentions to notify agents.",
			},
			"kind": {
				Type:        "string",
				Description: "Message kind: 'info' (default), 'request', 'response', 'completion', 'error'",
				Enum:        []string{"info", "request", "response", "completion", "error"},
			},
		},
		Required: []string{"channel", "content"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"id":         {Type: "string", Description: "Created message ID"},
			"seq":        {Type: "number", Description: "Message sequence number"},
			"channel_id": {Type: "string", Description: "Channel ID"},
			"mentions":   {Type: "array", Description: "Extracted @mentions"},
		},
		Required: []string{"id", "seq", "channel_id"},
	},
}

// ToolFabricReply posts a reply to an existing message thread.
var ToolFabricReply = Tool{
	Name:        "fabric_reply",
	Description: "Reply to a message thread. Creates a threaded reply under the specified message. Replies inherit the channel of the parent message.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"message_id": {
				Type:        "string",
				Description: "ID of the message to reply to",
			},
			"content": {
				Type:        "string",
				Description: "Reply content. Include @mentions to notify agents.",
			},
			"kind": {
				Type:        "string",
				Description: "Message kind: 'response' (default), 'info', 'completion', 'error'",
				Enum:        []string{"info", "request", "response", "completion", "error"},
			},
		},
		Required: []string{"message_id", "content"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"id":              {Type: "string", Description: "Created reply ID"},
			"seq":             {Type: "number", Description: "Message sequence number"},
			"parent_id":       {Type: "string", Description: "Parent message ID"},
			"mentions":        {Type: "array", Description: "Extracted @mentions"},
			"thread_depth":    {Type: "number", Description: "Depth in thread (1 = direct reply)"},
			"thread_position": {Type: "number", Description: "Position in thread (1-indexed)"},
		},
		Required: []string{"id", "seq", "parent_id"},
	},
}

// ToolFabricAck marks messages as acknowledged (read).
var ToolFabricAck = Tool{
	Name:        "fabric_ack",
	Description: "Acknowledge messages to mark them as read. Acked messages won't appear in fabric_inbox.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"message_ids": {
				Type:        "array",
				Description: "Array of message IDs to acknowledge",
				Items:       &PropertySchema{Type: "string"},
			},
		},
		Required: []string{"message_ids"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"acked_count": {Type: "number", Description: "Number of messages acknowledged"},
		},
		Required: []string{"acked_count"},
	},
}

// ToolFabricSubscribe subscribes to a channel for notifications.
var ToolFabricSubscribe = Tool{
	Name:        "fabric_subscribe",
	Description: "Subscribe to a channel. Mode controls when you receive notifications: 'all' (every message), 'mentions' (only when @mentioned), 'none' (no notifications).",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel": {
				Type:        "string",
				Description: "Channel slug to subscribe to",
				Enum:        []string{"tasks", "planning", "general", "system", "observer"},
			},
			"mode": {
				Type:        "string",
				Description: "Notification mode: 'all' (default), 'mentions', 'none'",
				Enum:        []string{"all", "mentions", "none"},
			},
		},
		Required: []string{"channel"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel_id": {Type: "string", Description: "Subscribed channel ID"},
			"mode":       {Type: "string", Description: "Active notification mode"},
		},
		Required: []string{"channel_id", "mode"},
	},
}

// ToolFabricUnsubscribe removes a channel subscription.
var ToolFabricUnsubscribe = Tool{
	Name:        "fabric_unsubscribe",
	Description: "Unsubscribe from a channel to stop receiving notifications.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel": {
				Type:        "string",
				Description: "Channel slug to unsubscribe from",
				Enum:        []string{"tasks", "planning", "general", "system", "observer"},
			},
		},
		Required: []string{"channel"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"success": {Type: "boolean", Description: "Whether unsubscribe succeeded"},
		},
		Required: []string{"success"},
	},
}

// ToolFabricAttach attaches a file artifact to a message or channel.
var ToolFabricAttach = Tool{
	Name:        "fabric_attach",
	Description: "Attach a file (diff, log, code) to a message or channel. Provide the absolute file path - the file is referenced, not copied.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"target_id": {
				Type:        "string",
				Description: "ID of the message or channel to attach to",
			},
			"path": {
				Type:        "string",
				Description: "Absolute file path to attach (e.g., '/path/to/changes.diff')",
			},
			"name": {
				Type:        "string",
				Description: "Optional display name (defaults to filename)",
			},
		},
		Required: []string{"target_id", "path"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"id":         {Type: "string", Description: "Created artifact ID"},
			"name":       {Type: "string", Description: "Artifact display name"},
			"size_bytes": {Type: "number", Description: "File size in bytes"},
		},
		Required: []string{"id", "name", "size_bytes"},
	},
}

// ToolFabricHistory gets message history for a channel.
var ToolFabricHistory = Tool{
	Name:        "fabric_history",
	Description: "Get message history for a channel. Returns messages in chronological order.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel": {
				Type:        "string",
				Description: "Channel slug to get history for",
				Enum:        []string{"tasks", "planning", "general", "system", "observer"},
			},
			"limit": {
				Type:        "number",
				Description: "Maximum number of messages to return (default: 50)",
			},
			"include_acked": {
				Type:        "boolean",
				Description: "Include messages already acknowledged (default: true)",
			},
		},
		Required: []string{"channel"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"channel_id":   {Type: "string", Description: "Channel ID"},
			"channel_slug": {Type: "string", Description: "Channel slug"},
			"messages": {
				Type:        "array",
				Description: "Messages in chronological order",
				Items: &PropertySchema{
					Type: "object",
					Properties: map[string]*PropertySchema{
						"id":           {Type: "string", Description: "Message ID"},
						"seq":          {Type: "number", Description: "Sequence number"},
						"content":      {Type: "string", Description: "Message content"},
						"kind":         {Type: "string", Description: "Message kind"},
						"created_by":   {Type: "string", Description: "Sender ID"},
						"created_at":   {Type: "string", Description: "Timestamp"},
						"reply_count":  {Type: "number", Description: "Number of replies"},
						"is_acked":     {Type: "boolean", Description: "Whether message is acked by caller"},
						"mentions":     {Type: "array", Description: "Mentioned agent IDs"},
						"has_artifact": {Type: "boolean", Description: "Whether message has artifacts"},
					},
				},
			},
			"total_count": {Type: "number", Description: "Total messages in channel"},
		},
		Required: []string{"channel_id", "channel_slug", "messages"},
	},
}

// ToolFabricReadThread reads a message thread with all replies.
var ToolFabricReadThread = Tool{
	Name:        "fabric_read_thread",
	Description: "Read a message thread including all replies and artifacts. Use for detailed task discussion review.",
	InputSchema: &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"message_id": {
				Type:        "string",
				Description: "ID of the root message to read",
			},
			"include_artifacts": {
				Type:        "boolean",
				Description: "Include artifact metadata (default: true)",
			},
		},
		Required: []string{"message_id"},
	},
	OutputSchema: &OutputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"message": {
				Type:        "object",
				Description: "Root message",
				Properties: map[string]*PropertySchema{
					"id":         {Type: "string"},
					"content":    {Type: "string"},
					"kind":       {Type: "string"},
					"created_by": {Type: "string"},
					"created_at": {Type: "string"},
					"mentions":   {Type: "array"},
				},
			},
			"replies": {
				Type:        "array",
				Description: "Threaded replies in chronological order",
			},
			"artifacts": {
				Type:        "array",
				Description: "Artifacts attached to the message",
			},
			"participants": {
				Type:        "array",
				Description: "Unique agent IDs who participated in the thread",
			},
		},
		Required: []string{"message", "replies"},
	},
}
