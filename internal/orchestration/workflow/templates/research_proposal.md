---
name: "Research Proposal"
description: "Collaborative research with parallel exploration and structured synthesis for implementation plans"
category: "Planning"
workers: 4
---

# Multi-Agent Research & Proposal Format

## Overview

This document defines the format, roles, and workflow for conducting collaborative research and proposal development using multiple coordinated AI agents. The process combines parallel exploration with structured synthesis to produce comprehensive, well-reasoned implementation plans.

## Roles and Responsibilities

### Coordinator/Synthesizer (Worker 1)
**Responsibilities:**
- Create shared proposal document with problem statement and research questions
- Frame the feature/change and explain why it matters
- Introduce research areas and worker assignments
- Write final proposal synthesis combining all research
- Define clear acceptance criteria and next steps
- Maintain document structure and completeness

**Key Actions:**
1. Create proposal file at start with problem statement
2. Define research questions for each worker
3. Wait for all research phases to complete
4. Read all research findings
5. Write cohesive implementation plan
6. Define acceptance criteria

---

### Research Lead (Worker 2)
**Responsibilities:**
- Conduct primary research on existing codebase patterns
- Identify similar implementations to learn from
- Document technical constraints and dependencies
- Map out affected files and components
- Provide implementation precedents

**Key Actions:**
1. Read proposal file to understand problem statement
2. Use Grep/Glob to find relevant existing code
3. Read key files to understand patterns
4. Document findings with file paths and line numbers
5. Use Edit tool to append under designated section
6. Flag any blocking dependencies or constraints

---

### Architecture Designer (Worker 3)
**Responsibilities:**
- Design implementation approach based on existing patterns
- Identify files/components that need modification
- Propose data structures, interfaces, or APIs
- Consider integration points and side effects
- Provide implementation strategy

**Key Actions:**
1. Read proposal file and research findings
2. Design architecture that fits codebase patterns
3. Break down implementation into logical steps
4. Identify files to create/modify
5. Use Edit tool to append under designated section
6. Include code examples or structure diagrams

---

### Risk/Trade-off Analyst (Worker 4)
**Responsibilities:**
- Identify potential risks and edge cases
- Analyze alternative approaches and trade-offs
- Consider testing requirements and complexity
- Flag performance, security, or maintenance concerns
- Provide balanced perspective on costs vs benefits

**Key Actions:**
1. Read proposal file and all prior research
2. Identify risks, edge cases, and gotchas
3. Compare alternative approaches
4. Document trade-offs and complexity costs
5. Use Edit tool to append under designated section
6. Recommend mitigation strategies

---

## Workflow Phases

### Phase 1: Setup (Coordinator)
**Action**: Create shared proposal document

**Structure:**
```markdown
# Proposal: [Feature Name]

## Problem Statement
[2-3 paragraphs explaining what needs to be built and why]

## Research Questions
### For Research Lead (Worker 2):
- What similar implementations exist in the codebase?
- What patterns should we follow?
- What are the technical constraints?

### For Architecture Designer (Worker 3):
- How should this be structured?
- What files/components need changes?
- What's the implementation strategy?

### For Risk/Trade-off Analyst (Worker 4):
- What are the risks and edge cases?
- What alternative approaches exist?
- What are the trade-offs?

## Research Findings
[Workers will append their findings below]
```

---

### Phase 2: Parallel Research
**Action**: All 3 research workers explore independently

**Order**: Simultaneous (no dependencies between research tasks)

**Important**: Workers use codebase tools (Grep, Glob, Read) during this phase but do NOT write to the file yet. Just explore and build mental models. Prepare 3-5 key findings in working memory to document later.

**Research Goals:**
- Research Lead: Map out patterns, find duplications, identify constraints
- Architecture Designer: Explore consolidation opportunities, design approaches
- Risk Analyst: Identify edge cases, fragile areas, testing gaps

---

### Phase 3: Research Documentation
**Action**: Workers write findings to shared document sequentially

**Order**: Sequential to avoid file conflicts
1. Research Lead writes findings
2. Architecture Designer writes findings
3. Risk/Trade-off Analyst writes findings

