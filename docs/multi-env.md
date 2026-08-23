# Multi-environment tools

A project usually holds a family of env files — `.env`, `.env.local`, `.env.staging`, `.env.prod`, `.env.example`. These commands answer the cross-file questions: what exists, what drifts, what's missing, and how do I hand someone a readable report.

## envs — discover the family

```sh
dotenvctl envs
dotenvctl envs --dir ./deploy
dotenvctl envs --json | jq -r '.data.files[].file'
```

```
ENV      FILE          KEYS  DISABLED  INHERITED  PLACEHOLDERS
default  .env          3     0         0          0
stag     .env.staging  72    4         0          2
prod     .env.prod     70    4         0          0
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

```
KEY             dev  stag  prod
DB_HOST         ✓    ✓     ✓
DB_PASS         ✓    !     ✓
FEATURE_X       ✓    #     ∅
EXTRA           —    ✓     —

✓ present · ∅ empty · ! placeholder · # disabled · → inherited · — missing
```

Cell legend: `✓` an active pair with a real value · `!` set but to a `__PLACEHOLDER__` · `∅` set to empty · `#` only a commented-out setting exists · `→` a name-only declaration (value comes from the environment) · `—` no entry at all.

Notes and sharp edges:

- **`--only-drift` counts a missing cell as drift.** If one column is much sparser than the rest (a 3-key `.env` beside full staging/prod files), nearly every row drifts and the filter appears useless — the tool prints a hint naming the sparse column when that happens. Compare just the files you mean: `dotenvctl matrix .env.staging .env.prod --only-drift`.
- **`--contract FILE`** adds the contract's keys to the table (all-missing rows are synthesized so gaps are visible), reports which envs miss which contract keys, and exits `1` when any gap exists — drop it into CI.
- **`--placeholder REGEX`** changes what counts as a placeholder.
- **Two files classifying to the same environment** (`.env.stag` + `.env.staging`, or two directories' bare `.env` via explicit args) get relabeled by filename or path — labels are always unique and no file's data is ever silently dropped.
- **JSON is leak-proof by default**: `--json` carries cell states only; `value` fields exist only under `--reveal`.

## HTML reports

`matrix` and `diff` take `--format human|json|html` and `-o PATH`:

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

# Any placeholder values left anywhere?
dotenvctl matrix --only-drift | grep '!'

# CI: fail the build when an env misses a key the template requires
dotenvctl matrix --contract .env.example --json > /dev/null

# Weekly drift report for the team (no secrets inside)
dotenvctl matrix --only-drift --format html -o reports/env-drift.html
```
