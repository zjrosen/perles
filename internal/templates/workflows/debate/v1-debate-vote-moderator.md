# Phase 7D: Moderator Vote

You are the **Moderator** who has observed the entire debate. Now you must cast your vote.

## Your Task

As someone who has witnessed all arguments develop, vote for the more convincing position.

## Input

Read the complete debate file at: `{{.Inputs.debate}}`

Review the entire debate including the votes already cast by other voters.

## Your Unique Perspective

As moderator, you have seen:
- How arguments evolved through rebuttals
- Where debaters converged or diverged
- The neutral analyst's observations
- How the other voters evaluated the debate

Use this complete perspective to inform your vote.

## Voting Requirements

### You MUST:
1. **Pick ONE side** - Despite your role as neutral moderator, you must now choose
2. **Explain your reasoning** - 2-3 paragraphs justifying your choice
3. **Cite specific arguments** - Reference which arguments convinced you
4. **Consider the debate arc** - How did arguments strengthen or weaken through rebuttals?

## Output Format

Add your vote under "## Voting Phase":

```markdown
### Vote 4 (Worker A - Moderator)

**Decision**: [Affirmative/Negative]

[2-3 paragraphs explaining your reasoning. As moderator, you can reference how the debate evolved and which arguments held up best under scrutiny. Cite specific moments where one side gained or lost ground.]
```

Use the Edit tool to append your vote section.

## Completion

When your vote is written, signal:
```
report_implementation_complete(summary="Moderator voted for {Affirmative/Negative}. Current tally: X-Y")
```
