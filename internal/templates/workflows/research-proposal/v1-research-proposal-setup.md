# Setup: Create Proposal Directory and Document

## Role: Coordinator/Synthesizer

You are the Coordinator responsible for initiating the research proposal process.

## Goal

{{.Args.goal}}

## Objective

Create the proposal directory and shared proposal document with problem statement and research questions for the team.

## Instructions

1. **Create the proposal directory** at `{{.Config.document_path}}/{{ .Date }}--{{ .Name }}/`
   - Format: `YYYY-MM-DD--feature-name` (e.g., `2026-01-10--workflow-templates`)
   - This keeps proposals organized chronologically and by feature

2. **Create the proposal file** at `{{.Config.document_path}}/{{ .Date }}--{{ .Name }}/research-proposal.md`

3. Write a clear problem statement (2-3 paragraphs explaining what needs to be built and why)

4. Frame the feature/change and explain why it matters

5. Define specific research questions for each worker role

## Directory Structure

```
{{.Config.document_path}}/
└── {{ .Date }}--{{ .Name }}/
    └── research-proposal.md
```

## Output Structure

Create the proposal with this structure:

```markdown
# Proposal: {{ .Name }}

## Problem Statement
[2-3 paragraphs explaining what needs to be built and why]

## Research Questions

### For Research Lead:
- What similar implementations exist in the codebase?
- What patterns should we follow?
- What are the technical constraints?

### For Architecture Designer:
- How should this be structured?
- What files/components need changes?
- What's the implementation strategy?

### For Risk/Trade-off Analyst:
- What are the risks and edge cases?
- What alternative approaches exist?
- What are the trade-offs?

## Research Findings
[Workers will append their findings below]
```

## Success Criteria

- [ ] Proposal directory created at `{{.Config.document_path}}/{{ .Date }}--{{ .Name }}/`
- [ ] Proposal file created at `{{.Config.document_path}}/{{ .Date }}--{{ .Name }}/research-proposal.md`
- [ ] Problem statement clearly articulates the need
- [ ] Research questions are specific and actionable for each role
- [ ] Document structure follows the template
