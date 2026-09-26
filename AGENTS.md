# Agent notes

Working notes for contributors and coding agents. Usage docs are in
[README.md](README.md), design decisions in
[docs/design-notes.md](docs/design-notes.md) (with known gaps and the
performance baseline), open work in GitHub issues, history in `git log` and
CHANGELOG.md.

## Checks

- `just check` runs format, lint, tests and `go mod tidy`. Run
  `just fetch-testdata` once first.
- Markdown, YAML and JSON are formatted by prettier via treefmt. If prettier
  is missing locally, `just fmt` skips it silently while CI fails, so run
  `npx -y prettier@3 -w <files>` on edited Markdown (CHANGELOG.md excluded).
- `just interop` writes one file per DataType and reads it back with h5py and
  netCDF4 (`pip install h5py netCDF4`). Run it after any write-path change.
- Fuzzing: `GOMEMLIMIT=2GiB go test -run '^$' -fuzz FuzzOpen -fuzztime 60s`.
- Large files: `go test -tags largefiles -run . -bench . -benchtime 1x ./...`
  (≥ 100 MB synthetic file; weekly in `.github/workflows/largefiles.yml`).
  Compare against the baseline in docs/design-notes.md after read-path or
  go-hdf5 changes.

## Test fixtures

- Third-party `.sofa` files are never committed; `scripts/fetch-testdata.sh`
  is the manifest and `testdata/PROVENANCE.md` documents sources and licences.
- CI fetches only the required fixtures (CIPIC, MIT_KEMAR, Mesh2HRTF,
  Mesh2HRTF_HRTF_FourPointHorPlane_r100cm, tester). Files in
  `optionalTestdata` (`testhelpers_test.go`) skip when absent. Any other local
  file (e.g. `sofa20_sh_test.sofa`) hard-fails in CI, so check a fixture is
  in one of those two lists before using it in a test.
- `testdata/sofar/*.sofa` are the exception: synthetic MIT files written by
  sofar (netCDF-C) with `scripts/make_sofar_fixtures.py`, committed so CI
  covers GeneralTF 2.0, GeneralTF-E, FreeFieldHRTF (plain and SH),
  SimpleFreeFieldHRSOS, SingleRoomSRIR and SingleRoomDRIR. Their tests
  (`sofa_sofar_fixtures_test.go`) never skip. If you regenerate them
  (`pip install sofar==1.3.0`), the output is byte-stable for the same
  sofar/netCDF-C/HDF5 versions; update the hashes in `PROVENANCE.md`, and
  the pinned values only if you changed the generator on purpose.
- Prefer synthetic files built with `Save` into `t.TempDir()` where possible.
  CLI tests do this via `internal/clitest`.
- SOFA Toolbox cross-validation (MATLAB/Octave) is manual: README
  "Cross-validation". Octave 8.4 + `octave-netcdf` from apt works.

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
  around one meanwhile, open an issue on the fork and note the workaround in
  docs/design-notes.md (go-hdf5).
