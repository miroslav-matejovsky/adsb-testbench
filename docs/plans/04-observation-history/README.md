# Received observations and bounded history

Implementation plan for [backlog item 04](../../backlog/04-observation-history.md).

Status: specified; implementation pending. The numbered files are ordered work
instructions. Proposed names below are new APIs unless explicitly identified
as existing APIs.

## Goal and scope

Retain exact per-station receptions and expose bounded, resumable history and
selected-station aircraft observations. Every observed field must come from a
received frame and age against engine virtual time. Truth, transmissions,
receptions, and decoded observations remain distinct representations.

Deliverables are engine-owned reception rings, immutable receiver provenance,
restart-aware cursors, a received-frame projection, transport-neutral DTOs in
simulatorapi, deterministic tests, and synchronized package documentation.
Runtime pacing, HTTP handlers, display sources, and UI remain backlog work.

## Existing implementation and architecture impact

- simulation/engine.go stages state.clone(), calls state.record() and
  state.deliver(), then commits after cancellation checks.
- simulation/history.go retains 1000 transmissions. Reception batches are
  complete, ordered by transmission sequence and station creation order.
- simulation/registry.go reserves removed station IDs for the run. New creates
  aircraft before stations can exist, so initial reports have no receptions.
- simulation/snapshot.go exposes coherent truth and transmission history.
- internal/adsb provides Decode and DecodeGlobal with an inclusive 10-second
  CPR age limit. It owns neither history nor a clock.
- simulatorapi currently contains only doc.go.

Keep simulation dependent only on internal/adsb and standard library packages.
Keep simulatorapi independent of simulation: define transport DTOs there;
mapping into them belongs to simulator in backlog 06. No architecture rule
changes are required. The engine projection is a diagnostic received-data
snapshot; backlog 07 will consume raw receptions and own display decoding.

## Required semantics

| Concern | Decision |
| --- | --- |
| Run identity | Config.ID identifies one engine lifetime. Caller must supply a fresh ID after restart, even with identical seed and start time. No random ID generation in the engine. |
| Reception identity | Run ID, station ID, and a station-local sequence starting at 1. Retain the existing transmission sequence separately. |
| Retention | Fixed ReceptionHistoryLimit = 1000 records per active station; evict oldest records only. Publish the effective limit. |
| Receiver provenance | Every Reception includes a value copy of the full Station settings, revision, and creation time in effect at reception. |
| History scope | One station per page. Cursor binds run ID, station ID, and last consumed reception sequence. |
| Selection | Explicit station IDs; empty means none. Deduplicate IDs, sort lexically, reject any unknown or removed ID atomically. |
| Observation source | Replay only retained selected-station receptions at snapshot time; never read fleet truth or generated history. |
| Deduplication | Decode each run/transmission sequence once after unioning selected histories; preserve all retained selected receiver copies as provenance. |
| CPR | Global pairing only, across selected receivers for the same aircraft; no truth or station-position reference fallback. |
| Aging | Explicit positive identity, position, altitude, and velocity TTLs in each observation request. Fresh when age <= TTL; expire when age > TTL. |
| Bounded projection | Snapshot covers retained evidence only. Eviction can remove a field before its TTL. Report retention truncation per station. |
| Station lifecycle | Disable retains history; updates retain old settings in old records; removal frees its reception ring and future selection returns ErrNotFound. |
| Aircraft lifecycle | Removing truth aircraft does not erase receptions. Observations remain while retained fields are fresh. |
| Ownership | Pages, observations, provenance, and nested optional values are detached from engine state. |

Projection replays records in transmission sequence order, ages CPR at each
reception timestamp, and applies field TTLs at the captured snapshot Now.
This preserves a fix that was valid when received even when its original pair
is now older than the CPR pairing window. A later lone half does not refresh
that fix. History truncation is visible rather than silently presented as a
complete reconstruction.

## Order and success criteria

Follow steps 01 through 07. Each step depends on its predecessor and includes
tests. See [progress](progress.md) for status and [assessment](assessment.md)
for risks and validation.

Success requires all backlog acceptance cases in step 06, complete artifacts
from steps 01-07, unchanged generated-frame determinism, and passing task all.
Preparing this plan does not mark backlog implementation complete.

