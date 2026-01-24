# Technical Debate: {{.Slug}}

## Overview

This is a structured multi-perspective technical debate designed to explore opposing positions while maintaining objectivity, evidence-based reasoning, and practical wisdom.

**Philosophy:** Present strong arguments for both sides, cite concrete examples, acknowledge legitimate opposing use cases, and converge on context-dependent wisdom.

## Proposition

{{.Args.proposition}}

## Worker Roles

| Worker | Role | Responsibility |
|--------|------|----------------|
| worker-1 | Moderator | Create debate file, frame topic, write closing summary, vote, write voting summary |
| worker-2 | Affirmative | Argue FOR the proposition with technical evidence |
| worker-3 | Negative | Argue AGAINST the proposition with technical evidence |
| worker-4 | Neutral Analyst | Provide unbiased analysis of both positions |
| worker-5 | Voter 1 | Fresh unbiased voter (replaces affirmative context) |
| worker-6 | Voter 2 | Fresh unbiased voter (replaces negative context) |
| worker-7 | Voter 3 | Fresh unbiased voter (replaces neutral context) |

## Debate Phases

```
Phase 1:  worker-1 (Moderator)     → Create debate file with introduction
Phase 2A: worker-2 (Affirmative)   → Opening argument (3-4 paragraphs)
Phase 2B: worker-3 (Negative)      → Opening argument (3-4 paragraphs)
Phase 3A: worker-2 (Affirmative)   → Rebuttal to negative (2-3 paragraphs)
Phase 3B: worker-3 (Negative)      → Counter-rebuttal (2-3 paragraphs)
Phase 4A: worker-2 (Affirmative)   → Closing statement (2 paragraphs)
Phase 4B: worker-3 (Negative)      → Closing statement (2 paragraphs)
Phase 5:  worker-4 (Neutral)       → Neutral analysis (3-4 paragraphs)
Phase 6:  worker-1 (Moderator)     → Closing summary (2-3 paragraphs)
Phase 7A: worker-5 (Voter 1)       → Cast vote with reasoning
Phase 7B: worker-6 (Voter 2)       → Cast vote with reasoning
Phase 7C: worker-7 (Voter 3)       → Cast vote with reasoning
Phase 7D: worker-1 (Moderator)     → Cast moderator vote
Phase 7E: worker-1 (Moderator)     → Write voting summary with argument impact analysis
```

## Ground Rules

### Technical Rigor
- Base arguments on concrete evidence, not speculation
- Cite real-world examples with specific systems/companies
- Include performance numbers, benchmarks, or metrics where relevant
- Acknowledge trade-offs honestly
- Use diagrams, code examples, or architecture sketches to illustrate

### Respectful Discourse
- Focus on technical merit, not rhetoric
- Acknowledge valid points from opposition
- Use phrases like "acknowledges that" not "admits that"
- Frame disagreements as trade-off discussions

### Balanced Advocacy
- Each debater advocates strongly for their position
- But also recognizes legitimate use cases for opposition
- Closing statements must include pragmatic guidance
- Avoid claiming universal superiority

### Evidence-Based Reasoning
- Prefer specific examples over general assertions
- Challenge assumptions with data or logic
- Distinguish between "can it work" and "should it be default"

## Output Artifacts

- `{{.Outputs.debate}}` - Complete debate document with all arguments

## Execution Instructions

1. **Spawn all 4 workers** at the start - they will be used across phases
2. **Follow phase order strictly** - workers must read before writing
3. **Sequential file writes** - NEVER assign multiple workers to write simultaneously
4. **Mark tasks complete immediately** when workers signal completion
5. **Use `bd ready --parent <epic-id>`** to see which tasks are unblocked

## Critical: File Race Prevention

**ALL phases MUST be completely sequential to prevent file conflicts.**

Workers MUST:
1. Use Read tool before Edit tool
2. Wait for previous phase to complete before starting
3. Append under their designated section headers

## Success Criteria

- [ ] Strong technical arguments presented for both positions
- [ ] Concrete examples and real-world systems cited
- [ ] Opposition use cases acknowledged by both sides
- [ ] Context-dependent wisdom in closing statements
- [ ] Neutral analysis adds perspective neither side provided
- [ ] Actionable decision frameworks included
- [ ] Respectful, evidence-based tone maintained throughout
