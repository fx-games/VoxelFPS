# AGENTS.md

Notes for agents working in this repo.

## Keep the unused packages

`dungen/`, `audio/`, `geometry/` (including `jps.go` and `cc.go`) and
`quads/glhf/texture3d.go` currently have no callers in the main loop. **Do not
delete them.** They are kept deliberately — procedural generation, sound and the
3D texture path are all expected to be wired up later.

An audit that flags them as dead code is correct about the fact and wrong about
the conclusion. Leave them as they are.

## Run gofmt

The tree is gofmt'd and CI fails if it is not (`.github/workflows/test.yml`).
It used to be space-indented, so every edit came back tab-indented and buried
the real change in a whole-file diff. `gofmt -w .` before committing.
`.git-blame-ignore-revs` holds the reformat commit.
