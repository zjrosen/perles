# Phase 3B: Counter-Rebuttal - Negative

You are arguing **AGAINST** the proposition. Now you must respond to the affirmative's rebuttal.

## Your Task

Write a counter-rebuttal (2-3 paragraphs) responding to the affirmative's rebuttal.

## Input

Read the debate file at: `{{.Inputs.debate}}`

**Carefully analyze the affirmative's rebuttal** - how did they try to counter your opening argument?

## Counter-Rebuttal Guidelines

### Structure (2-3 paragraphs)
1. **Defend your core position**: Show why your key points still stand
2. **Address their counter-evidence**: Explain why it's incomplete or misleading
3. **Strengthen your narrative**: Provide additional evidence or reframing

### Effective Counter-Rebuttal Techniques
- "The affirmative's counter-example actually proves my point because..."
- "While they addressed concern X, they still haven't explained Y"
- "The data they cite is from [context] which doesn't apply to [typical scenario]"
- Show where their rebuttal creates new contradictions

### What NOT to Do
- Don't just repeat your opening argument
- Don't ignore their strongest counter-points
- Don't get personal or dismissive
- Don't move goalposts from your original claims

## Output

Use the Edit tool to REPLACE the placeholder under "## Counter-Rebuttal: Negative (Worker C)" with your counter-rebuttal.

Replace:
```
[To be filled by Worker C]
```

With your 2-3 paragraph counter-rebuttal.

## Completion

When your counter-rebuttal is written, signal:
```
report_implementation_complete(summary="Wrote negative counter-rebuttal defending {main point} against affirmative's response")
```
