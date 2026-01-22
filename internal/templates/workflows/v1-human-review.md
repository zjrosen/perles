# Human Review Checkpoint

## Type: Manual Gate

This is a human review checkpoint that pauses workflow execution.

## Purpose

Allow the human operator to:
- Review progress so far
- Provide feedback or corrections
- Approve continuation or request changes
- Mediate conflicting findings between workers

## Coordinator Instructions

**CRITICAL**: Before pausing for human review, you MUST call the `notify_user` tool to alert the user:

```
notify_user(
    message="Human review required: Please review the research findings in the proposal document. Check for conflicting findings between workers and verify the research direction is correct. Reply when ready to continue.",
    phase="clarification-review"
)
```

After calling `notify_user`:
1. Wait for the user to respond
2. Address any feedback or corrections they provide
3. Only proceed to the next phase when the user confirms

## Proposal Location

```
docs/proposals/{{ .Date }}--{{ .Name }}/research-proposal.md
```

## When This Checkpoint Is Used

- After research documentation (check for conflicting findings)
- After proposal synthesis (verify implementation plan makes sense)
- After final reviews (confirm all concerns addressed)

## User Actions

The user may:
- **Continue**: Signal to proceed to next phase
- **Request Clarification**: Ask workers for more detail
- **Provide Correction**: Add human context or corrections
- **Abort**: Stop the workflow if fundamentally off-track

## Notes

This checkpoint is optional in some workflows. It exists to:
- Prevent runaway execution on wrong assumptions
- Allow course correction before expensive phases
- Catch misunderstandings early
