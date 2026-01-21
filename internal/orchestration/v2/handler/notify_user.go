// Package handler provides command handlers for the v2 orchestration architecture.
// This file contains the handler for user notification commands.
package handler

import (
	"context"
	"fmt"

	"github.com/zjrosen/perles/internal/orchestration/events"
	"github.com/zjrosen/perles/internal/orchestration/v2/command"
	"github.com/zjrosen/perles/internal/sound"
)

// ===========================================================================
// NotifyUserHandler
// ===========================================================================

// NotifyUserHandler handles CmdNotifyUser commands.
// It plays a notification sound and emits a ProcessUserNotification event.
type NotifyUserHandler struct {
	soundService sound.SoundService
}

// NotifyUserHandlerOption configures NotifyUserHandler.
type NotifyUserHandlerOption func(*NotifyUserHandler)

// WithNotifyUserSoundService sets the sound service for audio feedback on user notifications.
// If svc is nil, the handler keeps its default NoopSoundService.
func WithNotifyUserSoundService(svc sound.SoundService) NotifyUserHandlerOption {
	return func(h *NotifyUserHandler) {
		if svc != nil {
			h.soundService = svc
		}
	}
}

// NewNotifyUserHandler creates a new NotifyUserHandler.
func NewNotifyUserHandler(opts ...NotifyUserHandlerOption) *NotifyUserHandler {
	h := &NotifyUserHandler{
		soundService: sound.NoopSoundService{},
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle processes a NotifyUserCommand.
// 1. Validates the command
// 2. Plays the user_notification sound
// 3. Emits ProcessUserNotification event for the TUI to display
func (h *NotifyUserHandler) Handle(_ context.Context, cmd command.Command) (*command.CommandResult, error) {
	notifyCmd := cmd.(*command.NotifyUserCommand)

	// 1. Validate the command
	if err := notifyCmd.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// 2. Play notification sound
	h.soundService.Play("notification", "user_notification")

	// 3. Build ProcessUserNotification event
	event := events.ProcessEvent{
		Type:      events.ProcessUserNotification,
		ProcessID: "coordinator",
		Role:      events.RoleCoordinator,
		Output:    notifyCmd.Message,
		TaskID:    notifyCmd.TaskID,
	}

	result := &NotifyUserResult{
		Message: notifyCmd.Message,
		Phase:   notifyCmd.Phase,
		TaskID:  notifyCmd.TaskID,
	}

	return SuccessWithEvents(result, event), nil
}

// NotifyUserResult contains the result of notifying the user.
type NotifyUserResult struct {
	Message string
	Phase   string
	TaskID  string
}
