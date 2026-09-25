# Agent notes

Working notes for contributors and coding agents. Usage docs are in
[README.md](README.md), design decisions in
[docs/design-notes.md](docs/design-notes.md), open work in [PLAN.md](PLAN.md)
(open items only; history is in `git log` and CHANGELOG.md).

## Checks

- `just check` runs format, lint, tests and `go mod tidy`. Run
  `just fetch-testdata` once first.
- Markdown, YAML and JSON are formatted by prettier via treefmt. If prettier
  is missing locally, `just fmt` skips it silently while CI fails, so run
  `npx -y prettier@3 -w <files>` on edited Markdown (CHANGELOG.md excluded).
- `just interop` writes one file per DataType and reads it back with h5py and
  netCDF4 (`pip install h5py netCDF4`). Run it after any write-path change.
- Fuzzing: `GOMEMLIMIT=2GiB go test -run '^$' -fuzz FuzzOpen -fuzztime 60s`.

## Test fixtures

- Third-party `.sofa` files are never committed; `scripts/fetch-testdata.sh`
  is the manifest and `testdata/PROVENANCE.md` documents sources and licences.
- CI fetches only the required fixtures (CIPIC, MIT_KEMAR, Mesh2HRTF,
  Mesh2HRTF_HRTF_FourPointHorPlane_r100cm, tester). Files in
  `optionalTestdata` (`testhelpers_test.go`) skip when absent. Any other local
  file (e.g. `sofa20_sh_test.sofa`) hard-fails in CI, so check a fixture is
  in one of those two lists before using it in a test.
- Prefer synthetic files built with `Save` into `t.TempDir()` where possible.
  CLI tests do this via `internal/clitest`.

## Conventions

- Breaking API changes use `feat!:` commits and a CHANGELOG entry.
- Errors are sentinels (`ErrNotSOFA`, `ErrUnsupportedDataType`,
  `ErrIndexOutOfRange`, …) or `*ValidationError`. Tests assert them with
  `errors.Is` / `errors.As`, not message text.
- CLIs: `main` is `os.Exit(run(args, stdout, stderr))` with a
  `flag.FlagSet` (ContinueOnError). `-h` exits 0, a usage error 2, and any
  failed file 1 after the remaining files are processed. Progress and errors
  go to stderr.
- HDF5-level defects belong in the go-hdf5 fork. Where go-sofa has to work
  around one meanwhile, record it in PLAN.md Phase E.
