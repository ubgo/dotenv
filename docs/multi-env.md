# Multi-environment tools

A project usually holds a family of env files — `.env`, `.env.local`, `.env.staging`, `.env.prod`, `.env.example`. These commands answer the cross-file questions: what exists, what drifts, what's missing, and how do I hand someone a readable report.

On this page: [envs](#envs--discover-the-family) · [matrix](#matrix--the-drift-table) · [HTML reports](#html-reports) · [Recipes](#recipes) · [Pitfalls](#pitfalls) · [Related](#related)

**Before you start:** `dotenvctl` installed ([Getting started](getting-started.md#install)) and a directory holding at least two env files. Every output on this page comes from this family (the same files [`ExampleExecute_envs`](../cli/dotenvcmd/example_test.go) and its siblings use):

```env
# .env
# local development
DB_HOST=localhost
DB_PORT=5432
API_URL=https://api.${DOMAIN}
DOMAIN=acme.dev

# .env.staging
DB_HOST=stag.db.internal
DB_PASS=stag-pass
DOMAIN=staging.acme.io
FEATURE_X=on

# .env.prod
DB_HOST=prod.db.internal
DB_PASS=__YOU__
DOMAIN=acme.io
# FEATURE_X=on
SENTRY_DSN=https://key@sentry.example/1

# .env.example
DB_HOST=
DB_PASS=
DOMAIN=
SENTRY_DSN=
```

## envs — discover the family

```sh
dotenvctl envs
dotenvctl envs --dir ./deploy
dotenvctl envs --json | jq -r '.data.files[].file'
```

```console
$ dotenvctl envs
ENV                 FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default             .env          4     0         0          0
stag                .env.staging  4     0         0          0
prod                .env.prod     4     1         0          1
example (contract)  .env.example  4     0         0          0
$ dotenvctl envs --dir ./empty
no env files found
```

`--json` gives one object per file, with the path relative to `--dir` and a `contract` flag (pinned by [`ExampleExecute_json`](../cli/dotenvcmd/example_test.go)):

```console
$ dotenvctl envs --json
{"ok":true,"data":{"files":[{"env":"default","file":".env","path":"./.env","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"stag","file":".env.staging","path":"./.env.staging","contract":false,"keys":4,"disabled":0,"inherited":0,"placeholders":0},{"env":"prod","file":".env.prod","path":"./.env.prod","contract":false,"keys":4,"disabled":1,"inherited":0,"placeholders":1},{"env":"example","file":".env.example","path":"./.env.example","contract":true,"keys":4,"disabled":0,"inherited":0,"placeholders":0}]}}
```

Detection rules (non-recursive, filename-based):

| Filename | Classified as |
|---|---|
| `.env` | `default` |
| `.env.local`, `.env.dev`/`.env.development`, `.env.test`/`.env.testing`, `.env.ci`, `.env.stag`/`.env.staging`, `.env.prod`/`.env.production` | the canonical short name — long forms collapse, so `.env.production` and `.env.prod` mean the same environment |
| `.env.example` / `.sample` / `.template` / `.dist` | a **contract** — listed here, excluded from matrix columns, usable via `--contract` |
| `.env.bak` / `.tmp` / `.old` / `.orig` / `.swp` / `.swo` / `.lock` | ignored (editor/backup droppings) |
| any other `.env.something` | kept, named `something` verbatim |

Ordering is always distance-from-prod: `default → local → dev → test → ci → stag → prod`, unknown names alphabetically, contracts last. `foo.env` is deliberately not detected — that naming belongs to other tools. The PLACEHOLDERS column counts values matching `^__[A-Z0-9_]+__$` (the `__YOU__` convention) — your "still needs a real value" signal.

## matrix — the drift table

```sh
dotenvctl matrix                                   # all detected files as columns
dotenvctl matrix .env.staging .env.prod            # exactly these files
dotenvctl matrix --only-drift                      # hide rows present everywhere
dotenvctl matrix --values                          # show values — masked as •••••• 
dotenvctl matrix --values --reveal                 # real values (treat output as a secret)
dotenvctl matrix --contract .env.example           # CI gate: exit 1 when an env misses a contract key
```

```console
$ dotenvctl matrix
KEY         default  stag  prod
API_URL     ✓        —     —
DB_HOST     ✓        ✓     ✓
DB_PASS     —        ✓     !
DB_PORT     ✓        —     —
DOMAIN      ✓        ✓     ✓
FEATURE_X   —        ✓     #
SENTRY_DSN  —        —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
$ dotenvctl matrix .env.staging .env.prod --contract .env.example; echo "exit=$?"
KEY         stag  prod
DB_HOST     ✓     ✓
DB_PASS     ✓     !
DOMAIN      ✓     ✓
FEATURE_X   ✓     #
SENTRY_DSN  —     ✓

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
contract: stag is missing 1 key(s): [SENTRY_DSN]
exit=1
```

The second run is pinned by [`ExampleExecute_matrix`](../cli/dotenvcmd/example_test.go). `.env.example` is never a column: it is the contract.

Cell legend: `✓` an active pair with a real value · `!` set but to a `__PLACEHOLDER__` · `∅` set to empty · `#` only a commented-out setting exists · `→` a name-only declaration (value comes from the environment) · `—` no entry at all.

Notes and sharp edges:

- **`--only-drift` counts a missing cell as drift.** If one column is much sparser than the rest (a 3-key `.env` beside full staging/prod files), nearly every row drifts and the filter appears useless — the tool prints a hint naming the sparse column when that happens. Compare just the files you mean: `dotenvctl matrix .env.staging .env.prod --only-drift`.
- **`--contract FILE`** adds the contract's keys to the table (all-missing rows are synthesized so gaps are visible), reports which envs miss which contract keys, and exits `1` when any gap exists — drop it into CI.
- **`--placeholder REGEX`** changes what counts as a placeholder. It replaces the default `^__[A-Z0-9_]+__$`, so write `'^(__[A-Z0-9_]+__|TODO)$'` to add a marker rather than swap one; an invalid expression exits `1` before anything is read. It applies to `matrix` only, not to the `envs` column or the plugin guards. Real runs: [Changing what counts as a placeholder](commands.md#changing-what-counts-as-a-placeholder---placeholder).
- **Two files classifying to the same environment** (`.env.stag` + `.env.staging`, or two directories' bare `.env` via explicit args) get relabeled by filename or path — labels are always unique and no file's data is ever silently dropped.
- **JSON is leak-proof by default**: `--json` carries cell states only; `value` fields exist only under `--reveal`.

## HTML reports

`matrix` and `diff` take `--format human|json|html` and `-o`/`--output PATH`:

```sh
dotenvctl matrix --format html -o envs.html
dotenvctl diff .env.staging .env.prod --format html -o diff.html
open envs.html
```

The page is fully self-contained — inline CSS, no JavaScript, no external requests, light/dark via your OS preference — and written with `0600` permissions. **Masked by default, structurally**: without `--reveal` the secret values are never placed in the document at all (not hidden by styling), so the default report is safe to attach or commit. `--reveal` embeds real values and stamps a red "CONTAINS SECRETS" banner.

## Recipes

```sh
# Which keys is prod missing relative to staging?
dotenvctl diff .env.staging .env.prod --json | jq -r '.data.removed[].key'

# Any placeholder values left anywhere? (exit 1 and the rows when there are)
dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'

# CI: fail the build when an env misses a key the template requires
dotenvctl matrix --contract .env.example --json > /dev/null

# Weekly drift report for the team (no secrets inside)
dotenvctl matrix --only-drift --format html -o reports/env-drift.html
```

```console
$ dotenvctl envs | awk 'NR > 1 && $NF > 0 { print "placeholders left:", $0; bad = 1 } END { exit bad }'; echo "exit=$?"
placeholders left: prod                .env.prod     4     1         0          1
exit=1
```

## Pitfalls

- **`matrix` needs two files.** With fewer, it exits `1` with `envkit: matrix needs at least two env files, found 1`. That includes `dotenvctl matrix .env.prod` on its own.
- **`grep '!'` always matches.** The legend line under every human matrix contains `! placeholder`. Count placeholders with `envs` (above) or select `"state":"placeholder"` cells from `matrix --json`.
- **`--values` hides the placeholder marker.** Masked values are all `••••••`, placeholders included; the `!` symbol only appears in the default symbol view.
- **The contract checks presence, not quality.** A contract key set to `__YOU__` passes `--contract`; a key that exists only commented out (`# KEY=…`) fails it. Run the placeholder gate beside the contract gate.
- **An empty or missing directory is quiet.** `envs --dir` on a directory with no env files prints `no env files found` and exits `0`; a directory that does not exist exits `1` with the read error.
- **`diff --json` carries real values, `matrix --json` does not.** Treat a `diff` envelope like the env files themselves.

## Related

- [Command reference](commands.md) — every flag of [`envs`](commands.md#envs--discover-the-env-family), [`matrix`](commands.md#matrix--the-keys--environments-drift-table) and [`diff`](commands.md#diff--compare-two-files-effective-configuration)
- [Recipes](recipes.md) — the [CI gate](recipes.md#ci-gate-the-build-on-env-completeness) and the weekly drift report in full
- [Go API guide](go-api.md) — `envkit.Inventories`, `envkit.BuildMatrix` and `envkit.Diff`, the functions behind these verbs
- [Feature index](reference/features.md)
