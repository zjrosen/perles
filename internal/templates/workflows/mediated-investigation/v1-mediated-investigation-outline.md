# Phase 1: Create Investigation Outline

You are the **Mediator** for a high-quality investigation workflow.

## Your Task

Create an investigation outline that will guide the research team. You are creating STRUCTURE, not doing research.

## Problem to Investigate

Read the epic description for the full problem statement.

## Output

Create the outline at: `{{.Outputs.outline}}`

## Outline Template

```markdown
# Investigation: {Problem Title}

## Problem Statement

{2-3 paragraphs describing the problem and why it matters}

## Initial Hypotheses

List possible explanations to investigate:
- [ ] **H1:** {First hypothesis} - Confidence: TBD
- [ ] **H2:** {Second hypothesis} - Confidence: TBD
- [ ] **H3:** {Third hypothesis} - Confidence: TBD

---

## 1. {First Area to Investigate}

### Questions to Answer
- {Specific question}
- {Specific question}

### Files/Functions to Examine
- [ ] {Suggested file or pattern} (`path/to/file.go`)
- [ ] {Suggested file or pattern}

### Findings
[To be filled by Researcher]

### Confidence Score
[To be filled by Researcher: High/Medium/Low with rationale]

---

## 2. {Second Area to Investigate}
[Same structure...]

---

## N. Gap Analysis / Root Cause

### Questions to Answer
- At what point do the two paths diverge?
- Is the data available but not used correctly?
- What's the root cause of the issue?

### Comparison Points
- [ ] Compare X between the two paths
- [ ] Identify any hardcoded assumptions

### Findings
[To be filled by Researcher]

---

## Key Files and Functions Summary

### Primary Files
| File | Purpose | Examined |
|------|---------|----------|
| `path/to/file.go` | {purpose} | [ ] |

### Key Functions
| Function | File | Purpose |
|----------|------|---------|
| {name} | {file} | {purpose} |

---

## Investigation Output

### Hypothesis Status
| ID | Hypothesis | Status | Confidence | Evidence |
|----|------------|--------|------------|----------|
| H1 | {hypothesis} | TBD | TBD | TBD |

### Root Cause
{To be filled after research}

### Code References
| File | Line(s) | Function/Purpose |
|------|---------|------------------|
| `path/to/file.go` | 123-145 | {description} |

### Solution Options
**OPTION A: {Description}**
- {Details}
- Confidence: {High/Medium/Low}

**OPTION B: {Description}** (if applicable)
- {Details}
- Confidence: {High/Medium/Low}
```

## Completion

When the outline is ready, signal completion:
```
report_implementation_complete(summary="Created investigation outline with N hypotheses and M areas to investigate")
```

**IMPORTANT:** You are creating structure only. Do NOT do research or fill in findings.
