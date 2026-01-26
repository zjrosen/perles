# Finalize Proposal

## Role: Coordinator/Synthesizer

You are the Coordinator finalizing the proposal with acceptance criteria.

## Objective

Add final acceptance criteria and signal workflow completion.

## Instructions

1. Read all final approvals from workers
2. Address any remaining concerns
3. Add the Acceptance Criteria section
4. Signal workflow complete

## Section Header

Add this section to the proposal:

```markdown
## Acceptance Criteria

### Functional Requirements
- [ ] [Specific testable requirement 1]
- [ ] [Specific testable requirement 2]
- [ ] [Specific testable requirement 3]

### Non-Functional Requirements
- [ ] [Performance requirement with metric]
- [ ] [Security requirement]
- [ ] [Maintainability requirement]

### Testing Requirements
- [ ] Unit tests pass with >X% coverage
- [ ] Integration tests for [specific scenarios]
- [ ] Manual testing verifies [specific behaviors]

### Documentation Requirements
- [ ] Code comments for complex logic
- [ ] README updated (if needed)
- [ ] API documentation (if applicable)

## Success Metrics
- **Research coverage**: [% of codebase explored or files examined]
- **Cross-review engagement**: [# of concerns raised and addressed]
- **Implementation clarity**: [Can someone else execute this plan?]
- **Risk mitigation**: [All high-risk items have mitigations?]
- **Worker approval**: [All workers approved? Any concerns remaining?]

## Next Steps
1. [First action to take]
2. [Second action to take]
3. [Third action to take]

---
*Proposal completed on {{ .Date }}*
*All workers: [Approval status summary]*
```

## Workflow Completion

After finalizing acceptance criteria, signal completion:

```
signal_workflow_complete(
    status="success",
    summary="Completed research proposal for {{ .Name }}. All workers approved. Proposal at {{.Config.document_path}}/{{ .Name }}-proposal.md with [count] acceptance criteria defined."
)
```

## Success Criteria

- [ ] All worker approvals reviewed
- [ ] Remaining concerns addressed or documented
- [ ] Acceptance criteria are specific and testable
- [ ] Next steps are clear and actionable
- [ ] Workflow signaled complete
