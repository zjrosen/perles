# Phase 4B: Closing Statement - Negative

You are arguing **AGAINST** the proposition. This is your final statement.

## Your Task

Write a closing statement (2 paragraphs) synthesizing your position and providing practical guidance.

## Input

Read the debate file at: `{{.Inputs.debate}}`

Review the entire debate so far, including the affirmative's closing statement.

## Closing Statement Requirements

### Structure (2 paragraphs)

**Paragraph 1: Reaffirm Core Value**
- Summarize why your position is sound
- Highlight the strongest evidence that went unchallenged
- Acknowledge what the affirmative got right

**Paragraph 2: Practical Guidance**
- When should someone choose your approach?
- When might the alternative be reasonable?
- End with a memorable takeaway

### Key Principles
- **Acknowledge legitimate opposing use cases** - Don't claim universal superiority
- **Provide decision framework** - Help the reader know when to choose each approach
- **Synthesize, don't escalate** - This is conclusion, not escalation
- **Be memorable** - Leave the reader with a clear takeaway

### Example Closing Patterns
- "For the majority of teams, the simpler approach offers [benefits] without [costs]. For the minority facing [specific conditions], the proposition may be justified."
- "The default should be [your approach]. Upgrade to [their approach] when you have [specific evidence of need]."

## Output

Use the Edit tool to REPLACE the placeholder under "## Closing Statement: Negative (Worker C)" with your closing.

Replace:
```
[To be filled by Worker C]
```

With your 2 paragraph closing statement.

## Completion

When your closing statement is written, signal:
```
report_implementation_complete(summary="Wrote negative closing statement with decision framework for default approach")
```
