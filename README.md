# Gooo Change Bundle

`gooo-change-bundle` is the safe actuator boundary for a Gooo self-improvement
loop. It turns an exact source-tree digest, an approved proposal and authority
receipt, and a `.gooo` change intent into a deterministic bundle in a
caller-owned temporary directory.

The product materializes evidence; it does not mutate the input repository.
Apply, commit, push, pull-request, and merge authority are hard-coded to zero.
An external consumer may apply the emitted patch only after independently
checking the apply-precondition receipt.

## Input and output

```text
exact source tree + approved proposal + authority receipt + change intent
        │
        └──> source/target manifests, patch, rollback, IR, evaluator,
             precondition receipt, replay receipt, and human dossier
```

```sh
go run ./cmd/gooo-change-bundle digest --source-root /path/to/source
go run ./cmd/gooo-change-bundle materialize \
  --source-root /path/to/source \
  --source-digest sha256:... \
  --proposal /path/to/approved-proposal.json \
  --authority /path/to/authority-receipt.json \
  --intent examples/change-bundle/change-intent.gooo \
  --contract contracts/change-bundle-denominator-v1.json \
  --out /caller-owned/empty-directory
```

The bundle output includes `patch.bundle.json` and `patch.diff`, the inverse
`rollback.bundle.json` and `rollback.diff`, exact preimage/postimage digests,
`semantic-ir.json`, `generated/evaluator.go`,
`apply-precondition-receipt.json`, `replay-receipt.json`, and
`human-dossier.md`. Output must be empty and outside the source tree. Source
files are read through a no-symlink tree boundary and are never opened for
writing.

Known contradictions are `REFUTED`; unavailable observations are `UNKNOWN`;
otherwise the result is `CLOSED`. The order is exactly:

```text
REFUTED > UNKNOWN > CLOSED
```

Every `UNKNOWN` retains `stage`, `step`, `reason`, `unknown_class`,
`next_operation`, and `blocked_by`. Stale preimages, traversal or symlink
escape, generated-file authority, conflicting hunks, unauthorized proposals,
and rollback mismatches fail closed.

An optional platform observation can bind a release audit to an external
GitHub API result. The external `immutable` value is authoritative over any
self-authored release-manifest claim. Therefore
`external_immutable=false` plus `self_authored_immutable=true` is the canonical
`REFUTED` case `EXTERNAL_IMMUTABILITY_CONTRADICTION`.

## Fixed contract

The intent and denominator contract have exactly 12 cells and exactly 12
activities, bound one-to-one. `FOUNDATION`, `COHERENCE`, and `REGRESSION` each
contain four cells; `DRIVER`, `OUTCOME`, and `GUARDRAIL` each contain four.
The fixture oracle exercises three normal, three `UNKNOWN`, and six `REFUTED`
cases, including the canonical external-platform contradiction. Each case
records exact changed-path, hunk, replay, and rollback comparison counts.

The CI receipt records exact bundle file count/bytes, changed paths/hunks,
replay and rollback comparisons/mismatches, build/test/conformance wall time,
peak RSS, executed/reused/skipped/not-observed tests, Go/Gooo physical lines,
files/directories, and the fixed zero tuple:

```text
repository_writes=0
local_test_executions=0
cross_project_required_gates=0
```

An exact before/after pair is required to claim an improvement; without one,
the improvement state is `UNKNOWN`.

## Repository process

The public bootstrap `main` commit contained only `.gitignore`, `LICENSE`, and
this README. Substantive implementation is carried by one pull request.
GitHub Actions is the only validation authority and uses Go 1.27. The
annotated `v0.1.0` release is retained as failed evidence because the GitHub
API reported `isImmutable=false`, despite its self-authored manifest claim.
It is not deleted, edited, or reused. The corrected product release is the
annotated `v0.1.1` release, created only after GitHub repository immutable
releases were enabled. Its source archive, release manifest, CI conformance
bundle, and SHA-256 digests are the only release-success evidence.
Failed CI/release attempts remain as append-only counterexamples in
[`docs/counterexamples/release-failures-v1.json`](docs/counterexamples/release-failures-v1.json).
The three post-PR direct-main commits are preserved there as
`DEVELOPMENT_PROCESS_DIRECT_MAIN` `REFUTED` events and are not counted as
successful development process evidence.

The existing portfolio already covers candidate selection, counterfactual
evaluation, semantic mutation, adoption transactions, and rollback policy.
This repository is intentionally narrower: it is the missing reproducible
change-bundle materialization boundary between an approved intention and an
external, separately authorized applier.
