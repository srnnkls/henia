# Henia as a Phora compiler hook

[Phora](https://github.com/srnnkls/phora) can fetch canonical skills, call
`henia build` from a hook, and deploy the compiled harness trees wherever you
want them. This keeps installation and deployment state in Phora's hands, for
setups that `henia install` doesn't fit.

```text
Phora source pins → canonical tree → Henia → harness trees → Phora targets
```

From a checkout of this repository, with both tools on PATH:

```console
$ cd examples/phora/stage
$ phora sync
Built 2 artifact(s) in ../../../.henia/build
sync complete
hook post_sync#henia --config ../../../.henia/canonical/henia.toml build … ok
sync complete

$ cd ../deploy && phora list
claude:
  claude/skills/review/SKILL.md  linked
  claude/skills/review/reference/guide.md  linked
codex:
  codex/skills/review/SKILL.md  linked
  codex/skills/review/agents/openai.yaml  linked
  codex/skills/review/reference/guide.md  linked
```

Everything it writes stays under `.henia/` at the repository root:
`canonical/` holds the staged inputs, `build/` the compiled trees, and
`installed/` the deployed links.

## Two phases

[The staging config](../examples/phora/stage/phora.toml) copies the example
skills and `henia.toml` from this repository into `.henia/canonical`. Its
`post_sync` hook builds them and then syncs
[the deployment config](../examples/phora/deploy/phora.toml) in a second
directory:

```toml
[hooks]
post_sync = "henia --config ../../../.henia/canonical/henia.toml build ../../../.henia/canonical --output ../../../.henia/build --clean && (cd ../deploy && phora sync --prune --fast-forward)"
```

The `&&` means a compiler error stops the deployment. The hook is a global
`post_sync` because that also runs after a sync that only removed files, which
`on_change` would miss. No source `build` setting, generated Git package or
wrapper script is involved.

The deployment config declares one source per harness output, bound to its
target. A local output directory needs no Git repository, and Phora's
`deploy = "link"` deploys it as links:

```toml
[sources]
claude = { path = "../../../.henia/build/claude", deploy = "link" }
codex = { path = "../../../.henia/build/codex", deploy = "link" }

[targets]
claude = { path = "../../../.henia/installed/claude", sources.claude = { collapse = false } }
codex = { path = "../../../.henia/installed/codex", sources.codex = { collapse = false } }
```

Pass `--clean`, or set `[build] clean = true`, so a successful build removes
stale artifacts and a failed one leaves the previous output in place. That
setting belongs to Henia. Phora owns installation paths, link ownership and
pruning.

## One phase

Phora's `pre_sync` runs before source resolution. When the canonical inputs
are already on disk, it can call `henia build` directly. Use the two-phase form
when Phora first has to fetch the pinned inputs.

`phora sync --frozen --no-hooks` replays one phase without running the
compiler. Generated links still need their build directory, and rebuilding from
the cached canonical inputs recreates it.

In this setup Henia reads no deployment state and writes nothing outside its
`--output`. `henia install` is the separate path where Henia writes harness
homes itself; see [Installing](runtime.md#installing).
