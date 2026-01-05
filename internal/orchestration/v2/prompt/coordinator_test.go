package prompt

import (
	"strings"
	"testing"
)

func TestBuildCoordinatorSystemPrompt_NoHardcodedWorkerCount(t *testing.T) {
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() error = %v", err)
	}

	// Verify prompt no longer contains "wait for 4 workers" or "all 4 workers"
	forbiddenPhrases := []string{
		"wait for all 4 workers",
		"all 4 are ready",
		"All 4 workers are ready",
		"wait for 4 workers",
	}

	for _, phrase := range forbiddenPhrases {
		if strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should not contain hardcoded worker count phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorSystemPrompt_ContainsSpawnWorkerInstructions(t *testing.T) {
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() error = %v", err)
	}

	// Verify prompt contains spawn_worker usage instructions
	requiredPhrases := []string{
		"spawn_worker",
		"STATE 1.5",
		"Spawn Workers",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should contain spawn worker instruction phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorSystemPrompt_ContainsSpawnFailureHandling(t *testing.T) {
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() error = %v", err)
	}

	// Verify prompt contains spawn failure handling instructions
	requiredPhrases := []string{
		"Spawn failure handling",
		"fails, inform the user",
		"maxWorkers",
		"worker limit",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should contain spawn failure handling phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorSystemPrompt_ContainsPrecedenceRule(t *testing.T) {
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() error = %v", err)
	}

	// Verify prompt contains precedence rule for frontmatter
	requiredPhrases := []string{
		"Precedence rule",
		"frontmatter",
		"workers: N",
		"Ignore any spawn counts mentioned in the workflow prose",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should contain precedence rule phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorSystemPrompt_TemplateCompiles(t *testing.T) {
	// This test verifies the template compiles without errors
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() should compile without error, got: %v", err)
	}

	if prompt == "" {
		t.Error("BuildCoordinatorSystemPrompt() should return non-empty prompt")
	}

	// Verify it contains essential sections
	if !strings.Contains(prompt, "# Coordinator Agent") {
		t.Error("Prompt should contain coordinator agent header")
	}

	if !strings.Contains(prompt, "## Core Session Workflow") {
		t.Error("Prompt should contain core session workflow section")
	}
}

func TestBuildCoordinatorSystemPrompt_LazySpawnBootState(t *testing.T) {
	prompt, err := BuildCoordinatorSystemPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorSystemPrompt() error = %v", err)
	}

	// Verify STATE 0 reflects lazy spawning (boot only, no waiting for workers)
	bootStatePhrases := []string{
		"STATE 0 — Boot",
		"Wait for your own initialization to complete",
		"DO NOT** spawn workers",
		"Coordinator ready",
	}

	for _, phrase := range bootStatePhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Prompt should contain lazy spawn boot state phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorInitialPrompt_LazySpawn(t *testing.T) {
	prompt, err := BuildCoordinatorInitialPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorInitialPrompt() error = %v", err)
	}

	// Verify initial prompt reflects lazy spawning
	requiredPhrases := []string{
		"Report coordinator readiness",
		"No workers are spawned yet",
		"spawn them after the user selects a workflow",
		"Coordinator ready",
	}

	for _, phrase := range requiredPhrases {
		if !strings.Contains(prompt, phrase) {
			t.Errorf("Initial prompt should contain lazy spawn phrase %q", phrase)
		}
	}

	// Verify it doesn't reference waiting for workers
	forbiddenPhrases := []string{
		"waiting the readiness signals from our workers",
		"workers are ready",
		"wait for workers",
	}

	for _, phrase := range forbiddenPhrases {
		if strings.Contains(prompt, phrase) {
			t.Errorf("Initial prompt should not contain old worker waiting phrase %q", phrase)
		}
	}
}

func TestBuildCoordinatorInitialPrompt_DoNotSpawnBeforeWorkflow(t *testing.T) {
	prompt, err := BuildCoordinatorInitialPrompt()
	if err != nil {
		t.Fatalf("BuildCoordinatorInitialPrompt() error = %v", err)
	}

	// Verify initial prompt instructs not to spawn before workflow
	if !strings.Contains(prompt, "DO NOT spawn workers") {
		t.Error("Initial prompt should instruct not to spawn workers before workflow")
	}
}
