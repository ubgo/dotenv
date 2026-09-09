# Changelog

All notable changes to **dotenv** are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
### Changed
### Deprecated
### Removed
### Fixed
### Security

## [0.1.1] - 2026-09-09

No library or CLI behaviour changed in this release. Every Go file outside the test suites is byte-identical to 0.1.0 — it is a licence, documentation and test-coverage release, which is why it is a patch.

### Changed

- **Licence: MIT to Apache-2.0.** Apache-2.0 adds an explicit patent grant and a clearer contribution clause; it does not restrict anything MIT permitted. Copies obtained as 0.1.0 remain under MIT, since a published version's terms cannot be withdrawn.
- README restructured so both deliverables — the library and `dotenvctl` — are visible up front, with an install line and a documentation route for each.
- Documentation reports **measured** coverage with statement counts (100.00% library, 99.52% CLI) rather than rounded claims.

### Fixed

- The coverage note in CONTRIBUTING carried three errors: it claimed seven uncovered statements while listing six, omitted the `os.Executable` fallback entirely, and labelled two arms in `get` and `list` as reachable gaps. They are not reachable — the CLI never attaches a library plugin, and a plugin is the only thing that makes expansion fail with something other than a `RequiredError`. The arms remain in place because they guard a documented library contract.
- Every documented command now shows real captured output rather than illustrative text.

### Added

- Coverage tasks across both modules (`task cover`, `test:cover`, `test:uncovered`) and a pinning test for the root dispatcher's non-quiet `CmdError` arm, verified by breaking the arm it covers.

### Note on 0.1.0

The `v0.1.0` and `cli/v0.1.0` tags were published to the Go module proxy and then deleted from the repository. Deleting a tag does not withdraw it: both versions remain permanently recorded in the checksum database and are still installable. They are therefore **never reused** — this release skips forward to 0.1.1 rather than re-tagging.

<!--
Release process:
  1. Move the relevant [Unreleased] entries under a new version heading below.
  2. Date it: ## [1.2.0] - YYYY-MM-DD
  3. Tag the release (e.g. v1.2.0) and update the link refs at the bottom.
-->

[Unreleased]: https://github.com/ubgo/dotenv/compare/v0.1.1...main
[0.1.1]: https://github.com/ubgo/dotenv/releases/tag/v0.1.1
