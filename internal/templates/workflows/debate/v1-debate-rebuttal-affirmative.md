# Phase 3A: Rebuttal - Affirmative

You are arguing **FOR** the proposition. Now you must respond to the negative's opening argument.

## Your Task

Write a rebuttal (2-3 paragraphs) responding to the negative's opening argument.

## Input

Read the debate file at: `{{.Inputs.debate}}`

**Carefully analyze the negative's opening argument** - identify their key claims and evidence.

## Rebuttal Guidelines

### Structure (2-3 paragraphs)
1. **Address strongest opposing point**: Don't cherry-pick weak arguments
2. **Counter with evidence**: Use data, examples, or logic to refute
3. **Reframe the debate**: Show why your framing better captures reality

### Effective Rebuttal Techniques
- "The negative correctly identifies X, but overlooks the crucial factor of Y"
- "The cited example of Z actually supports the affirmative when you consider..."
- "This concern is valid in theory, but real-world data shows..."
- Challenge hidden assumptions in opposing arguments

### What NOT to Do
- Don't just repeat your opening argument
- Don't attack strawman versions of their argument
- Don't dismiss valid points without engagement
- Don't introduce entirely new arguments (save for closing)

## Output

Use the Edit tool to REPLACE the placeholder under "## Rebuttal: Affirmative (Worker B)" with your rebuttal.

Replace:
```
[To be filled by Worker B]
```

With your 2-3 paragraph rebuttal.

## Completion

When your rebuttal is written, signal:
```
report_implementation_complete(summary="Wrote affirmative rebuttal addressing {main point countered}")
```
