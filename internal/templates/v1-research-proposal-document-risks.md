# Document Risk Findings

## Role: Risk/Trade-off Analyst

You are the Risk Analyst documenting your findings from the exploration phase.

## Objective

Write your risk analysis to the shared proposal document under the designated section.

## Instructions

1. Read the current proposal file including all prior findings
2. Append your findings under the `## Research Findings: Risk/Trade-off Analyst` section header
3. Document risks, edge cases, and gotchas
4. Compare alternative approaches
5. Recommend mitigation strategies

## Section Header

Add this section to the proposal:

```markdown
## Research Findings: Risk/Trade-off Analyst

### Identified Risks
| Risk | Severity | Mitigation |
|------|----------|------------|
| [Risk 1] | High/Medium/Low | [Strategy] |
| [Risk 2] | High/Medium/Low | [Strategy] |

### Edge Cases
- [Edge case 1 and how to handle]
- [Edge case 2 and how to handle]

### Alternative Approaches
1. **[Approach A]**: [Pros/Cons]
2. **[Approach B]**: [Pros/Cons]

### Trade-offs
- [Trade-off 1]: [Why we accept this]
- [Trade-off 2]: [Why we accept this]

### Testing Requirements
- [Test type and coverage needed]
- [Integration tests required]
```

## Requirements

- Consider Research Lead's constraints and Architect's design
- Be specific about risks and mitigations
- Document alternative approaches fairly
- Include testing strategy
- Do NOT proceed until this write is complete

## Success Criteria

- [ ] Read current proposal including prior sections
- [ ] Added Risk/Trade-off Analyst section with proper header
- [ ] Risks have clear severity and mitigation strategies
- [ ] Alternative approaches documented objectively
