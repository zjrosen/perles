package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ===========================================================================
// NotifyUserCommand Tests
// ===========================================================================

func TestNotifyUserCommand_Validate(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		phase     string
		taskID    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "valid with all fields",
			message: "Please review the research findings",
			phase:   "clarification-review",
			taskID:  "perles-abc1",
			wantErr: false,
		},
		{
			name:    "valid with message only",
			message: "Human review required",
			phase:   "",
			taskID:  "",
			wantErr: false,
		},
		{
			name:    "valid with message and phase",
			message: "Checkpoint reached",
			phase:   "implementation-review",
			taskID:  "",
			wantErr: false,
		},
		{
			name:    "valid with message and taskID",
			message: "Task needs attention",
			phase:   "",
			taskID:  "perles-xyz.1",
			wantErr: false,
		},
		{
			name:      "empty message",
			message:   "",
			phase:     "some-phase",
			taskID:    "",
			wantErr:   true,
			errSubstr: "message is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewNotifyUserCommand(SourceMCPTool, tt.message, tt.phase, tt.taskID)
			err := cmd.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errSubstr != "" {
					require.Contains(t, err.Error(), tt.errSubstr)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestNotifyUserCommand_Type(t *testing.T) {
	cmd := NewNotifyUserCommand(SourceMCPTool, "message", "", "")
	require.Equal(t, CmdNotifyUser, cmd.Type())
}

func TestNotifyUserCommand_String(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		phase       string
		wantContain string
	}{
		{
			name:        "with phase",
			message:     "Review required",
			phase:       "clarification-review",
			wantContain: "phase=clarification-review",
		},
		{
			name:        "without phase",
			message:     "Review required",
			phase:       "",
			wantContain: "message=\"Review required\"",
		},
		{
			name:        "long message truncated",
			message:     "This is a very long message that should be truncated because it exceeds the maximum length allowed",
			phase:       "",
			wantContain: "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewNotifyUserCommand(SourceMCPTool, tt.message, tt.phase, "")
			str := cmd.String()
			require.Contains(t, str, tt.wantContain)
		})
	}
}

func TestNotifyUserCommand_ImplementsCommand(t *testing.T) {
	var _ Command = &NotifyUserCommand{}
}

func TestNewNotifyUserCommand(t *testing.T) {
	tests := []struct {
		name           string
		source         CommandSource
		message        string
		phase          string
		taskID         string
		expectedType   CommandType
		expectedSource CommandSource
	}{
		{
			name:           "from MCP tool with all fields",
			source:         SourceMCPTool,
			message:        "Human review required",
			phase:          "research-complete",
			taskID:         "perles-abc.1",
			expectedType:   CmdNotifyUser,
			expectedSource: SourceMCPTool,
		},
		{
			name:           "from internal with message only",
			source:         SourceInternal,
			message:        "Checkpoint reached",
			phase:          "",
			taskID:         "",
			expectedType:   CmdNotifyUser,
			expectedSource: SourceInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := NewNotifyUserCommand(tt.source, tt.message, tt.phase, tt.taskID)

			// Verify fields are set correctly
			require.Equal(t, tt.message, cmd.Message)
			require.Equal(t, tt.phase, cmd.Phase)
			require.Equal(t, tt.taskID, cmd.TaskID)

			// Verify BaseCommand fields
			require.Equal(t, tt.expectedType, cmd.Type())
			require.Equal(t, tt.expectedSource, cmd.Source())
			require.NotEmpty(t, cmd.ID())
			require.False(t, cmd.CreatedAt().IsZero())
		})
	}
}

func TestCmdNotifyUser_ConstantValue(t *testing.T) {
	require.Equal(t, CommandType("notify_user"), CmdNotifyUser)
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 8, "hello..."},
		{"empty", "", 5, ""},
		{"single char", "x", 1, "x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			require.Equal(t, tt.want, got)
		})
	}
}
