# Proposal Synthesis

## Role: Coordinator/Synthesizer

You are the Coordinator synthesizing all research into a cohesive implementation plan.

## Objective

Read all research findings and cross-review notes, then write the final implementation plan.

## Instructions

1. Read the entire proposal including all research and cross-review notes
2. Synthesize (don't concatenate) findings into a cohesive plan
3. Resolve any remaining disagreements
4. Add the Implementation Plan section

## Section Header

Add this section to the proposal:

```markdown
## Implementation Plan

### Overview
[2-3 paragraphs summarizing the approach, incorporating insights from all workers]

### Out of Scope / Intentionally Not Changed
- [Area/component being left alone and why]
- [Risky area workers recommend not touching]
- [Complexity that's inherent to the feature]

### Files to Create/Modify
- `path/to/file1.go`: [what changes and why]
- `path/to/file2.go`: [what changes and why]

### Implementation Steps
1. [Step 1 with rationale]
2. [Step 2 with rationale]
3. [Step 3 with rationale]

### Dependencies
- [Any blocking dependencies or prerequisites]

### Risk Mitigations
- [Risk 1]: [How we're handling it]
- [Risk 2]: [How we're handling it]

### Testing Strategy
- **Unit tests**: [What needs unit testing]
- **Integration tests**: [What needs integration testing]
- **Manual testing**: [What needs manual verification]

### Complexity Estimate
- **Files affected**: [Approximate count]
- **Testing effort**: [Description]
- **Estimated time**: [Total time range]
```

## Synthesis Guidelines

- **Integrate, don't append**: The plan should feel like one coherent document
- **Resolve conflicts**: If workers disagreed, explain the chosen resolution
- **Address concerns**: All cross-review concerns should be addressed
- **Be specific**: Use file paths, step-by-step instructions

## Success Criteria

- [ ] Read all research and cross-review notes
- [ ] Implementation Plan synthesizes all perspectives
- [ ] Conflicts resolved with rationale
- [ ] Plan is specific and actionable
- [ ] Testing strategy is comprehensive
