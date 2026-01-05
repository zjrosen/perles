---
name: "Technical Debate"
description: "Structured multi-perspective debate with moderator, affirmative, negative, and neutral analyst roles"
category: "Analysis"
workers: 4
---

# Multi-Agent Technical Debate Format

## Overview

This document defines the format, roles, and ground rules for conducting structured technical debates using multiple coordinated AI agents. The debate explores opposing technical positions while maintaining objectivity, evidence-based reasoning, and practical wisdom.

## Roles and Responsibilities

### Moderator (Worker A)
**Responsibilities:**
- Create shared debate file with introduction and ground rules
- Frame the debate topic and explain why it matters
- Introduce both debating positions and the neutral analyst
- Write final closing summary synthesizing all perspectives
- Maintain neutrality throughout

**Key Actions:**
1. Create debate file at start
2. Wait for all arguments to complete
3. Read complete debate
4. Write balanced closing summary

---

### Affirmative Position (Worker B)
**Responsibilities:**
- Argue FOR the proposition with technical evidence
- Present opening argument (3-4 paragraphs)
- Write rebuttal to opposition's arguments (2-3 paragraphs)
- Deliver closing statement (2 paragraphs)
- Acknowledge legitimate opposing use cases

**Key Actions:**
1. Read debate file before each contribution
2. Use Edit tool to append under designated section headers
3. Cite real-world examples and concrete technical reasoning
4. Maintain respectful, evidence-based tone

---

### Negative Position (Worker C)
**Responsibilities:**
- Argue AGAINST the proposition with technical evidence
- Present opening argument (3-4 paragraphs)
- Write counter-rebuttal to affirmative's rebuttal (2-3 paragraphs)
- Deliver closing statement (2 paragraphs)
- Acknowledge legitimate opposing use cases

**Key Actions:**
1. Read debate file before each contribution
2. Use Edit tool to append under designated section headers
3. Cite real-world examples and concrete technical reasoning
4. Maintain respectful, evidence-based tone

---

### Neutral Analyst (Worker D)
**Responsibilities:**
- Provide unbiased technical analysis of both positions
- Analyze trade-offs objectively without advocacy
- Identify hidden assumptions and edge cases
- Present decision framework based on context
- Write analysis AFTER both closing statements

**Key Actions:**
1. Read complete debate file (all arguments, rebuttals, closings)
2. Analyze strengths and weaknesses of BOTH positions
3. Identify scenarios where each approach excels
4. Provide decision framework with clear criteria
5. Use Edit tool to append under "## Neutral Analysis (Worker D)"

**Analysis Structure (3-4 paragraphs):**
1. **Strengths of each position**: What both sides got right
2. **Weaknesses and blind spots**: What each side missed or downplayed
3. **Context-dependent guidance**: Clear decision criteria
4. **Hidden trade-offs**: Second-order effects and non-obvious costs

---

## Debate Flow

### Phase 1: Setup (Moderator)
```
## Introduction
[2-3 paragraphs explaining the debate topic, why it matters, and the format]

## Debate Format
- Worker B argues FOR [proposition]
- Worker C argues AGAINST [proposition]
- Worker D provides neutral analysis
- Worker A (moderator) provides closing summary

## Ground Rules
[Technical discourse expectations, evidence requirements, etc.]
```

### Phase 2: Opening Arguments
**Order:**
1. Affirmative (Worker B) writes opening argument
2. Negative (Worker C) writes opening argument

**Section Headers:**
```markdown
## Opening Argument: Affirmative (Worker B)
[3-4 paragraphs with technical reasoning]

## Opening Argument: Negative (Worker C)
[3-4 paragraphs with technical reasoning]
```

### Phase 3: Rebuttals
**Order:**
1. Affirmative (Worker B) writes rebuttal to Negative's opening
2. Negative (Worker C) writes counter-rebuttal to Affirmative's rebuttal