**Section Headers:**
```markdown
## Research Findings: Research Lead (Worker 2)
[Detailed findings with file paths, patterns, constraints]

## Research Findings: Architecture Designer (Worker 3)
[Implementation approach, file changes, integration points]

## Research Findings: Risk/Trade-off Analyst (Worker 4)
[Risks, alternatives, trade-offs, mitigation strategies]
```

**Requirements:**
- Include specific file paths and line numbers (e.g., `internal/mode/board.go:145`)
- Cite concrete examples from codebase
- Use code snippets or structure examples where helpful
- Keep findings focused and actionable

---

### Phase 3.5: Research Clarification
**Action**: Workers can ask each other questions before documenting

**Order**: If needed, coordinator mediates clarification discussion

**When to Use:**
- Conflicting findings between workers
- Ambiguous patterns that need discussion
- Technical questions about implementation feasibility

**Process:**
- Workers send clarification questions via coordinator
- Quick back-and-forth to resolve ambiguity
- Helps prevent documentation of incorrect assumptions

---

### Phase 4: Cross-Review
**Action**: Workers read each other's findings and add clarifications

**Order**: Sequential
1. Architecture Designer reads Research Lead's findings, adds notes/concerns
2. Risk Analyst reads both prior findings, adds notes/concerns
3. Research Lead reads all findings, adds final notes

**Section Headers:**
```markdown
## Cross-Review Notes

### Architecture Designer Notes (Worker 3)
[Comments on Research Lead's findings, questions, clarifications]

### Risk Analyst Notes (Worker 4)
[Comments on all prior findings, concerns, gaps]

### Research Lead Notes (Worker 2)
[Final comments, answers to questions, clarifications]
```

**Structured Cross-Review Checklist:**

Each worker reviewing others should address:
- [ ] Any findings that contradict your research?
- [ ] Gaps or areas not covered?
- [ ] Clarifications needed on specific points?
- [ ] Agreement or disagreement with conclusions?
- [ ] Additional risks or opportunities identified?

**Requirements:**
- Focus on gaps, concerns, or questions
- Provide clarifications or additional context
- Keep notes concise (1-2 paragraphs each)
- Be specific about disagreements

---

### Phase 5: Proposal Synthesis (Coordinator)
**Action**: Coordinator reads all research and writes implementation plan

**Order**: After all cross-review completes

**Section Header:**
```markdown
## Implementation Plan

### Overview
[2-3 paragraphs summarizing the approach]

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

### Testing Strategy
- [How this will be tested]
- [What test files need creation/modification]

### Risks and Mitigations
- **Risk**: [identified risk]
  - **Mitigation**: [how to address it]

### Alternative Approaches Considered
- **Approach**: [alternative]
  - **Trade-off**: [why not chosen]
```

---

### Phase 6: Final Review
**Action**: All workers review proposal and flag concerns

**Order**: Sequential
1. Research Lead reviews proposal
2. Architecture Designer reviews proposal
3. Risk Analyst reviews proposal

**Section Header:**
```markdown
## Final Review Comments

### Research Lead Review (Worker 2)
**Status**: [Approved/Concerns]
[Comments on proposal accuracy and completeness]

### Architecture Designer Review (Worker 3)
**Status**: [Approved/Concerns]
[Comments on implementation feasibility]

### Risk Analyst Review (Worker 4)
**Status**: [Approved/Concerns]
[Comments on risk coverage and mitigations]
```

**Requirements:**
- Explicitly state "Approved" or "Concerns"
- If concerns, be specific about what's missing or unclear
- Keep review focused (1-2 paragraphs)

---

### Phase 7: Acceptance Criteria (Coordinator)
**Action**: Coordinator finalizes proposal with acceptance criteria

**Order**: After all final reviews

**Section Header:**
```markdown
## Acceptance Criteria

### Functional Requirements
- [ ] [Specific testable requirement]
- [ ] [Specific testable requirement]

### Technical Requirements
- [ ] [Specific testable requirement]
- [ ] [Specific testable requirement]

### Testing Requirements
- [ ] [Specific test coverage requirement]
- [ ] [Specific test execution requirement]

## Estimated Timeline
- **Phase 1**: [time estimate]
- **Phase 2**: [time estimate]
- **Phase N**: [time estimate]
- **Total**: [total time range]

## Next Steps
1. [Immediate next action]
2. [Follow-up action]
3. [Final action]

## Estimated Scope
- **Complexity**: [Low/Medium/High]
- **Files affected**: [Approximate count]
- **Testing effort**: [Description]
- **Estimated time**: [Total time range]

## Success Metrics
- **Research coverage**: [% of codebase explored or files examined]
- **Cross-review engagement**: [# of concerns raised and addressed]
- **Implementation clarity**: [Can someone else execute this plan?]
- **Risk mitigation**: [All high-risk items have mitigations?]
- **Worker approval**: [All workers approved? Any concerns remaining?]
```

