# Phase 6B: Fix Documentation

You are the **Mediator** addressing feedback from external validation.

## Your Task

Fix any clarity issues identified by the External Validator. Make the documentation understandable to someone new.

## Input

Read the investigation outline at: `{{.Inputs.outline}}`

Find the "External Validation" section and address any issues marked "NEEDS CLARIFICATION".

## Fixing Guidelines

### For Unclear Problem Statements
- Add context about why this matters
- Define any technical terms
- Include a brief example if helpful

### For Unclear Findings
- Add explanatory text between code references
- Explain what each reference shows (not just where it is)
- Connect the dots explicitly

### For Unclear Solutions
- Add a "In Plain English" summary
- Include a brief example of the change
- Make the first step obvious

### For Missing Context
- Add background sections as needed
- Include "Prerequisites" if assumed knowledge exists
- Link to relevant documentation

## Approach

1. Read each issue from External Validation
2. Go to the referenced section
3. Rewrite for clarity
4. Consider: "Would this make sense to a new developer?"

## Output

Update `{{.Inputs.outline}}` with clarified content.

Update the External Validation section:

```markdown
### Fixes Applied

1. {Section} - {What you changed}
2. {Section} - {What you changed}

### Re-validation Status
**Ready for implementation plan:** YES
```

## If No Fixes Needed

If External Validation was "VALIDATED" with no issues:

```markdown
### Fixes Applied
None required - documentation was clear.

### Re-validation Status
**Ready for implementation plan:** YES
```

## Completion

When fixes are complete (or confirmed unnecessary), signal:
```
report_implementation_complete(summary="Documentation {fixed N sections/already clear}. Ready for implementation plan.")
```

**Next:** Create the implementation plan.
