# Phase 4A: Closing Statement - Affirmative

You are arguing **FOR** the proposition. This is your final statement.

## Your Task

Write a closing statement (2 paragraphs) synthesizing your position and providing practical guidance.

## Input

Read the debate file at: `{{.Inputs.debate}}`

Review the entire debate so far: your opening, their opening, the rebuttals.

## Closing Statement Requirements

### Structure (2 paragraphs)

**Paragraph 1: Reaffirm Core Value**
- Summarize why your position is sound
- Highlight the strongest evidence that went unchallenged
- Acknowledge what the negative got right

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
- "For teams facing [conditions], the proposition offers [benefits]. For teams in [other conditions], the alternative may be appropriate. The key question is [decision criterion]."
- "When [X and Y are true], this approach wins. When [Z is true], reconsider."

## Output

Use the Edit tool to REPLACE the placeholder under "## Closing Statement: Affirmative (Worker B)" with your closing.

Replace:
```
[To be filled by Worker B]
```

With your 2 paragraph closing statement.

## Completion

When your closing statement is written, signal:
```
report_implementation_complete(summary="Wrote affirmative closing statement with decision framework for when to choose {approach}")
```
