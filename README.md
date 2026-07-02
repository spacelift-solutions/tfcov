# tfcov

Test coverage for Terraform/OpenTofu modules.

`tfcov` measures how much of a module's surface its example instantiations collectively exercise,
entirely statically — no `terraform apply`, no cloud credentials. Point it at a module and the
examples that instantiate it, and it reports:

- **Variable coverage** — the percentage of input variables that at least one example passes a
  value for.
- **Branch coverage** — the percentage of *assessable* branch points (`count`, `for_each`,
  `dynamic`, and conditional expressions) that the examples exercise **both ways** (e.g. a
  `count = var.enabled ? 1 : 0` gate seen both enabled and disabled).

Each branch is evaluated against every example's inputs and classified:

| Status | Meaning |
| --- | --- |
| `covered` | Exercised both ways (proven by evaluation). |
| `uncovered` | Only ever exercised one way. |
| `partial` | One way proven; the other undetermined (heuristic). |
| `unknown` | Depends on values not statically knowable (resource attributes, data sources, module outputs). Excluded from the coverage denominator. |

`tfcov` only measures and reports; gating on the numbers (CI thresholds, PR checks) is left to the
caller.

## Install

Download a binary from the [releases page](https://github.com/spacelift-solutions/tfcov/releases),
or install with Go:

```sh
go install github.com/spacelift-solutions/tfcov/cmd/tfcov@latest
```

## Usage

From a module whose examples live under `examples/`:

```sh
tfcov --module-root . --examples 'examples/*' --format table
```

| Flag | Default | Description |
| --- | --- | --- |
| `--module-root` | `.` | Path to the module under test. |
| `--examples` | `examples/*` | Glob of example directories that instantiate the module. |
| `--spacelift` | `false` | Discover the module and its examples from `.spacelift/config.yml` instead (searched upward from the working directory). |
| `--base-ref` | *(empty)* | Git ref to also measure for a ratchet comparison; empty disables it. |
| `--format` | `json` | Stdout format: `json` or `table`. |
| `--markdown-out` | *(empty)* | Also write a Markdown summary to this file. |
| `--version` | | Print version information and exit. |

### Ratchet mode

`--base-ref` measures the same module at another git ref and includes that baseline in the report,
so a caller can fail a change that *lowers* coverage without imposing an absolute floor. An
unreachable base ref (e.g. a shallow clone) prints a warning and skips the comparison rather than
failing.

### Spacelift mode

With `--spacelift`, the module and its examples are discovered from the test cases in
[`.spacelift/config.yml`](https://docs.spacelift.io/vendors/terraform/module-registry). This is how
the [Module Test Coverage plugin](https://github.com/spacelift-io/plugins) runs `tfcov` to gate
module PRs on coverage inside Spacelift.

## Limitations

- **`.tf.json` is not supported.** A module authored in JSON is rejected loudly (rather than
  silently undercounted).
- **Override files** (`*_override.tf`) are merged for the cases that affect coverage — variable
  defaults, locals, and `count`/`for_each` — but deep per-argument body merging is not performed.
- **Function support is a curated subset** of Terraform/OpenTofu built-ins. A branch whose
  condition uses an unsupported function is marked `unknown` — never scored incorrectly.

## Releasing

Releases are cut from the [Release workflow](../../actions/workflows/release.yml): enter a semver
version and pick `release` or `release-candidate`. The workflow tags the current commit and
[goreleaser](https://goreleaser.com) publishes binaries for linux/darwin/windows (amd64/arm64) with
a changelog grouped by conventional-commit prefix (`feat:`, `fix:`, `docs:`; `chore:`/`ci:`/
`test:`/`style:` are excluded) — so keep commit messages in that shape. Release candidates
(`vX.Y.Z-rc.N`) publish as GitHub pre-releases: downloadable, but excluded from `/releases/latest`.

## License

[MIT](LICENSE)
