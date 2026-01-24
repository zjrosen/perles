# Phase 7: Create Implementation Plan

You are the **Mediator** synthesizing the validated investigation into an actionable plan.

## Your Task

Create an implementation plan based on the validated investigation findings. This plan should be immediately actionable.

## Input

Read the investigation outline at: `{{.Inputs.outline}}`

## Output

Create the implementation plan at: `{{.Outputs.plan}}`

## Plan Template

```markdown
# Implementation Plan: {Problem Title}

## Executive Summary

### Problem
{One paragraph summary of the problem}

### Root Cause
{One paragraph summary of the validated root cause}

### Solution
{One paragraph summary of the proposed solution}

### Confidence
**Level:** HIGH (validated through adversarial review)
- Devil's Advocate challenges: ADDRESSED
- Counter-investigation: PASSED (could not disprove)
- Review sub-agents: APPROVED
- External validation: VALIDATED

---

## Investigation Summary

### Hypotheses Tested
| ID | Hypothesis | Result | Evidence |
|----|------------|--------|----------|
| H1 | {hypothesis} | CONFIRMED | {brief evidence} |
| H2 | {hypothesis} | RULED OUT | {brief evidence} |

### Alternatives Ruled Out
- {Alternative 1} - {why ruled out}
- {Alternative 2} - {why ruled out}

### Root Cause
{Clear statement of the root cause with confidence level}

### Validation Status
- [x] Devil's Advocate challenges addressed
- [x] Counter-investigation passed
- [x] Review sub-agents approved
- [x] External validation passed

---

## Proposed Solution

### Approach
{Solution strategy with rationale}

### Files to Modify
| File | Changes | Risk |
|------|---------|------|
| `path/to/file.go` | {changes} | Low/Med/High |

---

## Implementation Steps

### Step 1: {Title}
{Description with code example if helpful}

**Verification:** {How to verify this step worked}

### Step 2: {Title}
{Description}

**Verification:** {How to verify}

### Step 3: {Title}
...

---

## Testing Strategy

### Unit Tests
- [ ] `TestName_Scenario` - {description}

### Integration Tests
- [ ] `TestName_Flow` - {description}

### Manual Verification
- [ ] {Step to manually verify}

---

## Risks and Mitigations

### Risk 1: {Title}
{Description}
**Mitigation:** {How to address}

---

## Estimated Complexity

**Scope:** {Small/Medium/Large}
- Test additions: ~{X} lines
- Production changes: ~{Y} lines
- Confidence: HIGH (validated through counter-investigation)

---

## Success Criteria

1. {Specific criterion}
2. {Specific criterion}
3. All tests pass
4. No regressions
```

## Quality Criteria

The plan should:
- [ ] Be immediately actionable (no more research needed)
- [ ] Include specific file paths and line numbers
- [ ] Have clear verification steps
- [ ] Reference the validated investigation findings

## Important Notes

- **Synthesize, don't copy** - The plan should distill the investigation, not duplicate it
- **Be specific** - Include code examples where helpful
- **Be realistic** - Estimates should reflect actual complexity
- **Reference validation** - The plan's strength is that it's been validated

## Completion

When the plan is complete, signal:
```
report_implementation_complete(summary="Created implementation plan. Scope: {Small/Medium/Large}. Steps: N. Tests: M. Ready for execution.")
```

**Workflow Complete:** After this phase, the coordinator will close the workflow.
