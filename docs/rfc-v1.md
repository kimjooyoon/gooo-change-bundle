# RFC: Gooo Change Bundle v1

## Scope

The bundle is a materialization boundary, not an applier. Its only write
destination is an empty directory owned by the caller. The source tree is
read-only, and all external repository operations remain outside the product's
authority ceiling.

The input chain is:

```text
source tree digest + approved proposal + authority receipt + .gooo intent
       → exact tree observation → semantic IR → deterministic patch bundle
       → precondition/rollback/replay evidence
```

Proposal and receipt digests are self-binding. The authority receipt binds to
the proposal digest; the proposal binds to the receipt identity, avoiding a
cyclic digest while preserving the full approval chain.

## Safety rules

The source tree manifest is sorted by canonical relative path and excludes
only the exact root `.git` directory. Symlinks, non-regular entries, absolute
paths, `..` traversal, non-canonical paths, and generated-file targets are
rejected. A target's preimage digest must equal the observed source file.
Duplicate target paths and overlapping hunk ranges are rejected as conflicts.

The forward and inverse transformations operate on an in-memory file map. The
fixture oracle compares the forward state with the postimage tree and the
rollback state with the original tree, so the source tree is not used as a
scratch area.

## Resolution

`REFUTED` is used for a known contradiction or unsafe request. `UNKNOWN` is
used when direct observation or a dependency is unavailable. Every unknown
claim carries the six operational coordinates required by the Gooo protocol.
Resolution precedence is `REFUTED > UNKNOWN > CLOSED`, even when an unknown
finding and a refutation coexist.

## Metrics

CI owns environment-dependent wall time and peak RSS. It emits integer metrics
for bundle file count/bytes, changed paths/hunks, replay and rollback
comparisons/mismatches, executed/reused/skipped/not-observed tests,
Go/Gooo physical lines, and files/directories. Product and CI authority fields
are always `repository_writes=0`, `local_test_executions=0`, and
`cross_project_required_gates=0`.

