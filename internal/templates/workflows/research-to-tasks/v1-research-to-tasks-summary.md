# Summary: Complete the Planning Workflow

## Role: Mediator

You are the Mediator completing the research-to-tasks planning workflow.

## Objective

Write the final summary synthesizing the planning process and confirming the epic is ready for implementation.

## Input

- **Plan Document:** `docs/proposals/{{ .Date }}--{{ .Name }}/plan.md`

## Instructions

1. **Read the entire plan document** to understand what was accomplished

2. **Verify both reviewers approved** in the "## Final Approval" section:
   - If either shows NOT APPROVED, flag for coordinator
   - Do not write summary if approvals are missing

3. **Get the epic ID** from "## Epic Created" section

4. **Count the tasks** using `bd list --parent {epic-id}`

5. **Write the final summary**

## Document Your Summary

Use the Edit tool to update "## Summary" in the plan document:

```markdown
## Summary

### Planning Outcome

**Status:** APPROVED AND READY

### Epic Created

- **ID:** `{epic-id}`
- **Title:** {{ .Name }}
- **Tasks:** {count} tasks

### Key Decisions Made

- [Decision 1 from planning process]
- [Decision 2 from planning process]
- [Decision 3 from planning process]

### Reviewer Consensus

- **Implementation:** APPROVED - {brief summary of what was verified}
- **Test Coverage:** APPROVED - {brief summary of what was verified}

### Test Integration Summary

All tasks include their tests as part of the implementation. No deferred testing.
- {count} tasks with embedded test requirements
- Edge cases identified and covered
- Test dependencies properly ordered

### Ready for Execution

The epic and tasks are ready for implementation. Each task includes:
- Clear implementation steps
- Specific test requirements
- Proper dependencies

### Next Steps

1. Use `/cook` or `/pickup` to begin implementation
2. Each task should be completed with its tests before moving on
3. See research doc for detailed context (path in plan document's Source section)

---

*Plan completed: {{ .Date }}*
```

## If Approvals Missing

If either reviewer has NOT APPROVED status:

```markdown
## Summary

### Planning Outcome

**Status:** NEEDS ADDITIONAL WORK

**Blocking Issues:**
- [Which reviewer did not approve and why]

**Action Required:**
Coordinator must trigger additional revision cycle.
```

Then notify the coordinator that workflow cannot complete.

## Success Criteria

- [ ] Read the entire plan document
- [ ] Verified both reviewers approved
- [ ] Epic ID and task count documented
- [ ] Key decisions summarized
- [ ] Summary accurately reflects the planning process
- [ ] Next steps are clear
