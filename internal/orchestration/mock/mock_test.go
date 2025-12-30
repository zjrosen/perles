package mock

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/zjrosen/perles/internal/orchestration/client"
)

func TestClient_Type(t *testing.T) {
	c := NewClient()
	require.Equal(t, client.ClientMock, c.Type())
}

func TestClient_Spawn_Default(t *testing.T) {
	c := NewClient()

	proc, err := c.Spawn(context.Background(), client.Config{
		WorkDir: "/test",
		Prompt:  "test prompt",
	})

	require.NoError(t, err)
	require.NotNil(t, proc)
	require.True(t, proc.IsRunning())
	require.Equal(t, 1, c.SpawnCount())
}

func TestClient_Spawn_CustomFunc(t *testing.T) {
	c := NewClient()
	customProc := NewProcess()
	customProc.SetSessionID("custom-session")

	c.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		return customProc, nil
	}

	proc, err := c.Spawn(context.Background(), client.Config{})
	require.NoError(t, err)
	require.Equal(t, "custom-session", proc.SessionRef())
}

func TestClient_Spawn_Error(t *testing.T) {
	c := NewClient()
	expectedErr := errors.New("spawn failed")

	c.SpawnFunc = func(ctx context.Context, cfg client.Config) (client.HeadlessProcess, error) {
		return nil, expectedErr
	}

	proc, err := c.Spawn(context.Background(), client.Config{})
	require.ErrorIs(t, err, expectedErr)
	require.Nil(t, proc)
}

func TestClient_Spawn_WithSessionID_IsResume(t *testing.T) {
	c := NewClient()

	proc, err := c.Spawn(context.Background(), client.Config{
		SessionID: "session-123",
	})
	require.NoError(t, err)
	require.NotNil(t, proc)
	require.Equal(t, "session-123", proc.SessionRef())
	require.Equal(t, 1, c.SpawnCount())
	require.Equal(t, 1, c.ResumeCount())
}

func TestClient_Reset(t *testing.T) {
	c := NewClient()

	_, _ = c.Spawn(context.Background(), client.Config{})
	_, _ = c.Spawn(context.Background(), client.Config{})
	_, _ = c.Spawn(context.Background(), client.Config{SessionID: "s"})

	require.Equal(t, 3, c.SpawnCount())
	require.Equal(t, 1, c.ResumeCount())

	c.Reset()

	require.Equal(t, 0, c.SpawnCount())
	require.Equal(t, 0, c.ResumeCount())
}

func TestProcess_InitialState(t *testing.T) {
	p := NewProcess()

	require.True(t, p.IsRunning())
	require.Equal(t, client.StatusRunning, p.Status())
	require.Empty(t, p.SessionRef())
	require.Empty(t, p.WorkDir())
	require.Equal(t, 0, p.PID())
}

func TestProcess_SetMethods(t *testing.T) {
	p := NewProcess()

	p.SetSessionID("sess-123")
	p.SetWorkDir("/work")
	p.SetPID(12345)

	require.Equal(t, "sess-123", p.SessionRef())
	require.Equal(t, "/work", p.WorkDir())
	require.Equal(t, 12345, p.PID())
}

func TestProcess_SendEvent(t *testing.T) {
	p := NewProcess()

	p.SendEvent(client.OutputEvent{
		Type:      client.EventSystem,
		SubType:   "init",
		SessionID: "sess-abc",
	})

	select {
	case event := <-p.Events():
		require.Equal(t, client.EventSystem, event.Type)
		require.Equal(t, "init", event.SubType)
		require.Equal(t, "sess-abc", event.SessionID)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for event")
	}
}

func TestProcess_Complete(t *testing.T) {
	p := NewProcess()

	require.True(t, p.IsRunning())

	p.Complete()

	require.False(t, p.IsRunning())
	require.Equal(t, client.StatusCompleted, p.Status())

	err := p.Wait()
	require.NoError(t, err)
}

func TestProcess_Fail(t *testing.T) {
	p := NewProcess()
	failErr := errors.New("failure")

	p.Fail(failErr)

	require.False(t, p.IsRunning())
	require.Equal(t, client.StatusFailed, p.Status())

	err := p.Wait()
	require.Equal(t, failErr, err)
}

func TestProcess_Cancel(t *testing.T) {
	p := NewProcess()

	err := p.Cancel()
	require.NoError(t, err)

	require.False(t, p.IsRunning())
	require.Equal(t, client.StatusCancelled, p.Status())
}

func TestProcess_SendInitEvent(t *testing.T) {
	p := NewProcess()

	p.SendInitEvent("sess-xyz", "/workspace")

	require.Equal(t, "sess-xyz", p.SessionRef())
	require.Equal(t, "/workspace", p.WorkDir())

	select {
	case event := <-p.Events():
		require.True(t, event.IsInit())
		require.Equal(t, "sess-xyz", event.SessionID)
		require.Equal(t, "/workspace", event.WorkDir)
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for init event")
	}
}

func TestProcess_SendTextEvent(t *testing.T) {
	p := NewProcess()

	p.SendTextEvent("Hello, world!")

	select {
	case event := <-p.Events():
		require.True(t, event.IsAssistant())
		require.NotNil(t, event.Message)
		require.Equal(t, "Hello, world!", event.Message.GetText())
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for text event")
	}
}

func TestProcess_SendToolResultEvent(t *testing.T) {
	p := NewProcess()

	p.SendToolResultEvent("tool-123", "Bash", "command output")

	select {
	case event := <-p.Events():
		require.True(t, event.IsToolResult())
		require.NotNil(t, event.Tool)
		require.Equal(t, "tool-123", event.Tool.ID)
		require.Equal(t, "Bash", event.Tool.Name)
		require.Equal(t, "command output", event.Tool.GetOutput())
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "timeout waiting for tool result event")
	}
}

func TestProcess_Done(t *testing.T) {
	p := NewProcess()

	select {
	case <-p.Done():
		require.Fail(t, "should not be done")
	default:
	}

	p.Complete()

	select {
	case <-p.Done():
	case <-time.After(100 * time.Millisecond):
		require.Fail(t, "should be done")
	}
}

func TestNewProcessWithConfig(t *testing.T) {
	p := NewProcessWithConfig(client.Config{
		WorkDir:   "/test/dir",
		SessionID: "config-session",
	})

	require.Equal(t, "/test/dir", p.WorkDir())
	require.Equal(t, "config-session", p.SessionRef())
}

func TestClientRegistration(t *testing.T) {
	c, err := client.NewClient(client.ClientMock)
	require.NoError(t, err)
	require.Equal(t, client.ClientMock, c.Type())
}

func TestProcess_MultipleComplete(t *testing.T) {
	p := NewProcess()

	p.Complete()
	p.Complete()

	require.Equal(t, client.StatusCompleted, p.Status())
}

func TestProcess_CancelTwice(t *testing.T) {
	p := NewProcess()

	err1 := p.Cancel()
	err2 := p.Cancel()

	require.NoError(t, err1)
	require.NoError(t, err2)
	require.Equal(t, client.StatusCancelled, p.Status())
}
