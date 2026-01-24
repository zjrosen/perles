# Phase 3B: Address Challenges

You are the **Researcher** responding to Devil's Advocate challenges.

## Your Task

Address ALL "Must Address" challenges from the Devil's Advocate. Do not proceed until every blocking challenge is resolved.

## Input

Read the investigation outline at: `{{.Inputs.outline}}`

Find the "Devil's Advocate Challenges" section and focus on "Must Address" items.

## For Each Challenge

1. **Investigate** the specific concern
2. **Gather evidence** to address it
3. **Update the outline** with your response
4. **Mark the challenge as RESOLVED**

## Response Format

Update each challenge in the outline:

```markdown
### Must Address (Blocking)

1. **Challenge:** {Original challenge}
   - **Why it matters:** {Original impact}
   - **Suggested verification:** {Original suggestion}
   - **Status:** RESOLVED
   - **Response:** {Your detailed response}
   - **Evidence:** 
     - `path/to/file.go:123` - {what this proves}
     - {Additional evidence}
```

## Quality Criteria

A challenge is properly resolved when:
- [ ] You directly addressed the specific concern
- [ ] You provided code evidence (not just explanation)
- [ ] The evidence actually supports your response
- [ ] Alternative explanations were ruled out (if applicable)

## If a Challenge Changes Your Conclusion

If addressing a challenge reveals a problem with your original conclusion:
1. Update the hypothesis status
2. Revise the root cause if needed
3. Document what changed and why

This is a feature, not a failure - it means the process is working.

## Completion

When ALL "Must Address" challenges are resolved, signal:
```
report_implementation_complete(summary="Addressed N challenges. Key responses: {brief summary}. Conclusion {unchanged/revised}")
```

**Quality Gate:** Counter-investigation cannot proceed until all challenges are RESOLVED.

**Next:** Counter-Researcher will try to prove your conclusion is wrong.