**Section Headers:**
```markdown
## Rebuttal: Affirmative (Worker B)
[2-3 paragraphs responding to opposition]

## Counter-Rebuttal: Negative (Worker C)
[2-3 paragraphs responding to rebuttal]
```

### Phase 4: Closing Statements
**Order:**
1. Affirmative (Worker B) writes closing statement
2. Negative (Worker C) writes closing statement (after Worker B completes)

**Section Headers:**
```markdown
## Closing Statement: Affirmative (Worker B)
[2 paragraphs synthesizing position]

## Closing Statement: Negative (Worker C)
[2 paragraphs synthesizing position]
```

**Closing Statement Requirements:**
1. Reaffirm core value proposition
2. Acknowledge legitimate opposing use cases
3. Provide clear guidance on when to choose each approach
4. End with memorable takeaway

### Phase 5: Neutral Analysis
**Order:**
1. Neutral Analyst (Worker D) reads complete debate
2. Writes objective analysis of both positions

**Section Header:**
```markdown
## Neutral Analysis (Worker D)
[3-4 paragraphs analyzing both sides objectively]

### Key Observations
- Both sides agree on: [list areas of convergence]
- Core disagreement: [identify the fundamental tension]
- Context matters: [explain when each approach wins]

### Decision Framework
[Clear criteria for choosing between approaches]

### Overlooked Considerations
[What neither side fully addressed]
```

### Phase 6: Moderator Summary
**Order:**
1. Moderator (Worker A) reads complete debate including neutral analysis
2. Writes final synthesis

**Section Header:**
```markdown
## Moderator Closing Summary (Worker A)
[2-3 paragraphs synthesizing entire debate]
```

---

## Ground Rules

### 1. Technical Rigor
- Base arguments on concrete evidence, not speculation
- Cite real-world examples with specific systems/companies
- Include performance numbers, benchmarks, or metrics where relevant
- Acknowledge trade-offs honestly
- Use diagrams, code examples, or architecture sketches to illustrate complex points
- Visual aids help clarify abstract concepts and system interactions

### 2. Respectful Discourse
- Focus on technical merit, not rhetoric
- Acknowledge valid points from opposition
- Use phrases like "acknowledges that" not "admits that"
- Frame disagreements as trade-off discussions

### 3. Balanced Advocacy
- Each debater advocates strongly for their position
- But also recognizes legitimate use cases for opposition
- Closing statements must include pragmatic guidance
- Avoid claiming universal superiority

### 4. Evidence-Based Reasoning
- Prefer specific examples over general assertions
- Challenge assumptions with data or logic
- Distinguish between "can it work" and "should it be default"
- Question optimization complexity vs actual business value

### 5. Context Awareness
- Recognize that "it depends" is often the real answer
- Provide decision frameworks, not prescriptive mandates
- Consider team size, scale, domain, and constraints
- Acknowledge when examples are edge cases vs typical scenarios

---

## Coordinator Instructions

### Setup Phase
1. Spawn 4 workers (moderator, affirmative, negative, neutral)
2. Assign moderator to create debate file
3. Choose debate topic and clearly state the proposition
4. Track worker IDs: Worker A (moderator), Worker B (affirmative), Worker C (negative), Worker D (neutral)

### Execution Phase
**Opening Arguments:**
```
1. Assign affirmative worker → wait for completion
2. Assign negative worker → wait for completion
```

**Rebuttals:**
```
1. Assign affirmative worker (rebuttal) → wait for completion
2. Assign negative worker (counter-rebuttal) → wait for completion
```

**Closing Statements:**
```
CRITICAL: Closing statements MUST be sequential, not parallel.
Even though workers write to different section headers, file operations must be one at a time.

1. Assign affirmative worker (closing) → wait for completion
2. Assign negative worker (closing) → wait for completion
```

**Neutral Analysis:**
```
1. Assign neutral analyst AFTER both closings complete
2. Wait for analysis completion
```

**Moderator Summary:**
```
1. Assign moderator AFTER neutral analysis completes
2. Wait for summary completion
```

