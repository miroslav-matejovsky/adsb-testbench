# Assessment

## Feasibility and dependencies

The engine already centralizes accepted receptions in state.deliver(), owns
virtual time, and stages mutations atomically. Extend those boundaries instead
of adding a runtime collector. The codec supplies frame validation and global
CPR reconstruction. No new third-party dependency or external research is
required. Backlog 05 is not a prerequisite.

The concrete API must follow the decisions in README.md and step 01.
Backlog 06 maps engine values to the shared DTOs and assigns fresh run IDs at
runtime creation. Backlog 07 implements display accumulation from receptions.

## Risks and mitigations

| Risk | Required mitigation and evidence |
| --- | --- |
| Transmission gaps mistaken for lost receptions | Separate consecutive station sequences; step 03 tests intentional reception loss without a retention gap. |
| Restart reuses deterministic settings | Require unique caller run ID per lifetime; reject mismatched cursor IDs before reading records. Test identical scenario with different IDs. |
| Receiver edits rewrite historical meaning | Copy full Station values into each accepted reception; test edit, disable, and removal. |
| Partial staged state escapes | Deep-copy station ring backing arrays in registry cloning; extend cancellation and overflow tests. |
| Ring replay loses older live fields | Explicit retained-evidence semantics and truncation metadata; test eviction before TTL and successful recovery from newer evidence. |
| Decoder accidentally uses truth | Projection takes only receptions, Now, and request policy; test missing frames and deliberately different wire-quantized values. |
| Duplicate receiver copies bias state | Union by transmission sequence before decoding, retaining all receiver provenance. Test permutations and complementary CPR halves. |
| CPR result is treated as operational navigation | Use the existing codec guards for synthetic engine frames; no operational integrity claim or arbitrary external-frame ingestion in this work. Display validation remains backlog 07. |
| Copy cost grows with station count | Maximum 8000 retained receptions plus bounded output; no unbounded aircraft cache. Exercise wraparound with all 8 stations. |
| Documentation claims future code exists | Update implementation docs only as the corresponding steps land. Keep plan progress accurate. |

Receiver-history memory is bounded by 8 * 1000 Reception values, plus temporary
staged copies and complete mutation batches already bounded by
MaxBatchReceptions. Observation evidence is bounded by selected rings. The
existing station-ID reservation set grows with lifetime station churn; this
plan does not claim to bound all engine memory.

## Validation

Each numbered step specifies targeted Go tests. Use table-driven tests and
testify/require, fixed virtual timestamps, and caller-controlled Advance.
Use existing atomicity test techniques for deterministic cancellation. No
network services, sleeps, or wall-clock timing are needed.

Finish with go test ./simulation ./simulatorapi, task all, and a diff review.
Run race tests for the changed packages when supported by the installed Go
race toolchain; any unsupported race setup is recorded separately and does
not replace task all. task all must pass.

## Rollback

No persisted data migration exists. Before implementation, preserve unrelated
working-tree edits. If a step fails, correct its changes locally and retain
the failing regression test; do not advance its status. Revert only explicitly
identified changes from this work if requested. Never reset the worktree,
modify vendor, or commit. Leave concrete unfinished implementation work in
the repository-root .todo if implementation is started but cannot finish.

