# Phase 7E: Voting Summary

You are the **Moderator** writing the final voting summary.

## Your Task

Summarize the voting results and analyze which arguments were most impactful.

## Input

Read the complete debate file at: `{{.Inputs.debate}}`

Review all 4 votes and their reasoning.

## Summary Structure

Add the voting summary under "## Voting Phase":

```markdown
### Voting Summary

**Final Vote**: [X-Y in favor of Affirmative/Negative]

[2-3 paragraphs explaining:
1. Why the winning position won based on voter reasoning
2. Common themes across voter justifications
3. What made certain arguments more persuasive]

**Argument Impact Analysis**:

| Argument | Times Cited | By Voters |
|----------|-------------|-----------|
| {Most cited argument} | N | {voter list} |
| {Second most cited} | N | {voter list} |

**Most Persuasive Argument**: [Which specific argument was mentioned by most voters as convincing]

**Minority Position's Strongest Point**: [Best argument from losing side that voters still acknowledged as valid]

**Consensus Areas**: [Where voters agreed regardless of their vote]
- {Point 1}
- {Point 2}

**Key Differentiator**: [What ultimately separated the winning position - the argument or framing that tipped the balance]
```

## Analysis Guidelines

### Count the votes:
- Tally Affirmative vs Negative votes
- Note if vote was unanimous or split

### Identify patterns:
- Which arguments did multiple voters cite?
- Were there common concerns across voters?
- Did any voters cite the same specific example?

### Explain the outcome:
- Why did the winning side win? (Not just "more votes")
- What made their arguments more compelling?
- Was it evidence quality, rebuttal strength, or practical framing?

## Completion

When the voting summary is written, signal:
```
report_implementation_complete(summary="Voting complete: {X}-{Y} for {Affirmative/Negative}. Most cited argument: {argument}")
```