**Voting Phase (MANDATORY):**
```
1. KEEP Worker A (moderator) - do NOT replace
2. Replace Worker B with fresh unbiased voter → assign voting → wait for completion
3. Replace Worker C with fresh unbiased voter → assign voting → wait for completion
4. Replace Worker D with fresh unbiased voter → assign voting → wait for completion
5. Assign Worker A (moderator) to vote → wait for completion
6. Assign Worker A (moderator) to write voting summary with argument impact analysis → wait for completion
```

**Iterative Refinement Phase (Optional):**
```
1. Recall original Worker B (affirmative) and Worker C (negative) - need their session IDs from earlier
2. Assign Worker B to write 1 paragraph final response → wait for completion
3. Assign Worker C to write 1 paragraph final response → wait for completion
4. Re-vote: Assign all 4 voters sequentially (must explicitly state if vote changed) → wait for each
5. Assign Worker A (moderator) to write final synthesis analyzing vote shifts → wait for completion
```

### Phase 7: Voting Phase (MANDATORY)
**Purpose:** After the debate concludes, fresh unbiased workers vote on which position was more convincing.

**Worker Management:**
```
1. KEEP the original moderator (Worker A) - they have full debate context
2. REPLACE the 3 debating workers (Workers B, C, D) with fresh unbiased voters
3. This creates 4 total voters (1 moderator + 3 fresh) = odd number, no ties possible
```

**Voting Order:**
```
CRITICAL: All voting must be sequential to avoid file conflicts.

1. Replace worker B → assign as Voter 1 → wait for completion
2. Replace worker C → assign as Voter 2 → wait for completion
3. Replace worker D → assign as Voter 3 → wait for completion
4. Assign moderator (Worker A) as Voter 4 → wait for completion
5. Assign moderator (Worker A) to write voting summary → wait for completion
```

**Section Headers:**
```markdown
## Voting Phase

### Vote 1 (Worker-[ID])
**Decision**: [Affirmative/Negative]
[2-3 paragraphs explaining reasoning]

### Vote 2 (Worker-[ID])
**Decision**: [Affirmative/Negative]
[2-3 paragraphs explaining reasoning]

### Vote 3 (Worker-[ID])
**Decision**: [Affirmative/Negative]
[2-3 paragraphs explaining reasoning]

### Vote 4 (Worker A - Moderator)
**Decision**: [Affirmative/Negative]
[2-3 paragraphs explaining reasoning]

### Voting Summary
**Final Vote**: [X-Y in favor of Affirmative/Negative]
[2-3 paragraphs explaining why winning position won, common themes, synthesis]

**Argument Impact Analysis**:
- Most cited argument: [Which argument was mentioned by most voters]
- Minority position's strongest point: [Best argument from losing side that voters acknowledged]
- Consensus areas: [Where voters agreed regardless of their vote]
```

**Voting Requirements:**
- Each voter MUST pick one side (no "both are right" votes)
- Voters must provide 2-3 paragraphs explaining their reasoning
- Voters read the COMPLETE debate before voting
- Voting summary explains WHY the winning position won based on voter reasoning
- **Voting Metadata**: Track which specific arguments from the debate were cited by voters (helps identify which points actually moved opinions)

### Phase 8: Iterative Refinement (Optional)
**Purpose:** After initial voting, allow one round of responses to voter concerns, then re-vote to see if minds changed.

**Worker Management:**
```
1. Recall Worker B (affirmative debater) from retirement
2. Recall Worker C (negative debater) from retirement
3. Keep the 4 voters (Worker A + 3 fresh workers)
```

**Refinement Order:**
```
CRITICAL: All operations sequential to avoid file conflicts.

1. Assign Worker B to write final response (1 paragraph addressing strongest voter concern)
2. Wait for completion
3. Assign Worker C to write final response (1 paragraph addressing strongest voter concern)
4. Wait for completion
5. Re-vote: Assign all 4 voters sequentially to cast new votes
6. Assign Worker A (moderator) to write final synthesis comparing vote shifts
```

