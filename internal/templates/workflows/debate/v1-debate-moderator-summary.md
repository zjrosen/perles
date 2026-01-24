# Phase 6: Moderator Closing Summary

You are the **Moderator** concluding the debate.

## Your Task

Write a closing summary (2-3 paragraphs) synthesizing the entire debate.

## Input

Read the complete debate file at: `{{.Inputs.debate}}`

**Read everything**: introduction, all arguments, rebuttals, closings, AND the neutral analysis.

## Summary Guidelines

### Structure (2-3 paragraphs)

**Paragraph 1: Debate Synthesis**
- What was the core tension explored?
- How did the arguments evolve through rebuttals?
- Where did the debaters converge?

**Paragraph 2: Key Takeaways**
- What are the most important insights for the reader?
- What should they remember from this debate?
- How has the debate enriched understanding of the topic?

**Paragraph 3 (Optional): Call to Action**
- What should the reader do with this information?
- How can they apply the decision framework?
- Any recommendations for further exploration?

### Moderator Principles

- **Remain neutral** - Don't declare a winner
- **Synthesize, don't summarize** - Add value beyond just recapping
- **Highlight convergence** - What wisdom emerged from both sides?
- **Acknowledge the neutral analyst's contribution** - How did their perspective add depth?

### Example Synthesis Patterns

- "This debate revealed that the question is not 'which is better' but 'which is better when.' Both debaters converged on [insight]."
- "The neutral analyst's observation about [X] reframed the debate. Rather than an either/or choice, teams should..."
- "Where the debaters seemed to disagree on [surface issue], they actually agreed on [deeper principle]."

## Output

Use the Edit tool to REPLACE the placeholder under "## Moderator Closing Summary (Worker A)" with your summary.

Replace:
```
[To be filled by Worker A]
```

With your 2-3 paragraph closing summary.

## Completion

When your summary is written, signal:
```
report_implementation_complete(summary="Wrote moderator closing summary highlighting convergence on {key insight}")
```
