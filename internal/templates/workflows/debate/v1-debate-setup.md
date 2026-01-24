# Phase 1: Create Debate File

You are the **Moderator** for a structured technical debate.

## Your Task

Create the debate file that will hold all arguments. You are setting up STRUCTURE and framing the topic.

## Proposition

Read the epic description for the full proposition.

## Output

Create the debate file at: `{{.Outputs.debate}}`

## Debate File Template

```markdown
# Technical Debate: {Topic Title}

## Introduction

{2-3 paragraphs explaining:}
- What the topic is and why it matters to the technical community
- The stakes: what decisions hang on this debate
- Why reasonable engineers disagree on this

## Debate Format

- **Worker B (Affirmative)** argues FOR the proposition
- **Worker C (Negative)** argues AGAINST the proposition
- **Worker D (Neutral Analyst)** provides unbiased analysis
- **Worker A (Moderator)** provides closing summary

## Ground Rules

1. **Technical Rigor**: Base arguments on evidence, cite specific examples
2. **Respectful Discourse**: Focus on technical merit, acknowledge valid opposing points
3. **Balanced Advocacy**: Advocate strongly but recognize legitimate opposition use cases
4. **Evidence-Based**: Prefer specific examples over general assertions

---

## Opening Argument: Affirmative (Worker B)

[To be filled by Worker B]

---

## Opening Argument: Negative (Worker C)

[To be filled by Worker C]

---

## Rebuttal: Affirmative (Worker B)

[To be filled by Worker B]

---

## Counter-Rebuttal: Negative (Worker C)

[To be filled by Worker C]

---

## Closing Statement: Affirmative (Worker B)

[To be filled by Worker B]

---

## Closing Statement: Negative (Worker C)

[To be filled by Worker C]

---

## Neutral Analysis (Worker D)

[To be filled by Worker D]

---

## Moderator Closing Summary (Worker A)

[To be filled by Worker A]
```

## Completion

When the debate file is created with introduction and structure, signal:
```
report_implementation_complete(summary="Created debate file with introduction framing '{topic}' and all section placeholders")
```

**IMPORTANT:** You are creating structure only. Do NOT fill in arguments - other workers will do that.