---

## Coordinator Instructions

### Setup Phase
```
1. Spawn 4 workers (coordinator, research lead, architect, risk analyst)
2. Assign coordinator to create proposal file with problem statement
3. Define clear research questions for each worker role
4. Track worker IDs: Worker-1 (coordinator), Worker-2 (research), Worker-3 (architect), Worker-4 (risk)
```

### Execution Phase

**Parallel Research (Workers gather info, don't write yet):**
```
1. Assign Worker-2 (research lead) to explore codebase → wait for completion
2. Assign Worker-3 (architect) to design approach → wait for completion
3. Assign Worker-4 (risk analyst) to identify concerns → wait for completion
```

**Research Documentation (Sequential writes):**
```
CRITICAL: Must be sequential to avoid file conflicts.

1. Assign Worker-2 to write findings → wait for completion
2. Assign Worker-3 to write findings → wait for completion
3. Assign Worker-4 to write findings → wait for completion
```

**Cross-Review (Sequential):**
```
1. Assign Worker-3 to review and add notes → wait for completion
2. Assign Worker-4 to review and add notes → wait for completion
3. Assign Worker-2 to review and add notes → wait for completion
```

**Synthesis (Coordinator):**
```
1. Assign Worker-1 to write implementation plan → wait for completion
```

**Final Review (Sequential):**
```
1. Assign Worker-2 to review proposal → wait for completion
2. Assign Worker-3 to review proposal → wait for completion
3. Assign Worker-4 to review proposal → wait for completion
```

**Acceptance Criteria (Coordinator):**
```
1. Assign Worker-1 to finalize with acceptance criteria → wait for completion
```

---

## Important Notes

### File Management
- Always use single shared file for entire proposal process
- Workers MUST use Read tool before Edit tool
- Each worker appends under their designated section header
- Don't proceed to next phase until current phase completes
- **CRITICAL: File Race Conditions**: NEVER assign multiple workers to write to the same file simultaneously. ALL write phases MUST be sequential.

### Research Quality
- Prefer specific file paths over general descriptions
- Include line numbers when referencing code (e.g., `file.go:123`)
- Use code snippets to illustrate patterns
- Cite multiple examples when patterns vary
- Be explicit about constraints and dependencies

### Proposal Completeness
- Implementation plan should be actionable (ready to execute)
- Acceptance criteria should be testable and specific
- Risks should have concrete mitigation strategies
- Next steps should be clear and ordered
- Timeline estimates provide rough guidance (not commitments)

### Large Proposals (>800 lines)
For complex proposals that exceed ~800 lines, consider splitting into two files:
- `[feature-name]-research.md`: All research findings + cross-review notes
- `[feature-name]-proposal.md`: Implementation plan + acceptance criteria only

This makes the actionable plan easier to reference during implementation while preserving full research context.

---

## Common Pitfalls to Avoid

1. **Don't proceed too fast**: Wait for each phase to complete
2. **Don't skip reading**: Workers must read file before each contribution
3. **Don't be vague**: Use specific file paths, line numbers, examples
4. **Don't ignore cross-review**: It catches gaps and concerns
5. **Don't create file conflicts**: ALL writes must be sequential
6. **Don't skip exploration**: Workers should thoroughly explore before documenting
7. **Don't neglect testing**: Testing strategy is mandatory
8. **Don't fabricate**: If something isn't clear from research, say so

---

## Success Criteria

A successful research & proposal should:
- Present comprehensive research from multiple perspectives
- Cite specific code examples with file paths and line numbers
- Provide actionable implementation plan
- Identify and mitigate risks proactively
- Include clear, testable acceptance criteria
- Consider alternative approaches and explain trade-offs
- Result in single cohesive proposal document
- Be ready for immediate execution
- All workers approved the plan (or concerns documented)
- Cross-review identified and resolved potential issues
- Timeline estimates provided for planning purposes

### Measuring Success

**Research Quality:**
- Did workers explore 70%+ of relevant codebase areas?
- Were specific line numbers and file paths cited?
- Were patterns identified with multiple examples?

**Cross-Review Effectiveness:**
- Were at least 2-3 concerns/clarifications raised?
- Were all concerns addressed in the synthesis?
- Did cross-review catch any bugs or risks?

**Implementation Readiness:**
- Could another developer execute this plan?
- Are all dependencies and prerequisites clear?
- Do acceptance criteria cover all requirements?

**Risk Coverage:**
- Are all high-risk items identified?
- Does each risk have a mitigation strategy?
- Is testing strategy adequate for risk level?

---

## Example Research Questions

### Feature Addition
**Problem**: Add user preferences persistence to settings page

**Research Questions**:
- Research Lead: How does the app currently handle persistence? What storage backend? What existing preference examples?
- Architect: Where should preferences be stored? What data structure? How to integrate with settings UI?
- Risk Analyst: What happens on storage failure? Migration concerns? Performance impact?

### Refactoring
**Problem**: Extract shared modal logic from three different components

**Research Questions**:
- Research Lead: What are the three modals? What's common vs unique? What existing shared components exist?
- Architect: What should the shared component API look like? How to handle customization? Where should it live?
- Risk Analyst: Backward compatibility concerns? Testing strategy? Risk of breaking existing functionality?

### Bug Fix
**Problem**: Fix race condition in background sync process

**Research Questions**:
- Research Lead: Where is the sync process? What's the concurrency pattern? What's the current locking strategy?
- Architect: How to add proper synchronization? What primitives to use? Impact on performance?
- Risk Analyst: Are there other race conditions? How to test this? Could the fix introduce deadlocks?

---

## File Location

Store completed proposals in: `docs/proposals/[feature-name]-proposal.md`

Reference this format document at: `docs/research_proposal_format.md`

---

## Learnings from Production Use

### What Works Exceptionally Well

1. **Parallel Research Phase** - Multiple perspectives exploring simultaneously provides comprehensive coverage that single-agent research misses.

2. **Cross-Review Catches Real Issues** - In the first production run, cross-review identified:
   - A latent bug (`minHeightPerWorker` discrepancy)
   - A helper function that wouldn't work at one call site (different semantics)
   - Defended keeping useful test utilities vs removing them
   - Clarified line count estimates and consolidation priorities

3. **Structured Progression** - The phase-by-phase workflow keeps everyone aligned and prevents workers from jumping ahead.

4. **Final Approval Gate** - Having all 3 workers explicitly approve creates high confidence in the output.

### Observed Metrics (First Production Run)

- **Total time**: ~15 minutes from start to completion
- **Proposal length**: 1,150 lines (comprehensive research + implementation)
- **Coverage**: 13 files analyzed, 5 duplication patterns identified
- **Cross-review value**: 3 significant concerns raised and addressed
- **Implementation readiness**: Resulted in 6 actionable bd tasks with clear dependencies
- **Worker agreement**: 100% approval (all 3 workers approved plan)

### Tips for Coordinators

1. **Don't rush phases** - Wait for completion messages, check message log actively
2. **Enforce sequential writes** - File conflicts will corrupt the proposal
3. **Let cross-review surface disagreements** - The best insights come from constructive friction
4. **Synthesize, don't concatenate** - The coordinator should integrate findings, not just append them
5. **Use the optional clarification phase** - If you see conflicting findings, pause and mediate

### When to Use This Format

**Good for:**
- Feature design and planning (new capabilities)
- Refactoring proposals (cleanup, consolidation)
- Bug fix strategies (complex root causes)
- Architecture decisions (multiple approaches to evaluate)

**Not necessary for:**
- Trivial changes (single-file edits)
- Well-understood patterns (just copy existing code)
- Emergency hotfixes (no time for research)
- Documentation-only changes

### Iteration Ideas

Future improvements to consider:
- Add a "research confidence score" for each finding
- Include code complexity metrics in research phase
- Create visual diagrams of affected components
- Add "test coverage delta" to acceptance criteria