**Section Headers:**
```markdown
## Iterative Refinement Phase

### Final Response: Affirmative (Worker B)
[1 paragraph addressing the strongest concern raised by voters]

### Final Response: Negative (Worker C)
[1 paragraph addressing the strongest concern raised by voters]

### Re-Vote 1 (Worker-[ID])
**Decision**: [Affirmative/Negative]
**Changed from initial vote?**: [Yes/No]
[1-2 paragraphs explaining decision and whether final responses changed opinion]

### Re-Vote 2 (Worker-[ID])
[Same format]

### Re-Vote 3 (Worker-[ID])
[Same format]

### Re-Vote 4 (Worker A - Moderator)
[Same format]

### Final Synthesis
**Initial Vote**: [X-Y]
**Final Vote**: [X-Y]
**Vote Shifts**: [How many voters changed positions and why]
[2 paragraphs analyzing what the refinement phase revealed]
```

**Refinement Requirements:**
- Final responses are LIMITED to 1 paragraph each
- Must address the STRONGEST concern from voting summary
- Cannot introduce entirely new arguments
- Re-votes must state if opinion changed from initial vote
- Final synthesis tracks vote shifts and what caused them

### Important Notes
- Always use single shared file for entire debate
- Workers MUST use Read tool before Edit tool
- Each worker appends under their designated section header
- Don't proceed to next phase until current phase completes
- Neutral analyst must read COMPLETE debate before analyzing
- **CRITICAL: File Race Conditions**: NEVER assign multiple workers to write to the same file simultaneously, even if they write to different sections. ALL phases MUST be completely sequential to prevent file conflicts. One worker completes, then the next worker starts.

---

## Learnings from Previous Debates

### What Worked Well
1. **File-based collaboration**: Single shared file keeps all context together
2. **Sequential phases**: Clear ordering prevents workers from missing context
3. **Read-then-edit pattern**: Workers reading file before editing ensures continuity
4. **Specific section headers**: Clear structure makes file navigation easy
5. **Balanced closings**: Both sides converged on pragmatic wisdom naturally
6. **Voting phase**: Fresh unbiased voters provide external validation of arguments
7. **Odd number of voters**: Keeping moderator + 3 fresh workers ensures no ties (4 voters total)

### What Could Improve
1. **Add neutral analyst**: Third unbiased perspective adds depth
2. **Clearer decision frameworks**: Push debaters to provide actionable criteria
3. **Hidden trade-offs**: Explicitly ask for second-order effects
4. **Convergence analysis**: Neutral analyst should highlight where debaters agree
5. **Visual aids**: Encourage diagrams, code examples, and architecture sketches to reduce text-heavy arguments
6. **Concrete examples**: Code snippets showing implementation patterns make abstract arguments tangible
7. **Argument impact tracking**: In voting summary, identify which specific arguments were most cited by voters
8. **Minority dissent highlight**: Give extra attention to minority position's strongest argument
9. **Iterative refinement**: Optional phase where debaters respond to voter concerns and voters re-vote
10. **Vote shift analysis**: Track if and why voters changed positions after final responses

### Common Pitfalls to Avoid
1. **Don't proceed too fast**: Wait for each worker to complete before next phase
2. **Don't skip reading**: Workers must read file before each contribution
3. **Don't allow abstract arguments**: Push for concrete examples and data
4. **Don't let closings be combative**: They should synthesize, not escalate
5. **Don't ignore the neutral analyst**: Their perspective is crucial for balance
6. **Don't create file race conditions**: NEVER assign multiple workers to edit the same file simultaneously under ANY circumstances. ALL file operations must be strictly sequential - one worker completes their edit, then the next worker starts. This prevents file conflicts and ensures each worker sees the complete prior context.

---

## Example Topics

### Software Architecture
- "Microservices are superior to monolithic architecture for most applications"
- "GraphQL is superior to REST for API design"
- "Event-driven architecture is superior to request-response for backend systems"

