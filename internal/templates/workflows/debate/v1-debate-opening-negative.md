# Phase 2B: Opening Argument - Negative

You are arguing **AGAINST** the proposition.

## Your Task

Write a compelling opening argument (3-4 paragraphs) opposing the proposition with technical evidence.

## Input

Read the debate file at: `{{.Inputs.debate}}`

Pay attention to the affirmative's opening argument - you'll need to counter their points in the rebuttal phase.

## Argument Guidelines

### Structure (3-4 paragraphs)
1. **Core thesis**: State your opposing position clearly and why it matters
2. **Primary evidence**: Your strongest technical argument with concrete examples
3. **Supporting evidence**: Additional arguments reinforcing your position
4. **Real-world validation**: Cite specific companies, systems, or benchmarks

### Evidence Requirements
- Name specific systems that succeeded with the alternative approach
- Include concrete numbers where possible (complexity costs, failure rates, team friction)
- Reference actual architectural decisions and their outcomes
- Acknowledge some validity to opposition but argue costs outweigh benefits

### Enhance with Visual Aids
- Code examples showing simpler alternative approaches
- Architecture diagrams showing complexity trade-offs
- Comparison tables with real failure modes

## Output

Use the Edit tool to REPLACE the placeholder under "## Opening Argument: Negative (Worker C)" with your argument.

Replace:
```
[To be filled by Worker C]
```

With your 3-4 paragraph opening argument.

## Completion

When your opening argument is written, signal:
```
report_implementation_complete(summary="Wrote negative opening argument focusing on {main thesis}")
```

**Remember:** This is technical discourse. Be rigorous, cite evidence, but advocate strongly for your position.
