package roles

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// ============================================================================
// Observer Prompt Tests
// ============================================================================

// TestObserverPrompt_ContainsPassiveInstruction verifies prompt contains
// "passive" or "observe" instruction as required by acceptance criteria.
func TestObserverPrompt_ContainsPassiveInstruction(t *testing.T) {
	prompt := ObserverSystemPrompt()
	promptLower := strings.ToLower(prompt)

	hasPassive := strings.Contains(promptLower, "passive")
	hasObserve := strings.Contains(promptLower, "observe")

	require.True(t, hasPassive || hasObserve,
		"ObserverSystemPrompt should contain 'passive' or 'observe' instruction")
}

// TestObserverPrompt_ContainsNeverRespondInstruction verifies prompt contains
// instruction to never respond to coordinator/worker as required by acceptance criteria.
func TestObserverPrompt_ContainsNeverRespondInstruction(t *testing.T) {
	prompt := ObserverSystemPrompt()

	require.Contains(t, prompt, "NEVER respond to coordinator or worker",
		"ObserverSystemPrompt should explicitly state 'NEVER respond to coordinator or worker messages'")
}

// TestObserverPrompt_ContainsObserverChannelOnly verifies prompt mentions
// #observer as the only allowed response channel as required by acceptance criteria.
func TestObserverPrompt_ContainsObserverChannelOnly(t *testing.T) {
	prompt := ObserverSystemPrompt()

	require.Contains(t, prompt, "ONLY respond",
		"ObserverSystemPrompt should contain 'ONLY respond' instruction")
	require.Contains(t, prompt, "#observer",
		"ObserverSystemPrompt should mention #observer channel")
	require.Contains(t, prompt, "ONLY WRITE CHANNEL",
		"ObserverSystemPrompt should emphasize #observer is the only write channel")
}

// TestObserverPrompt_ContainsChannelDescriptions verifies prompt includes
// descriptions of all fabric channels as required by acceptance criteria.
func TestObserverPrompt_ContainsChannelDescriptions(t *testing.T) {
	prompt := ObserverSystemPrompt()

	channels := []string{
		"#system",
		"#tasks",
		"#planning",
		"#general",
		"#observer",
	}

	for _, channel := range channels {
		require.Contains(t, prompt, channel,
			"ObserverSystemPrompt should contain description for %s", channel)
	}

	// Verify the channel descriptions section header exists
	require.Contains(t, prompt, "FABRIC CHANNEL DESCRIPTIONS",
		"ObserverSystemPrompt should have a channel descriptions section")
}

// TestObserverPrompt_ContainsActionLimitation verifies prompt explains
// Observer cannot take orchestration actions as required by acceptance criteria.
func TestObserverPrompt_ContainsActionLimitation(t *testing.T) {
	prompt := ObserverSystemPrompt()

	require.Contains(t, prompt, "CANNOT take orchestration actions",
		"ObserverSystemPrompt should explain Observer cannot take actions")
	require.Contains(t, prompt, "spawn workers",
		"ObserverSystemPrompt should mention inability to spawn workers")
	require.Contains(t, prompt, "assign tasks",
		"ObserverSystemPrompt should mention inability to assign tasks")
}

// TestObserverSystemPrompt_ReturnsNonEmpty verifies prompt is not empty.
func TestObserverSystemPrompt_ReturnsNonEmpty(t *testing.T) {
	prompt := ObserverSystemPrompt()
	require.NotEmpty(t, prompt,
		"ObserverSystemPrompt should return non-empty string")
}

// TestObserverIdlePrompt_ReturnsNonEmpty verifies idle prompt is not empty.
func TestObserverIdlePrompt_ReturnsNonEmpty(t *testing.T) {
	prompt := ObserverIdlePrompt()
	require.NotEmpty(t, prompt,
		"ObserverIdlePrompt should return non-empty string")
}

// TestObserverSystemPromptVersion_IsSemver verifies version follows semver format.
func TestObserverSystemPromptVersion_IsSemver(t *testing.T) {
	// Semver regex pattern (simplified for major.minor.patch)
	semverPattern := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	require.True(t, semverPattern.MatchString(ObserverSystemPromptVersion),
		"ObserverSystemPromptVersion %q should follow semver format (x.y.z)",
		ObserverSystemPromptVersion)
}

// TestObserverIdlePrompt_ContainsSubscriptionInstructions verifies idle prompt
// instructs the Observer to subscribe to all channels on startup.
func TestObserverIdlePrompt_ContainsSubscriptionInstructions(t *testing.T) {
	prompt := ObserverIdlePrompt()

	require.Contains(t, prompt, "fabric_subscribe",
		"ObserverIdlePrompt should contain subscription instructions")
	require.Contains(t, prompt, "Subscribe to all channels",
		"ObserverIdlePrompt should instruct subscribing to all channels")
}

// TestObserverIdlePrompt_IdentifiesAsObserver verifies role identification.
func TestObserverIdlePrompt_IdentifiesAsObserver(t *testing.T) {
	prompt := ObserverIdlePrompt()
	require.Contains(t, prompt, "Observer",
		"ObserverIdlePrompt should identify as Observer")
}

// TestObserverSystemPrompt_MentionsReadOnlyTools verifies prompt lists read-only tools.
func TestObserverSystemPrompt_MentionsReadOnlyTools(t *testing.T) {
	prompt := ObserverSystemPrompt()

	readOnlyTools := []string{
		"fabric_inbox",
		"fabric_history",
		"fabric_read_thread",
		"fabric_subscribe",
		"fabric_ack",
	}

	for _, tool := range readOnlyTools {
		require.Contains(t, prompt, tool,
			"ObserverSystemPrompt should mention read-only tool %s", tool)
	}
}

// TestObserverSystemPrompt_MentionsRestrictedWriteTools verifies prompt lists
// restricted write tools with their limitations.
func TestObserverSystemPrompt_MentionsRestrictedWriteTools(t *testing.T) {
	prompt := ObserverSystemPrompt()

	require.Contains(t, prompt, "fabric_send",
		"ObserverSystemPrompt should mention fabric_send as restricted")
	require.Contains(t, prompt, "fabric_reply",
		"ObserverSystemPrompt should mention fabric_reply as restricted")
	require.Contains(t, prompt, "Restricted write tools",
		"ObserverSystemPrompt should have a restricted write tools section")
}
