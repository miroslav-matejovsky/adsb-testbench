---
title: "07 - Documentation and final checks"
dependencies: ["06-acceptance-regressions.md"]
effort: "S"
complexity: "low"
---

# Documentation and final checks

## Objective

Synchronize implemented behavior and complete repository validation.

## Target Artifacts

README.md; simulation/doc.go; simulatorapi/doc.go;
docs/backlog/04-observation-history.md; docs/backlog/README.md;
docs/plans/04-observation-history/progress.md.

## Implementation Tasks

1. Update simulation/doc.go with new APIs, fixed retention, station sequence
   semantics, cursor recovery, explicit TTLs, partial fields, and selection.
   Replace existing statements that receptions are returned but not retained.
2. Update root README to describe implemented reception history and diagnostic
   observation snapshots. Retain accurate distinctions for planned runtime,
   transport mapping, and display accumulation.
3. Update simulatorapi/doc.go to describe the concrete observation DTOs and
   wire representation. Explain that backlog 06 owns conversion and HTTP.
4. After all criteria pass, mark backlog 04 completed and remove its link from
   the open backlog index. Keep its file as a completion record so existing
   dependency links from backlog 06 continue to resolve.
5. Run the commands below, review git diff for unrelated formatting/tidy changes,
   and preserve pre-existing edits. Do not commit.
6. Mark steps complete in progress.md only with actual validation evidence.
   If implementation cannot finish, document exact remaining work in .todo.

## Technical Details

Check documentation against public Go APIs with go doc. Do not describe
retained-evidence snapshots as lifetime caches or Config.ID as automatically
unique. The caller owns identity uniqueness across restarts. No schema or
database migration is involved.

## Verification

```text
go doc ./simulation
go doc ./simulatorapi
go test ./simulation ./simulatorapi
task all
git diff --check
git status --short
```

## Acceptance Criteria

task all passes tests, format, vet, architecture checks, and lint. All step
artifacts exist, every acceptance case passes, links resolve, and progress
matches implemented code. No changes are committed or made to vendor.

## Non-Goals

Runtime/API/display implementation from backlog 05-07.

