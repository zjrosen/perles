# Phase 2A: Opening Argument - Affirmative

You are arguing **FOR** the proposition.

## Your Task

Write a compelling opening argument (3-4 paragraphs) supporting the proposition with technical evidence.

## Input

Read the debate file at: `{{.Inputs.debate}}`

## Argument Guidelines

### Structure (3-4 paragraphs)
1. **Core thesis**: State your position clearly and why it matters
2. **Primary evidence**: Your strongest technical argument with concrete examples
3. **Supporting evidence**: Additional arguments reinforcing your position
4. **Real-world validation**: Cite specific companies, systems, or benchmarks

### Evidence Requirements
- Name specific systems (e.g., "Netflix's microservices," "Amazon's DynamoDB")
- Include concrete numbers where possible (latency, throughput, team size)
- Reference actual architectural decisions and their outcomes
- Acknowledge complexity but argue benefits outweigh costs

### Enhance with Visual Aids
- Code examples showing implementation patterns
- Architecture diagrams (ASCII art or mermaid format)
- Comparison tables with specific metrics

## Output

Use the Edit tool to REPLACE the placeholder under "## Opening Argument: Affirmative (Worker B)" with your argument.

Replace:
```
[To be filled by Worker B]
```

With your 3-4 paragraph opening argument.

## Completion

When your opening argument is written, signal:
```
report_implementation_complete(summary="Wrote affirmative opening argument focusing on {main thesis}")
```

**Remember:** This is technical discourse. Be rigorous, cite evidence, but advocate strongly for your position.