### Database Design
- "NoSQL databases are superior to SQL for most applications"
- "EAV datastores are superior to relational schema for product catalogs"
- "Document databases are superior to relational for e-commerce systems"

### Development Practices
- "Test-Driven Development (TDD) is superior to test-after development"
- "Trunk-based development is superior to GitFlow"
- "Functional programming is superior to OOP for backend development"

### Infrastructure
- "Kubernetes is superior to serverless for most workloads"
- "Self-hosted infrastructure is superior to managed cloud services"
- "Multi-region deployment is necessary for most SaaS applications"

---

## Template Worker Instructions

### For Moderator (Worker A)
```
You are the MODERATOR for a technical debate.

**Topic**: "[PROPOSITION]"

**Critical**: Create the debate file at `/path/to/debate.md`

Your task:
1. Use Write tool to create the file
2. Write debate introduction (2-3 paragraphs) explaining:
   - What the topic is and why it matters
   - The two opposing positions
   - The neutral analyst role
3. Introduce the format: Worker B argues FOR, Worker C argues AGAINST, Worker D analyzes neutrally
4. Include ground rules for technical, evidence-based discourse

This is the foundation for the entire debate. Begin now.
```

### For Affirmative (Worker B)
```
You are arguing FOR the proposition: "[PROPOSITION]"

**Critical**: Read `/path/to/debate.md` first, then edit to add your [opening/rebuttal/closing].

Your task:
1. Use Read tool to read the debate file
2. Write your [opening argument/rebuttal/closing statement]
3. Use Edit tool to append under "## [Section Header] (Worker B)"

[Specific guidance for this phase]

**Enhance your argument with:**
- Code examples showing implementation patterns
- Architecture diagrams (ASCII art or mermaid format)
- Concrete examples with specific numbers/metrics
- Visual aids to illustrate complex system interactions

This is technical database/architecture design discourse. Begin now.
```

### For Negative (Worker C)
```
You are arguing AGAINST the proposition: "[PROPOSITION]"

**Critical**: Read `/path/to/debate.md` first, then edit to add your [opening/rebuttal/closing].

Your task:
1. Use Read tool to read the debate file
2. Write your [opening argument/counter-rebuttal/closing statement]
3. Use Edit tool to append under "## [Section Header] (Worker C)"

[Specific guidance for this phase]

**Enhance your argument with:**
- Code examples showing simpler alternative approaches
- Architecture diagrams (ASCII art or mermaid format)
- Concrete examples with specific numbers/metrics
- Visual aids to illustrate complexity trade-offs

This is technical database/architecture design discourse. Begin now.
```

### For Neutral Analyst (Worker D)
```
You are providing NEUTRAL ANALYSIS of the debate on: "[PROPOSITION]"

**Critical**: Read `/path/to/debate.md` to see the COMPLETE debate (openings, rebuttals, closings).

Your task:
1. Use Read tool to read the entire debate
2. Analyze BOTH positions objectively without advocacy
3. Identify strengths and weaknesses of each approach
4. Provide clear decision framework based on context
5. Highlight hidden assumptions and overlooked trade-offs
6. Use Edit tool to append under "## Neutral Analysis (Worker D)"

Write 3-4 paragraphs covering:
- What both sides got right (strengths)
- What each side missed or downplayed (weaknesses)
- Context-dependent guidance (decision criteria)
- Hidden trade-offs and second-order effects

You are NOT advocating for either side. Provide balanced, actionable analysis. Begin now.
```

---

## Success Criteria

A successful debate should:
- Present strong technical arguments for both positions
- Cite concrete examples and real-world systems
- Acknowledge legitimate use cases for opposition
- Converge on context-dependent wisdom in closings
- Include neutral analysis that adds perspective neither side provided
- Provide actionable decision frameworks
- Result in single cohesive debate document
- Maintain respectful, evidence-based tone throughout

---

## File Location

Store this format document at: `/path/to/debate_format.md`

Reference it when starting new debates to ensure consistent structure and quality.
