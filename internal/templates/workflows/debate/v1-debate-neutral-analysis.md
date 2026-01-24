# Phase 5: Neutral Analysis

You are providing **NEUTRAL ANALYSIS** of the debate. You are NOT advocating for either side.

## Your Task

Analyze both positions objectively (3-4 paragraphs) and provide a decision framework.

## Input

Read the complete debate file at: `{{.Inputs.debate}}`

**Read the ENTIRE debate**: openings, rebuttals, and closing statements from both sides.

## Analysis Guidelines

### Structure (3-4 paragraphs)

**Paragraph 1: Strengths of Each Position**
- What did the affirmative get right?
- What did the negative get right?
- Where did both sides agree (convergence)?

**Paragraph 2: Weaknesses and Blind Spots**
- What did the affirmative miss or downplay?
- What did the negative miss or downplay?
- What assumptions went unchallenged?

**Paragraph 3: Context-Dependent Guidance**
- When does the affirmative position win?
- When does the negative position win?
- What are the key decision criteria?

**Paragraph 4: Hidden Trade-offs**
- What second-order effects were not discussed?
- What non-obvious costs exist on both sides?
- What long-term considerations matter?

### Key Observations Section

After your paragraphs, add structured observations:

```markdown
### Key Observations

- **Both sides agree on:** [list areas of convergence]
- **Core disagreement:** [identify the fundamental tension]
- **Context matters:** [explain when each approach wins]

### Decision Framework

| If you have... | Choose... | Because... |
|----------------|-----------|------------|
| {condition A}  | {approach} | {rationale} |
| {condition B}  | {approach} | {rationale} |

### Overlooked Considerations

1. {What neither side fully addressed}
2. {Non-obvious factor that matters}
```

## Critical: Maintain Neutrality

- Do NOT advocate for either position
- Do NOT declare a "winner"
- Focus on helping the reader make THEIR decision based on THEIR context
- Acknowledge trade-offs, not superiority

## Output

Use the Edit tool to REPLACE the placeholder under "## Neutral Analysis (Worker D)" with your analysis.

Replace:
```
[To be filled by Worker D]
```

With your 3-4 paragraph analysis plus Key Observations, Decision Framework, and Overlooked Considerations.

## Completion

When your analysis is written, signal:
```
report_implementation_complete(summary="Wrote neutral analysis identifying {N} key decision criteria and {M} overlooked considerations")
```
