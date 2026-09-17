---
title: "03 - Cursor paging"
dependencies: ["02-retained-receptions.md"]
effort: "M"
complexity: "high"
---

# Cursor paging

## Objective

Expose resumable single-station reception pages with precise gap handling.

## Target Artifacts

new simulation/reception_query.go and simulation/reception_query_test.go;
simulation/reception_history.go; simulation/types.go.

## Implementation Tasks

1. Implement Engine.ReceptionHistory(ctx, HistoryRequest) (ReceptionPage, error)
   under the engine mutex with entry and final cancellation checks.
2. Validate limit and cursor shape. Cursor StationID must equal request StationID;
   mismatched scope is ErrInvalid. Mismatched RunID is ErrConflict. An unknown
   or removed requested station is ErrNotFound. A future AfterSequence above
   that station's latest issued sequence is ErrInvalid.
3. With nil cursor, return the oldest retained records without Gap. With a
   cursor, return records strictly after AfterSequence. Set Gap when records
   after that position were evicted, using overflow-safe subtraction:
   nonempty ring and AfterSequence < OldestSequence - 1.
4. Return up to Limit records. NextCursor names the last returned reception,
   or preserves the requested AfterSequence when no record is returned; for
   a fresh empty read use 0. HasMore means additional records existed at this
   captured read instant. A gap page starts at the oldest retained record.
5. Capture Now, bounds, records, and cursor from one lock acquisition. Deep-copy
   output and preserve the current cursor if no receptions arrive.
6. Test empty/start/tail reads, exact page boundaries, wraparound during paging,
   intentional lost transmissions, stale run IDs, removed stations, invalid
   limits/scopes/future sequences, and MaxUint64 arithmetic boundaries.

## Technical Details

Paging is a live continuation, not a frozen multi-page snapshot. Eviction
between requests may produce Gap on the next request. Gap is data with an
explicit recovery cursor, not a swallowed error. The caller discards incomplete
incremental reconstruction and rebuilds from retained data after a gap.
A fresh page still reports truncation through OldestSequence > 1.

## Verification

```text
go test ./simulation -run 'ReceptionHistory|Cursor|Page|Gap'
```

## Acceptance Criteria

Concatenated pages without concurrent mutation equal chronological retained
records exactly. Normal missed transmissions never set Gap. Restart cursors
are rejected, and station removal cannot redirect a cursor to another station.

## Non-Goals

Multi-station paging, opaque encoded tokens, HTTP status codes.

