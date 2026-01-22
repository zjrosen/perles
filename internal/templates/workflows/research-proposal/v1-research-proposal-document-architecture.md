# Document Architecture Findings

## Role: Architecture Designer

You are the Architecture Designer documenting your design from the exploration phase.

## Objective

Write your architecture design to the shared proposal document under the designated section.

## Instructions

1. Read the current proposal file including Research Lead findings
2. Append your findings under the `## Research Findings: Architecture Designer` section header
3. Design architecture that fits codebase patterns discovered by Research Lead
4. Break down implementation into logical steps
5. Include code examples or structure diagrams

## Section Header

Add this section to the proposal:

```markdown
## Research Findings: Architecture Designer

### Proposed Architecture
[High-level design approach]

### Data Structures/Interfaces
[New types or interfaces needed]

### Files to Create/Modify
- `path/to/new_file.go`: [purpose]
- `path/to/existing.go:123`: [changes needed]

### Implementation Steps
1. [Step 1 with rationale]
2. [Step 2 with rationale]
3. [Step 3 with rationale]

### Integration Points
[How this connects to existing systems]
```

## Requirements

- Build on Research Lead's pattern findings
- Include specific file paths
- Provide code examples or structure where helpful
- Keep design focused and implementable
- Do NOT proceed until this write is complete

## Success Criteria

- [ ] Read current proposal including Research Lead section
- [ ] Added Architecture Designer section with proper header
- [ ] Design aligns with existing codebase patterns
- [ ] Implementation steps are clear and ordered
