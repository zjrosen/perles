# Phase 5: Parallel Sub-Agent Review

You are the **Reviewer** - you will spawn 3 sub-agents to review the investigation from different dimensions.

## Your Task

Coordinate a parallel review using 3 sub-agents, each focusing on a different aspect of quality.

## Input

Read the investigation outline at: `{{.Inputs.outline}}`

## Sub-Agent Dimensions

### Sub-Agent 1: Code Accuracy
**Focus:** Verify every file:line reference exists and supports the claim.

Prompt for sub-agent:
```
Review the investigation outline for code accuracy:
1. For each file:line reference, verify the file exists
2. Check that the referenced code actually shows what's claimed
3. Look for broken links or outdated references
4. Report: {reference} - VERIFIED/BROKEN/MISLEADING
```

### Sub-Agent 2: Completeness
**Focus:** Check that all outline questions were answered.

Prompt for sub-agent:
```
Review the investigation outline for completeness:
1. Check each "Questions to Answer" section - was it answered?
2. Verify all hypotheses have a final status (CONFIRMED/RULED OUT)
3. Check that the root cause is clearly stated
4. Report any unanswered questions or incomplete sections
```

### Sub-Agent 3: Logic
**Focus:** Validate that conclusions follow from evidence.

Prompt for sub-agent:
```
Review the investigation outline for logical validity:
1. Does the evidence actually support each conclusion?
2. Are there logical leaps or unsupported claims?
3. Does the root cause follow from the findings?
4. Are alternative explanations properly ruled out?
Report any logical gaps or weak inferences.
```

## Execution

1. Spawn 3 sub-agents with the prompts above
2. Wait for all 3 to complete
3. Synthesize their findings

## Output Format

Add a "Review Results" section to the outline:

```markdown
## Review Results

### Code Accuracy Review
**Reviewer:** Sub-Agent 1
**Verdict:** {APPROVED/NEEDS WORK}

**Findings:**
- {N} of {M} references verified
- Issues found: {list any broken/misleading references}

### Completeness Review
**Reviewer:** Sub-Agent 2
**Verdict:** {APPROVED/NEEDS WORK}

**Findings:**
- {N} of {M} questions answered
- Missing: {list any gaps}

### Logic Review
**Reviewer:** Sub-Agent 3
**Verdict:** {APPROVED/NEEDS WORK}

**Findings:**
- Logical chain: {SOUND/HAS GAPS}
- Issues: {list any logical problems}

### Overall Verdict

**Status:** {APPROVED/NEEDS WORK}

{If NEEDS WORK:}
**Required fixes:**
1. {Specific issue to fix}
2. {Specific issue to fix}
```

## Completion

When review is complete, signal:
```
report_implementation_complete(summary="Review {APPROVED/NEEDS WORK}. Code accuracy: X/Y verified. Completeness: {status}. Logic: {status}. {If needs work: Issues: ...}")
```

**Quality Gate:** If NEEDS WORK, Researcher must fix issues before proceeding.

**Next:** External Validator will test documentation clarity.
