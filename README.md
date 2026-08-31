# Gooo Change Bundle

Bootstrap repository for a fail-closed actuator boundary for the Gooo
self-improvement loop. The initial public `main` commit intentionally contains
only `.gitignore`, `LICENSE`, and this `README.md`; the implementation is
introduced through one pull request and verified by GitHub Actions.

The product will materialize a deterministic, caller-owned change bundle from
an exact source-tree digest, an approved proposal and authority receipt, and a
`.gooo` change intent. It will never apply, commit, push, open a pull request,
or merge a change, and will never write to the input source tree.

