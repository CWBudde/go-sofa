# go-sofa — SOFA (AES69) File Reader/Writer in Go

## Goal

A pure-Go library for reading and writing SOFA files (AES69 — Spatially
Oriented Format for Acoustics), built on top of
[MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5)
(our fork of scigolib/hdf5). The Go equivalent of
`PasSofa/SofaFile.pas`.

## Status

Read, write, CLI tools, CI, lint, and the `FIR`, `TF`, `TF-E`, and `SOS`
`DataType`s are implemented; written files open in h5py/netCDF4 with named
netCDF-4 dimensions (checked in CI). This file tracks only what's still open;
history is in `git log` / CHANGELOG.md, design decisions in
[docs/design-notes.md](docs/design-notes.md).

## Prior Art & Key Resources

| Resource                                                                                | Notes                                                         |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `../PasSofa/Source/SofaFile.pas`                                                        | Our own SOFA reader (~430 LOC Pascal). Behavioural reference. |
| `../go-hdf` (go-hdf5 fork)                                                              | Pure-Go HDF5 read+write. Provides the low-level file access.  |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based).                |
| [libmysofa](https://github.com/hoene/libmysofa)                                         | Lightweight C SOFA reader — useful as behavioural reference.  |

---

## Open work

Phase R: the release blockers R1–R4 are done (go-hdf5 fixes from
[CWBudde/go-hdf5#1](https://github.com/CWBudde/go-hdf5/pull/1) and
[CWBudde/go-hdf5#2](https://github.com/CWBudde/go-hdf5/pull/2), released as
go-hdf5 v0.16.0). R5 and R6 are done; R7 is done except R7c (blocked on go-hdf5 reader/writer entry points) and R7f (API decision); R8 is done; R9 is done except R9d (release-tag decision). Phase C1–C4
and C3b (lazy measurement reads for every DataType) are done; C5 is open. Phases C–E are optional /
future and can be picked up on demand when a real use case appears.

### Phase R — Review findings 2026-09-24 (blocking)

Multi-area review (read path 4/10, write path 2/10, API/CLI 4/10,
tests/tooling 3/10; overall ≈3/10). Items are ordered by priority;
R1–R4 are release blockers.

#### R1–R6 — ✅ done (2026-09-24 … 2026-09-25)

- **R1 Interop:** go-hdf5 output readable by libhdf5/netCDF-C; full netCDF-4
  dimension scales; `just interop` CI job (h5py + netCDF4).
- **R2 Crash safety:** dimensions validated before reads; upstream OOMs
  fixed; `FuzzOpen` with seed corpus.
- **R3 Tests on fresh clone:** `just fetch-testdata` + `PROVENANCE.md`,
  third-party fixtures, `LICENSE`.
- **R4 Save durability:** close error returned, atomic temp+fsync+rename,
  deterministic output.
- **R5 Read path:** accessors return errors, no slice aliasing, unsupported
  DataTypes rejected, shape-aware reads (incl. Toolbox TF-E `[M,R,N,E]`),
  broadcasting helpers (`SourcePositionAt`, `DelayAt`, `SamplingRateAt`),
  errors propagated, SH detection by `EmitterPosition:Type`.
- **R6 Write conformance:** mandatory attributes, variable attributes, 2-D
  `Data.Delay`, data validation, lossless round-trip of unknown
  attributes/variables, TF-E written `[M,R,N,E]`, per-measurement layouts.

#### R7 — API ergonomics (medium)

- [x] **R7a.** Sentinel/typed errors (`ErrNotSOFA`,
      `ErrUnsupportedDataType`, `*ValidationError{Field}`) usable with
      `errors.Is/As`; capitalise field names in messages.
  - (2026-09-26) — `Open` wraps `ErrNotSOFA` for a non-SOFA `Conventions`;
    every error of `validate` (dimensions, shapes, coordinate Types,
    values, extras, convention rules) is a `*ValidationError{Field, Err}`
    whose `Field` is the `File` field at fault and whose message starts
    with it (`M: must be > 0`, `ImpulseResponses[0] length …`); an unknown
    DataType also unwraps to `ErrUnsupportedDataType`.
    `TestValidationErrorField`, `TestValidationErrorUnwraps` and
    `TestOpenNonSOFAHDF5` cover it, and the reject tables assert
    `errors.As`.
- [x] **R7b.** Export DataType constants (`DataTypeFIR`, …).
  - (2026-09-26) — `DataTypeFIR`, `DataTypeTF`, `DataTypeTFE`,
    `DataTypeSOS` replace the unexported constants; the CLIs and the
    interop generator use them instead of string literals.
- [ ] **R7c.** `io.ReaderAt`/`io.Writer` entry points (`Read(r)`,
      `(*File).WriteTo(w)`), if go-hdf5 allows.
  - (2026-09-26) — partial: blocked upstream. go-hdf5 v0.16.1 only has
    `Open(filename)` and `CreateForWrite(filename)`; add a reader-based
    open and a writer-based create in go-hdf5, release them, then add the
    entry points here.
- [x] **R7d.** Release the HDF5 handle after eager `Open` (or make `Close`
      meaningful via Phase C lazy mode).
  - (2026-09-26) — `Open` closes the HDF5 file before returning (all reads
    were already eager); `Close` is a documented no-op returning nil.
    `TestOpenReleasesHandle` checks the process holds no extra
    descriptor after `Open`.
- [x] **R7e.** Fix godoc: `Vector3` "in meters" (wrong for spherical),
      `File` "an open SOFA file", `Delay` shape.
  - (2026-09-26) — `Vector3` names the units per coordinate Type, `File`
    describes eagerly loaded contents, and `Delay` lists its 1/M/R/M×R
    layouts.
- [ ] **R7f.** Consider replacing parallel per-DataType fields (`TFReal` vs
      `TFRealE`, …) with a `Data` interface / tagged union before v1 to
      avoid API churn.

#### R8 — CLIs — ✅ done (2026-09-26)

`flag`-based CLIs processing all file arguments with proper exit codes;
streaming sofa2json (library field names, NaN → `null`, `-f` to overwrite);
sofainfo shows conventions and a delay summary; sofaprobe previews
Real/Imag/SOS via `ReadSlice` (whole rows, see E5); smoke tests via
`internal/clitest` (84–90 % coverage).

#### R9 — Docs, tooling, CI hygiene (low)

- [x] **R9a.** README: fix module path (`github.com/cwbudde/go-sofa`, not
      `MeKo-Christian`) in all `go get`/`go install`/import lines and
      go-hdf5 links; "reading" → "reading and writing"; update "Known
      limitations" (CLASS/NAME are emitted), supported DataTypes, document
      `--include-sos`.
  - (2026-09-26) — module path and go-hdf5 links point at `CWBudde`; the
    intro says "reading and writing"; Features list the four DataTypes,
    netCDF-4 output and all three CLIs (with a `sofaprobe` install line);
    the stale CLASS/NAME limitation is removed (`--include-sos` was
    already documented).
- [x] **R9b.** justfile: `GOPRIVATE` points at the wrong owner;
      `just build` builds only sofaprobe; add `-race` to `just test`.
  - (2026-09-26) — `GOPRIVATE` and `-race` had been fixed earlier;
    `just build` / `just install` now loop over sofainfo, sofa2json and
    sofaprobe, and the tool install comment pins the CI versions.
- [x] **R9c.** CI: trigger on `pull_request`; pin tool and golangci-lint
      versions; enforce a coverage floor; install shellcheck or drop it
      from treefmt; optional Go-version matrix.
  - (2026-09-26) — `pull_request` was already a trigger; golangci-lint
    v2.13.2, gofumpt v0.12.0, gci v0.14.0, shfmt v3.14.1 and prettier 3
    are pinned; shellcheck is installed with apt; `just coverage-check`
    (root package ≥ 85 %, 89.4 % with the CI fixtures) runs in test-unit,
    which covers the go.mod Go version and `stable`.
- [ ] **R9d.** Tag `v0.1.0` only after R1–R4 (CHANGELOG documents a
      release that has no tag).
  - (2026-09-26) — open: `v0.1.0` is tagged and pushed, but at 24eebca
    (2026-08-16), before R1–R4 landed. Decide whether to keep it or tag
    the next release after Phase R.
- [x] **R9e.** Coverage gaps: `readGlobalAttributes` 50 % (15 of 20
      attributes never read in tests), `Save` error branches,
      `write*AudioDatasets` 66–71 %.
  - (2026-09-26) — `TestOpenReadsEveryGlobalField` writes and reads back
    all 22 attributes of `globalFields()`; `TestSaveTargetIsDirectory`
    and `TestSaveMissingDirectory` (now `errors.Is(fs.ErrNotExist)`)
    cover the path errors; `TestSaveAudioWriteErrors` injects a failure
    per audio variable through `writeVariableTestHook`, bringing all
    `write*AudioDatasets` to 100 % and `Save` to 79 %. The remaining
    `Save` branches (chmod/rename/fsync failures) need a filesystem seam.

### Phase A — SH HRTF support ✅ done (2026-05-10)

SH accessors (`IsSHEncoded`, `SHOrder`, `SHCoefficientCount`, `SHWarnings`)
on top of TF-E I/O; no separate SH DataType (see design notes).

### Phase B — Convention-specific behaviour ✅ done (2026-09-25)

Convention registry (`sofa_conventions.go`) with BRIR, SRIR, SimpleFreeField*,
FreeFieldHRTF and Directivity rules, `ConventionWarnings()`, `IsBRIR`,
`IsSRIR`/`AmbisonicsOrder`, `IsDirectivity`, `RoomVolume`/`RoomTemperature`
(see README "Conventions"). A Directivity validator needs an example file.

### Phase C — Streaming / partial reads

Hyperslab reads would let consumers stream a subset (e.g. one
measurement at a time) instead of loading the whole `[M][R][N]` array
into memory. Useful for very large HRTF databases.

Tasks:

- [x] **C1. Upstream capability check.** Inspect the go-hdf5 API for
      hyperslab / partial-read support; if missing, file an upstream
      issue (link it back here) before continuing.
  - Acceptance: this PLAN cites either the supporting go-hdf5 API or
    the tracking issue URL.
  - (2026-09-26) — go-hdf5 v0.16.1 has `(*Dataset).ReadSlice(start,
count)` and `ReadHyperslab(sel)` (`dataset_read_hyperslab.go:65`,
    `:143`) for compact, contiguous and chunked layouts. Selections over
    whole trailing axes take the linear path and avoid E5; chunked
    datasets are decompressed chunk by chunk without a cache (see E6).
- [x] **C2. Lazy `File` mode.** Add
      `OpenLazy(path string) (*File, error)` that parses metadata but
      leaves audio datasets unloaded. Existing `Open` keeps eager
      semantics.
  - Acceptance: `TestOpenLazyDoesNotAllocateAudio` opens a >10 MB
    file and asserts `len(f.ImpulseResponses)==0` plus `runtime.MemStats`
    delta below an eager-open baseline by ≥ 50 %.
  - (2026-09-26) — `OpenLazy` checks the audio datasets' layout but does
    not read them and keeps the file open until `Close` (idempotent;
    `Open` is unchanged). On a 700×2×1024 synthetic file (> 10 MB) it
    allocates 1.3 MB against Open's 24.4 MB (`TestOpenLazyDoesNotAllocateAudio`);
    `TestOpenLazyMatchesOpen` compares it with `Open` on the CI fixtures.
- [x] **C3. Per-measurement reader.** Implement
      `(*File).ReadMeasurement(m int) ([][]float64, error)` shaped
      `[R][N]` for FIR, with sibling helpers for TF/SOS as needed.
  - Acceptance: `TestReadMeasurementMatchesEager` loads the same file
    eagerly and via `ReadMeasurement` for every `m`, asserts deep
    equality.
  - (2026-09-26) — FIR only (`ErrUnsupportedDataType` otherwise); reads
    the hyperslab `[m,0,0]+[1,R,N]`, or copies `ImpulseResponses[m]` on an
    eager File. `TestReadMeasurementMatchesEager` checks every `m` of a
    synthetic contiguous file and first/middle/last plus the 354/355 chunk
    boundary of the chunked FIR CI fixtures: those chunk Data.IR across all
    measurements, so each read costs 10–30 ms (godoc and README say so).
- [x] **C3b. TF / TF-E / SOS measurement readers.** Siblings of
      `ReadMeasurement` for `Data.Real`/`Data.Imag` (TF-E in either axis
      order) and `Data.SOS`; `OpenLazy` already skips and checks them.
  - (2026-09-26) — `ReadMeasurementTF` (`[R][N]` real/imag),
    `ReadMeasurementTFE` (`[R][E][N]`, transposing a file stored
    `[M,R,N,E]`) and `ReadMeasurementSOS` share one hyperslab helper with
    `ReadMeasurement`. `TestReadMeasurementTypesMatchEager` compares every
    measurement of crafted TF, SOS and TF-E files (both axis orders,
    labelled and not) plus the TF CI fixture with `Open`, lazily and
    eagerly. `OpenLazy` now also checks each audio dataset's datatype from
    `Dataset.Info` (E4 workaround) and rejects what `Open` rejects
    (`TestOpenLazyRejectsNonNumericAudio`, the Codex finding on #13).
- [x] **C4. Range callback.** Add
      `(*File).RangeMeasurements(func(m int, ir [][]float64) error) error`
      for ergonomic iteration.
  - Acceptance: callback returning a non-nil error short-circuits and
    propagates; covered by `TestRangeMeasurementsAbort`.
  - (2026-09-26) — returns the callback's error unwrapped and read errors
    with the measurement index; `TestRangeMeasurementsVisitsAll` compares
    the full iteration with `Open`.
- [ ] **C5. Benchmark.** `go test -bench BenchmarkStreamVs Eager` over
      a synthetic ≥ 100 MB file generated in `TestMain`.
  - Acceptance: benchmark runs in CI under the `largefiles` build
    tag; results table appended to PLAN.md.

### Phase D — Extended testing & cross-validation

- [ ] **D1. Large-file build tag.** Introduce `//go:build largefiles`
      and split a new `sofa_largefiles_test.go` containing tests that
      exercise ≥ 100 MB synthetic files.
  - Acceptance: default `go test ./...` time unchanged (±10 %);
    `go test -tags largefiles ./...` runs the new suite green.
- [ ] **D2. MATLAB toolbox round-trip.** Document a manual procedure
      (script + commands) to round-trip a go-sofa-written file through
      MATLAB's SOFA Toolbox; commit the script under
      `scripts/matlab/`.
  - Acceptance: README has a "Cross-validation" section with the
    exact command sequence; reference output diff is bit-exact for
    `Data.IR`, `SourcePosition`, `ListenerPosition`.
- [ ] **D3. Benchmarks for write/read.** Add `BenchmarkWriteLarge`
      and `BenchmarkReadLarge` under the `largefiles` tag.
  - Acceptance: benchmark numbers (ns/op, MB/s) recorded in PLAN.md
    under a "Performance baseline" subsection so future regressions
    are visible.
- [ ] **D4. CI wiring.** Add a separate GitHub Actions job that runs
      `-tags largefiles -short=false` weekly (cron) only.
  - Acceptance: `.github/workflows/largefiles.yml` exists and is
    green on first run.

### Phase E — Cross-repo follow-ups in go-hdf5 (non-blocking)

These are upstream tasks; tracked here so go-sofa users can see why
certain features are absent.

- [ ] **E1. Dense storage for dataset attributes.** `WithAttribute`
      (go-hdf5 v0.15.0) caps at 8 attributes per dataset using compact
      storage. SOFA dimension scales never need more than 3, so this
      is fine today; extend the dense-storage path that already exists
      for root attributes if a future use case demands it.
  - Acceptance: upstream PR merged, version bumped in
    [go.mod](go.mod), and a regression test in this repo writes a
    dataset with 9+ attributes successfully.
- [ ] **E2. `DIMENSION_LIST` attribute on data datasets.** For full
      netCDF-4 parity, `Data.IR` / `Data.Real` etc. should carry a
      `DIMENSION_LIST` attribute (variable-length array of object
      references to dimension-scale datasets). Requires VLA + object
      reference support in go-hdf5 attribute encoding.
  - Acceptance: a go-sofa-written file passes
    `nc-config --has-nc4` netCDF-4 dimension scale validation
    (or `ncdump -h` shows attached dimension names) for `Data.IR`.
  - (2026-09-25) — covered by R1c: `ncdump -h` shows
    `Data.IR(M, R, N)`; the DIMENSION_LIST support is in the merged go-hdf5#1.
- [ ] **E3. go-hdf5 encoder/test defects found during R1c.** Not needed by
      go-sofa, but wrong for other users: `EncodeCompoundDatatypeV3` /
      `parseCompoundV3` put the member count in the properties (spec: class
      bit field) and use 4-byte member offsets; `CreateBasicDatatypeMessage`
      encodes integer bit precision in the offset byte;
      `TestGZIPFilter_CompressionRatio_Comparison` and (under `-race`)
      `TestMetricsCollector_Performance` fail on ca6206a already.
  - Acceptance: upstream fixes merged; a compound dataset written by
    `CreateCompoundDataset` opens in h5py.
- [ ] **E4. Public shape and REFERENCE_LIST access in go-hdf5.** go-sofa
      parses `Dataset.Info()` text for dataspace shapes and decodes
      `REFERENCE_LIST` bytes itself (compound reads of reference members are
      unsupported: "unsupported datatype class 6"). Add `Dataset.Shape()`
      and dimension-scale accessors upstream, then drop the parsers in
      [sofa_dataspace.go](sofa_dataspace.go) / [sofa_shapes.go](sofa_shapes.go).
- [ ] **E5. `Dataset.ReadSlice` returns zeros for 3-D+ contiguous
      hyperslabs that start inside a row.** Found in R8d (go-hdf5 v0.16.1):
      on a contiguous `[3,2,8]` dataset, `ReadSlice([0,0,5], [1,1,3])` or
      any non-zero start with a partial last axis returns `[0 0 0]` and no
      error; `readContiguousRowByRow` reads a dense bounding box from the
      start offset but extracts with absolute coordinates. Full-row
      selections (linear path) are correct, which sofaprobe relies on.
  - Acceptance: upstream fix and regression test merged; sofaprobe can
    read just the first/last 3 values.
- [ ] **E6. Chunk cache for hyperslab reads.** Found in C3 (go-hdf5
      v0.16.1): `ReadSlice` decompresses every chunk a selection touches on
      each call. SOFA Toolbox files chunk `Data.IR` as `[M,1,N]` (gzip), so
      `ReadMeasurement` costs a whole-dataset decompression per call
      (10–30 ms on the CI fixtures) and `RangeMeasurements` is quadratic.
  - Acceptance: upstream cache (e.g. last-N decompressed chunks per
    dataset) merged; reading all measurements of CIPIC lazily takes
    within 2× of `Open`.

---

## Risk Register

| Risk                                           | Impact | Mitigation                                      |
| ---------------------------------------------- | ------ | ----------------------------------------------- |
| go-hdf5 API changes                            | Medium | Pin dependency version; coordinate with fork.   |
| SOFA files using unsupported `DataType` values | Low    | FIR/TF/TF-E/SOS covered; SH tracked in Phase A. |
| SOFA convention evolution (2.0+)               | Low    | Phase A.                                        |
| go-hdf5 output unreadable by reference HDF5    | High   | R1a upstream fix + R1b interop CI job.          |
| Single-maintainer go-hdf5 fork (pre-1.0, OOMs) | High   | R2b upstream fixes; fuzzing in CI (R2c).        |

---

## References

- [SOFA Specifications](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications)
- [netCDF-4/HDF5 File Format](https://docs.unidata.ucar.edu/netcdf-c/current/file_format_specifications.html)
- [HDF5 Object Header Specification](https://docs.hdfgroup.org/hdf5/develop/_s_p_e_c.html#OHDRLayout)
