# Phase 7: Cast Your Vote

You are a **fresh, unbiased voter** who has NOT participated in the debate. Your job is to evaluate the arguments objectively and vote for the more convincing position.

## Your Task

Read the complete debate and vote for either the Affirmative or Negative position.

## Input

Read the complete debate file at: `{{.Inputs.debate}}`

**Read EVERYTHING**: introduction, all arguments, rebuttals, closing statements, neutral analysis, and moderator summary.

## Voting Requirements

### You MUST:
1. **Pick ONE side** - No "both are right" or abstaining
2. **Explain your reasoning** - 2-3 paragraphs justifying your choice
3. **Cite specific arguments** - Reference which arguments convinced you
4. **Be objective** - Judge based on evidence quality, not personal preference

### Consider:
- Which side presented stronger evidence?
- Which side better addressed counterarguments?
- Which side's examples were more concrete and relevant?
- Which side acknowledged trade-offs more honestly?
- Which side provided clearer decision frameworks?

## Output Format

Add your vote to the debate file under "## Voting Phase".

If the "## Voting Phase" section doesn't exist yet, create it after the Moderator Closing Summary:

```markdown
## Voting Phase

### Vote N (Worker-[your-id])

**Decision**: [Affirmative/Negative]

[2-3 paragraphs explaining your reasoning. Be specific about which arguments convinced you and why. Cite examples from the debate.]
```

Use the Edit tool to append your vote section.

## Completion

When your vote is written, signal:
```
report_implementation_complete(summary="Voted for {Affirmative/Negative} based on {key reason}")
```

**Remember:** You are judging argument quality, not personal beliefs. Vote for the side that made the better case.
