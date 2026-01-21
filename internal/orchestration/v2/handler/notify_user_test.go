package handler_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/mocks"
	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/orchestration/v2/handler"
	"github.com/zjrosen/perles/internal/sound"
)

// ===========================================================================
// NotifyUserHandler Tests
// ===========================================================================

func TestNotifyUserHandler_Success(t *testing.T) {
	h := handler.NewNotifyUserHandler()

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Human review required: Please review the research findings",
		"clarification-review",
		"perles-abc.1",
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify result data
	notifyResult := result.Data.(*handler.NotifyUserResult)
	assert.Equal(t, "Human review required: Please review the research findings", notifyResult.Message)
	assert.Equal(t, "clarification-review", notifyResult.Phase)
	assert.Equal(t, "perles-abc.1", notifyResult.TaskID)

	// Verify event was emitted
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessUserNotification, event.Type)
	assert.Equal(t, events.RoleCoordinator, event.Role)
	assert.Equal(t, "coordinator", event.ProcessID)
	assert.Equal(t, "Human review required: Please review the research findings", event.Output)
	assert.Equal(t, "perles-abc.1", event.TaskID)
}

func TestNotifyUserHandler_MessageOnly(t *testing.T) {
	h := handler.NewNotifyUserHandler()

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Simple notification",
		"",
		"",
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)

	notifyResult := result.Data.(*handler.NotifyUserResult)
	assert.Equal(t, "Simple notification", notifyResult.Message)
	assert.Empty(t, notifyResult.Phase)
	assert.Empty(t, notifyResult.TaskID)

	// Verify event
	require.Len(t, result.Events, 1)
	event := result.Events[0].(events.ProcessEvent)
	assert.Equal(t, events.ProcessUserNotification, event.Type)
	assert.Empty(t, event.TaskID)
}

func TestNotifyUserHandler_EmptyMessage(t *testing.T) {
	h := handler.NewNotifyUserHandler()

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"", // Empty message should fail validation
		"some-phase",
		"",
	)

	result, err := h.Handle(context.Background(), cmd)

	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, err.Error(), "message is required")
}

func TestNotifyUserHandler_PlaysNotificationSound(t *testing.T) {
	soundService := mocks.NewMockSoundService(t)

	// Expect sound to be played
	soundService.EXPECT().Play("notification", "user_notification").Once()

	h := handler.NewNotifyUserHandler(
		handler.WithNotifyUserSoundService(soundService),
	)

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Review required",
		"phase-1",
		"",
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	assert.True(t, result.Success)
	// Sound service mock expectations are automatically verified on cleanup
}

func TestNotifyUserHandler_DefaultNoopSoundService(t *testing.T) {
	// Create handler WITHOUT sound service option - should use NoopSoundService
	h := handler.NewNotifyUserHandler()

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Using default sound service",
		"",
		"",
	)

	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic - NoopSoundService handles the Play call
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestWithNotifyUserSoundService_SetsService(t *testing.T) {
	soundService := mocks.NewMockSoundService(t)

	h := handler.NewNotifyUserHandler(
		handler.WithNotifyUserSoundService(soundService),
	)

	// Verify the handler was created with the sound service
	require.NotNil(t, h)
}

func TestWithNotifyUserSoundService_NilIgnored(t *testing.T) {
	h := handler.NewNotifyUserHandler(
		handler.WithNotifyUserSoundService(nil), // nil should be ignored
	)

	// Should still have NoopSoundService as the default
	require.NotNil(t, h)

	// Test that the handler works with a successful notification
	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Testing nil sound service",
		"",
		"",
	)

	result, err := h.Handle(context.Background(), cmd)

	// Should succeed without panic - NoopSoundService handles the Play call
	require.NoError(t, err)
	require.True(t, result.Success)
}

func TestNotifyUserHandler_EventContainsAllFields(t *testing.T) {
	h := handler.NewNotifyUserHandler()

	cmd := command.NewNotifyUserCommand(
		command.SourceMCPTool,
		"Check proposal document",
		"proposal-review",
		"task-123",
	)

	result, err := h.Handle(context.Background(), cmd)

	require.NoError(t, err)
	require.Len(t, result.Events, 1)

	event := result.Events[0].(events.ProcessEvent)

	// Verify all event fields are populated correctly
	assert.Equal(t, events.ProcessUserNotification, event.Type)
	assert.Equal(t, "coordinator", event.ProcessID)
	assert.Equal(t, events.RoleCoordinator, event.Role)
	assert.Equal(t, "Check proposal document", event.Output)
	assert.Equal(t, "task-123", event.TaskID)
}

// Ensure the sound package import is used to satisfy LSP
var _ = sound.NoopSoundService{}
