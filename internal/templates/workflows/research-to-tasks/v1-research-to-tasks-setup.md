# Setup: Create Plan Document

## Role: Mediator

You are the Mediator responsible for initiating the research-to-tasks planning process.

## Objective

Read the research document and create a structured plan document that will capture the entire planning process.

## Input

- **Research Document:** (provided by user - read from workflow context)

## Output

- **Plan Document:** `docs/proposals/{{ .Date }}--{{ .Name }}/plan.md`

## Instructions

1. **Read the research document thoroughly** to understand what needs to be built

2. **Create the plan document** at `docs/proposals/{{ .Date }}--{{ .Name }}/plan.md` with the structure below

3. **Write a research summary** (2-3 paragraphs) based on what you read

4. **Extract key implementation points** from the research

## Plan Document Structure

Create the document with this exact structure:

```markdown
# Task Plan: {{ .Name }}

## Source

Research document: [path to research document]

## Research Summary

[2-3 paragraphs summarizing what needs to be built based on the research document]

## Key Implementation Points

- [Key point from research]
- [Key point from research]
- [Key point from research]

## Test Integration Philosophy

Every task in this plan includes its tests. We do NOT defer testing.
- Implementation + unit tests = one task
- Integration points get tested when implemented
- No separate "write tests" phase at the end

---

## Epic Created

**Epic ID:** [To be filled by Task Writer]

---

## Initial Task Breakdown

### Epic Structure

[To be filled by Task Writer]

### Tasks

[To be filled by Task Writer]

---

## Implementation Review (Worker 2)

**Verdict:** [APPROVED / CHANGES NEEDED]

**Findings:**

[To be filled by Implementation Reviewer]

---

## Test Review (Worker 3)

**Verdict:** [APPROVED / CHANGES NEEDED]

**Findings:**

[To be filled by Test Reviewer]

---

## Revisions

[To be filled by Task Writer after reviews, if needed]

---

## Final Approval

### Implementation Reviewer

**Status:** Pending
**Comments:**

### Test Reviewer

**Status:** Pending
**Comments:**

---

## Summary

[To be filled by Mediator after all approvals]
```

## Success Criteria

- [ ] Read and understood the research document
- [ ] Created plan document at `docs/proposals/{{ .Date }}--{{ .Name }}/plan.md`
- [ ] Research summary accurately reflects the research document
- [ ] Key implementation points are extracted
- [ ] All placeholder sections are present for subsequent phases
