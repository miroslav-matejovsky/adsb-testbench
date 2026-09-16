# Plans

This directory contains implementation plans. A plan is an executable specification, not a discussion document.

Each plan has:
- `README.md` describing goal, scope, architecture impact, deliverables, and success criteria
- `assessment.md` describing feasibility, risks, dependencies, validation, and rollback
- numbered implementation steps (`01-*.md`, `02-*.md`, ...)

Each step contains YAML front matter:

```yaml
title: "01 - Step title"
dependencies: []
effort: "S|M|L|XL"
complexity: "low|medium|high|very_high"
```

Each step must contain:
- **Objective**: exact outcome of the step
- **Target Artifacts**: files, modules, APIs, schemas, configurations, and tests to modify
- **Implementation Tasks**: ordered, concrete development actions
- **Technical Details**: classes, methods, interfaces, contracts, migrations, and configuration changes
- **Verification**: exact commands and validation activities
- **Acceptance Criteria**: binary, objectively testable conditions
- **Non-Goals**: what is explicitly not implemented

Requirements:
- Every task must describe **what to change, where to change it, and how it will be verified**
- Reference specific files, APIs, classes, methods, database objects, and tests whenever possible
- Define implementation order explicitly through dependencies
- Include required unit, integration, and regression tests or justify their absence
- Acceptance criteria must be measurable and independently verifiable

Forbidden:
- Vague work items such as "improve", "enhance", "optimize", "refactor", or "investigate"
- Open-ended research tasks
- Future considerations, ideas, or alternatives
- Statements without implementation details
- Acceptance criteria that require subjective judgment

A plan is complete only when all implementation tasks are finished, all tests pass, all acceptance criteria are verified, and the deliverables exist in the codebase.
