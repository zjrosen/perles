# Mediated Investigation: {{.Slug}}

## Overview

This is a rigorous 6-worker investigation workflow designed to produce the highest quality investigation possible. The workflow includes adversarial validation (devil's advocate, counter-investigation), parallel sub-agent reviews, and external validation.

**Philosophy:** Quality over speed. Every conclusion is challenged, every alternative is explored, every reference is verified.

## Problem Statement

{{.Args.problem}}

## Worker Roles

| Worker | Role | Responsibility |
|--------|------|----------------|
| worker-1 | Mediator | Lead investigation, create structure, fix issues, synthesize results |
| worker-2 | Researcher | Explore codebase (broad then deep), address challenges |
| worker-3 | Devil's Advocate | Challenge findings, question assumptions |
| worker-4 | Counter-Researcher | Try to prove the conclusion is WRONG |
| worker-5 | Reviewer | Parallel sub-agent review (3 dimensions) |
| worker-6 | External Validator | Fresh eyes - test documentation clarity |

## Workflow Phases

```
Phase 1:  worker-1 (Mediator)           → Create outline + hypothesis list
Phase 2A: worker-2 (Researcher)         → Broad exploration with confidence scores
Phase 2B: worker-2 (Researcher)         → Deep dive on high-confidence areas
Phase 3:  worker-3 (Devil's Advocate)   → Challenge findings, question assumptions
Phase 3B: worker-2 (Researcher)         → Address challenges
Phase 4:  worker-4 (Counter-Researcher) → Try to prove conclusion is WRONG
Phase 5:  worker-5 (Reviewer)           → Parallel sub-agent review (3 dimensions)
Phase 6:  worker-6 (External Validator) → Fresh eyes validation
Phase 6B: worker-1 (Mediator)           → Fix documentation if needed
Phase 7:  worker-1 (Mediator)           → Synthesize into implementation plan
```

## Quality Gates

| Gate | Pass Condition | Failure Action |
|------|----------------|----------------|
| Devil's Advocate | All challenges addressed | Researcher revises |
| Counter-Investigation | Cannot prove alternative | Back to research |
| Review Sub-Agents | All 3 approve | Address specific issues |
| External Validation | Fresh worker understands | Mediator fixes docs |

**CRITICAL:** Do NOT proceed past a quality gate until it passes.

## Output Artifacts

- `{{.Outputs.outline}}` - Investigation outline with findings
- `{{.Outputs.plan}}` - Implementation plan (created in final phase)

## Execution Instructions

1. **Spawn all 6 workers** at the start - they will be used across phases
2. **Follow phase order strictly** - dependencies enforce quality
3. **Quality gates are mandatory** - do not skip or rush past them
4. **Mark tasks complete immediately** when workers signal completion
5. **Use `bd ready --parent <epic-id>`** to see which tasks are unblocked

## Success Criteria

- [ ] All hypotheses marked CONFIRMED or RULED OUT with evidence
- [ ] Devil's Advocate challenges addressed
- [ ] Counter-investigation could not disprove conclusion
- [ ] All 3 review sub-agents approved
- [ ] External validator understood the documentation
- [ ] Implementation plan based on validated findings
- [ ] High confidence in proposed solution
