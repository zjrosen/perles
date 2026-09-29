package cmd

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeUserDashboardWorkflow writes ~/.perles/workflows/my-flow/template.yaml.
func writeUserDashboardWorkflow(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".perles", "workflows", "my-flow")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "template.yaml"), []byte(userWorkflowYAML), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "step1.md"), []byte("# Step 1"), 0o600))
}

// writeUserChatWorkflow writes ~/.perles/workflows/my_chat.md.
func writeUserChatWorkflow(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".perles", "workflows")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	content := "---\nname: \"My Chat Flow\"\ndescription: \"A chat workflow\"\ntarget_mode: \"chat\"\n---\n\n# My Chat Flow\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "my_chat.md"), []byte(content), 0o600))
}

func TestWorkflowsCommand_ListsDashboardAndChatPanelWorkflows(t *testing.T) {
	home, _ := isolateConfig(t)
	writeUserDashboardWorkflow(t, home)
	writeUserChatWorkflow(t, home)

	out, err := executeRoot(t, "workflows")
	require.NoError(t, err)

	dashboardIdx := regexp.MustCompile(`(?m)^Dashboard Workflows`).FindStringIndex(out)
	chatIdx := regexp.MustCompile(`(?m)^Chat Panel Workflows`).FindStringIndex(out)
	require.NotNil(t, dashboardIdx, "output should have a Dashboard Workflows section")
	require.NotNil(t, chatIdx, "output should have a Chat Panel Workflows section")
	require.Less(t, dashboardIdx[0], chatIdx[0], "dashboard workflows should be the primary (first) section")

	dashboard, chat := out[dashboardIdx[0]:chatIdx[0]], out[chatIdx[0]:]

	// Dashboard section: RegistryService namespace "workflow" with source labels.
	require.Regexp(t, `(?m)^  cook\s+\[built-in\]\s+Cook - `, dashboard)
	require.Regexp(t, `(?m)^  my-flow\s+\[user\]\s+My Flow - A user workflow for testing$`, dashboard)
	require.Contains(t, dashboard, "Community workflows: none enabled (add IDs to orchestration.community_workflows)")
	require.Contains(t, dashboard, filepath.Join(home, ".perles", "workflows", "<name>", "template.yaml"))
	require.Contains(t, dashboard, "press ctrl+o to open the dashboard, then n (New Workflow)")

	// Chat panel section: markdown workflows the chat panel's Workflows tab shows.
	require.Regexp(t, `(?m)^  chat_cook\s+\[built-in\]\s+Cook - `, chat)
	require.Regexp(t, `(?m)^  my_chat\s+\[user\]\s+My Chat Flow - A chat workflow$`, chat)
	require.NotRegexp(t, `(?m)^  epic_driven\s`, chat, "orchestration-only markdown workflows are not shown in the chat panel")
	require.Contains(t, chat, filepath.Join(home, ".perles", "workflows", "*.md"))
	require.Contains(t, chat, "press ctrl+w in kanban or search mode to open the chat panel")
	require.Contains(t, chat, "ctrl+t (Workflows tab)")

	// The removed orchestration mode must not be referenced.
	require.NotContains(t, out, "Ctrl+P")
	require.NotContains(t, out, "orchestration mode")
}

func TestWorkflowsCommand_CommunityWorkflowsEnabled(t *testing.T) {
	isolateConfig(t)
	loadConfigFile(t, "orchestration:\n  community_workflows:\n    - joke-contest\n")

	out, err := executeRoot(t, "workflows", "--config", cfgFile)
	require.NoError(t, err)

	require.Regexp(t, `(?m)^  joke-contest\s+\[community\]\s+Joke Contest - `, out)
	require.NotContains(t, out, "Community workflows: none enabled")
}

func TestWorkflowsCommand_UsesConfiguredDashboardKey(t *testing.T) {
	isolateConfig(t)
	loadConfigFile(t, "ui:\n  keybindings:\n    dashboard: ctrl+d\n")

	out, err := executeRoot(t, "workflows", "--config", cfgFile)
	require.NoError(t, err)

	require.Contains(t, out, "press ctrl+d to open the dashboard, then n (New Workflow)")
}
