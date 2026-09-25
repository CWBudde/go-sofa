# go-sofa — SOFA (AES69) File Reader/Writer in Go

## Goal

A pure-Go library for reading and writing SOFA files (AES69 — Spatially
Oriented Format for Acoustics), built on top of
[MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5)
(our fork of scigolib/hdf5). The Go equivalent of
`PasSofa/SofaFile.pas`.

## Status

Read, write, CLI tools, CI, lint, and the `FIR`, `TF`, `TF-E`, and `SOS`
`DataType`s are implemented. The 2026-09-24 review (Phase R below) found
release-blocking defects (R1–R4), all fixed: `Save` output now opens in
h5py/netCDF4 with named netCDF-4 dimensions (checked in CI), tests fetch
their reference data, and crafted input no longer panics or OOMs.
Phase R takes precedence over the remaining Phases C–E. See `git log` for history; this
file tracks only what's still open.

## Prior Art & Key Resources

| Resource                                                                                | Notes                                                         |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `../PasSofa/Source/SofaFile.pas`                                                        | Our own SOFA reader (~430 LOC Pascal). Behavioural reference. |
| `../go-hdf` (go-hdf5 fork)                                                              | Pure-Go HDF5 read+write. Provides the low-level file access.  |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based).                |
| [libmysofa](https://github.com/hoene/libmysofa)                                         | Lightweight C SOFA reader — useful as behavioural reference.  |

---

## Open work

Phase R: the release blockers R1–R4 are done (go-hdf5 fixes in
[CWBudde/go-hdf5#1](https://github.com/CWBudde/go-hdf5/pull/1) and the
stacked `feat/dimension-scales` branch, consumed via a pseudo-version until
they are merged and tagged). R5–R9 remain. Phases B–E are optional /
future and can be picked up on demand when a real use case appears.

### Phase R — Review findings 2026-09-24 (blocking)

Multi-area review (read path 4/10, write path 2/10, API/CLI 4/10,
tests/tooling 3/10; overall ≈3/10). Items are ordered by priority;
R1–R4 are release blockers.

#### R1 — Interoperability of written files (critical)

- [x] **R1a. Fix go-hdf5 output so reference HDF5 can open it.** Every file
      written by `Save` — and even an empty go-hdf5 `CreateForWrite`+`Close`
      file — fails in h5py/HDF5 1.12/1.14/2.0 and netCDF-C 4.9 with
      `actual len exceeds EOA` / `NetCDF: HDF error`. Suspected cause: root
      OHDR v2 chunk/checksum extends past the superblock EOA. Fix upstream
      in go-hdf5, bump [go.mod](go.mod).
  - Acceptance: `h5py.File(out)` and `netCDF4.Dataset(out)` open FIR, TF,
    TF-E and SOS files written by `Save`; `h5dump -H` exits 0.
- [x] **R1b. Interop CI job.** Add a CI step (Python + h5py + netCDF4) that
      writes one file per DataType via a small Go program and opens/reads it
      back with h5py and netCDF4, comparing values.
  - Acceptance: job fails on current `main`, passes after R1a.
- [x] **R1c. Real netCDF-4 dimensions.** Dimension-scale datasets (`/M`,
      `/R`, `/E`, `/N`, plus missing `/I`, `/C`) must have length equal to the
      dimension (currently shape `[1]` holding the size,
      `writeDimensionScale`); add `_Netcdf4Dimid`, `DIMENSION_LIST` /
      `REFERENCE_LIST`, `_NCProperties`; attach position variables to `M|I`
      and `C` (subsumes Phase E2).
  - Acceptance: `ncdump -h` on a written file shows `M`, `R`, `N`, `C`, `I`
    with correct lengths and named (non-phony) dims on every variable.
  - (2026-09-25) — go-hdf5 gained `FileWriter.AttachDimensionScale` /
    `DatasetWriter.Address` (DIMENSION_LIST + REFERENCE_LIST written on
    Close), libhdf5-readable VLEN data and >255-byte dataset headers
    (branch `feat/dimension-scales`, stacked on go-hdf5#1). `Save` now
    writes `M R E N C I` scales of full length with the netCDF-C `NAME`
    and `_Netcdf4Dimid`, `_NCProperties`, and creates every variable from
    named dimensions (`writeDimensionScales` / `writeVariable` in
    [sofa.go](sofa.go)); an M×R `Data.Delay` is written `[M,R]`.
    `ncdump -h` (netCDF-C 4.9.3) on FIR/TF/TF-E/SOS output lists
    `M R E N C I` and only named dims; `TestSaveWritesNetcdf4Dimensions`
    and the interop job (`just interop`, now rejecting `phony_dim`) cover it.

#### R2 — Crash safety on untrusted input (critical)

- [x] **R2a. Validate dimensions.** `parseDimensionSize` / `readDimensions`
      must reject values ≤ 0, NaN/Inf and products that overflow `int` or
      exceed a sane cap, _before_ any `Read`. Today `M=-1,R=-2` passes the
      `M*R*N == len` check and `reshapeIR` panics (`makeslice`).
- [x] **R2b. Upstream OOM in go-hdf5.** A 60 s `FuzzOpen` hits
      `fatal error: out of memory` (unrecoverable) in
      `internal/structures/localheap.go:94` and
      `internal/core/dataset_reader.go:86`. Bound allocations by file size
      upstream; file issues and link them here.
- [x] **R2c. `FuzzOpen` target** committed in the repo with a seed corpus of
      small valid files and the crashers found so far.
  - Acceptance: `go test -fuzz FuzzOpen -fuzztime 5m` runs clean.

#### R3 — Test suite must pass on a fresh clone (critical)

- [x] **R3a.** `/testdata/` is in `.gitignore`, ~18 tests fail with ENOENT
      and CI (`test-unit.yaml`) is therefore red. Either commit small
      reference files (with `testdata/PROVENANCE.md` listing source URL,
      licence, SHA-256) or add `just fetch-testdata` run by CI, and make
      data-dependent tests `t.Skip` locally (not fail) when data is absent.
- [x] **R3b. Third-party fixtures.** Include at least one file each from
      SOFA API (Matlab/Octave), SOFAtoolbox, libmysofa test set and
      netCDF-C/pysofaconventions — today every write test is a self
      round-trip through go-hdf5's own reader.
- [x] **R3c.** Remove the silent `t.Skip` in
      `hdf5_validation_test.go:31-33` (it hides the failure it tests for).
- [x] **R3d.** Add a `LICENSE` file (README links a non-existent one; no
      licence = not reusable).

#### R4 — Save durability and honesty (high)

- [x] **R4a.** Return the `fw.Close()` error from `Save` (currently
      `defer fw.Close()` swallows flush/disk-full errors → nil on a
      truncated file).
- [x] **R4b.** Write to a temp file in the same directory, `fsync`, then
      `os.Rename` over the target, so a failed Save leaves the original
      intact — as README and the `Save` godoc already (falsely) claim.
- [x] **R4c.** Deterministic output: write dimension scales in fixed order
      instead of iterating a map (currently 3 distinct md5s in 4 runs).

#### R5 — Read-path correctness (high)

- [ ] **R5a. Accessor panics.** `IRAt`/`IRPeakdB` index
      `ImpulseResponses` bounded by `f.M`/`f.R`; on TF/TF-E/SOS files the
      slice is empty → panic. Check `DataType` / slice length and return an
      error. `Duration()` must return an error/ok for non-FIR.
- [ ] **R5b. Slice aliasing.** `reshapeIR`/`reshape4D` hand out
      `flat[s:s+n]` with spare capacity, so `append` on one row overwrites
      the next. Use full slice expressions `flat[s:s+n:s+n]`.
- [ ] **R5c. Unknown DataType.** Stop defaulting unknown/empty `DataType`
      to FIR; return a typed `ErrUnsupportedDataType`. Explicitly handle or
      reject `FIR-E` (GeneralFIR-E) and legacy `FIRE`.
- [ ] **R5d. Shape-aware reads.** Check dataset _shapes_, not only total
      element count (any axis permutation is accepted today). Support
      `ReceiverPosition` `[R,C,M]` / `EmitterPosition` `[E,C,M]` (currently
      silently misread as R·M vectors) and `ListenerView/Up` `[M,C]`
      (currently truncated to element 0).
- [ ] **R5e. Broadcasting helpers.** `SourcePositionAt(m)`,
      `DelayAt(m, r)`, `SamplingRateAt(m)` resolving I- vs M-sized
      variables; make `SamplingRateScalar` report when rates vary.
- [ ] **R5f. Stop swallowing errors.** Attribute read errors
      (`readGlobalAttributes` `continue`) and position read errors
      (`readSpatialData`) must propagate or be collected as warnings.
- [ ] **R5g. SH detection per spec.** Use
      `EmitterPosition:Type == "spherical harmonics"` as the primary signal;
      demote "SH"-substring / History heuristics; allow `E=1` (order 0);
      require `DataType == TF-E` in `SHOrder`.

#### R6 — AES69 conformance of written files (medium)

- [ ] **R6a.** Emit mandatory global attributes (`DateCreated`,
      `DateModified`, `APIName`, `APIVersion`, `AuthorContact`,
      `Organization`, `License`, `Title`, `RoomType`), defaulting
      `APIName`/`APIVersion`/dates when empty; require
      `SOFAConventionsVersion`.
- [ ] **R6b.** Add required variable attributes: `Data.SamplingRate:Units`,
      `N:Units`/`LongName` for TF, Type/Units for `ListenerView/Up`;
      validate position `Type` ∈ {cartesian, spherical, spherical
      harmonics} and require it.
- [ ] **R6c.** Write `Data.Delay` as `[I,R]` or `[M,R]` (2-D), make it
      mandatory for FIR/SOS, remove the M==R ambiguity.
- [ ] **R6d.** Validate data: reject NaN/Inf where not allowed,
      SamplingRate ≤ 0, zero View/Up vectors, non-monotonic frequencies,
      and convention-specific constraints (e.g. `SimpleFreeFieldHRIR`
      requires R=2, E=1) — overlaps Phase B.
- [ ] **R6e. Lossless round-trip.** Preserve unknown global attributes,
      extra variables and variable attributes; stop lowercasing
      `Type`/`Units` on read (normalise only for comparisons).

#### R7 — API ergonomics (medium)

- [ ] **R7a.** Sentinel/typed errors (`ErrNotSOFA`,
      `ErrUnsupportedDataType`, `*ValidationError{Field}`) usable with
      `errors.Is/As`; capitalise field names in messages.
- [ ] **R7b.** Export DataType constants (`DataTypeFIR`, …).
- [ ] **R7c.** `io.ReaderAt`/`io.Writer` entry points (`Read(r)`,
      `(*File).WriteTo(w)`), if go-hdf5 allows.
- [ ] **R7d.** Release the HDF5 handle after eager `Open` (or make `Close`
      meaningful via Phase C lazy mode).
- [ ] **R7e.** Fix godoc: `Vector3` "in meters" (wrong for spherical),
      `File` "an open SOFA file", `Delay` shape.
- [ ] **R7f.** Consider replacing parallel per-DataType fields (`TFReal` vs
      `TFRealE`, …) with a `Data` interface / tagged union before v1 to
      avoid API churn.

#### R8 — CLIs (medium)

- [ ] **R8a.** Use the `flag` package: `-h/--help`, usage text, reject
      unknown flags; process _all_ file arguments (sofa2json/sofainfo
      silently ignore all but the first).
- [ ] **R8b.** Non-zero exit if any file fails; progress to stderr.
- [ ] **R8c.** sofa2json: handle NaN/Inf (e.g. `null` or string), include
      `Conventions`, `SOFAConventions`, versions, positions and coordinate
      Type/Units; consistent keys matching library field names; don't
      silently overwrite (`-f` to force); stream output instead of
      `MarshalIndent` of the whole document.
- [ ] **R8d.** sofainfo: show `SOFAConventions`, `Version`, full delay
      summary; sofaprobe: support Real/Imag/SOS, don't read all of
      `Data.IR` to print 6 values.
- [ ] **R8e.** Smoke tests for each CLI (currently 0 % coverage).

#### R9 — Docs, tooling, CI hygiene (low)

- [ ] **R9a.** README: fix module path (`github.com/cwbudde/go-sofa`, not
      `MeKo-Christian`) in all `go get`/`go install`/import lines and
      go-hdf5 links; "reading" → "reading and writing"; update "Known
      limitations" (CLASS/NAME are emitted), supported DataTypes, document
      `--include-sos`.
- [ ] **R9b.** justfile: `GOPRIVATE` points at the wrong owner;
      `just build` builds only sofaprobe; add `-race` to `just test`.
- [ ] **R9c.** CI: trigger on `pull_request`; pin tool and golangci-lint
      versions; enforce a coverage floor; install shellcheck or drop it
      from treefmt; optional Go-version matrix.
- [ ] **R9d.** Tag `v0.1.0` only after R1–R4 (CHANGELOG documents a
      release that has no tag).
- [ ] **R9e.** Coverage gaps: `readGlobalAttributes` 50 % (15 of 20
      attributes never read in tests), `Save` error branches,
      `write*AudioDatasets` 66–71 %.

### Phase A — SH HRTF support ✅ done 2026-05-10

Convention-aware spherical-harmonic accessors layered on the existing
TF-E I/O. No new DataType, no new wire format, no new write path. See
[`sofa_sh.go`](sofa_sh.go), [`sofa_sh_test.go`](sofa_sh_test.go),
README §"Spherical-harmonic (SH) HRTFs".

**Key finding.** Real-world SH SOFA files use `DataType=TF-E` with
the `E` (emitter) dimension as SH coefficient index
(`E = (Lmax+1)²`); SH semantics are declared via convention name
(`*HRSH*`) or `History` ("Converted to Spherical Harmonics"). AES69's
public test corpus does **not** expose `DataType="SH"` — the original
plan's separate-DataType design was wrong, hence the reframe.

**API surface** (`*File`): `IsSHEncoded()`, `SHOrder() (lmax, ok)`,
`SHCoefficientCount()`, `SHWarnings() []string`. Wired into
`cmd/sofainfo`. Coverage: package 80.1 %, all six SH helpers 100 %.

#### Reference files

- `testdata/sofa20_sh_test.sofa` — **misnamed**; actually
  `SimpleFreeFieldHRTF` / `DataType=TF`, no SH content.
- `testdata/demo_FreeFieldHRTF_4_SH.sofa` (4.2 MB, CC 3.0 BY-SA,
  from `sofaconventions.org/data/sofatoolbox_test/`, downloaded
  2026-05-09) — `DataType=TF-E`, `E=1156=34²` (Lmax=33), `N=129`,
  `M=1`, `R=2`; SH semantic in `History`, not in convention name.
  Re-survey any candidate via `go run ./cmd/sofaprobe <file>` or
  `h5dump -A -H <file>`.

#### Tasks

- [x] A1 — survey misnamed `sofa20_sh_test.sofa`
- [x] A1b — source real SH testdata (drove the reframe)
- [x] A2 — `DataType=SH` constant _(reverted; SH is convention-level)_
- [x] A3 — `IsSHEncoded` / `SHOrder` / `SHCoefficientCount`
- [x] A4 — read test against demo file (`TestReadSHEncodedTFE`)
- [x] A5 — `SHWarnings` + `cmd/sofainfo` integration; History-based
      detection
- [x] A6 — write round-trip via existing TF-E writer
      (`TestWriteSHEncodedRoundTrip`, Δ < 1e-12)
- [x] A7 — README "Spherical-harmonic (SH) HRTFs" subsection + godoc
      on `File.E` / `File.SOFAConventions`
- [x] A8 — coverage ≥ 80 % overall (80.1 %, 434/542 stmts) with SH
      helpers ≥ 90 % (all 100 %)

References: AES69-2022, sofaconventions.org SH page,
`testdata/demo_FreeFieldHRTF_4_SH.sofa` History attribute.

### Phase B — Specialised SOFA convention behaviour ✅ done 2026-09-25

Today only the convention name is stored as a string; behaviour is
generic across all conventions. Specialise where it would catch real
errors or add useful structure. We have testdata for SRIR
(`SingleRoomSRIR_1.1.sofa`) and BRIR-like files; Directivity needs an
example file before starting.

Tasks (one sub-bullet per convention; pick whichever has demand first):

- [x] **B1. Convention dispatcher.** Introduce a small registry
      `map[string]conventionRules` keyed by `SOFAConventions` attribute
      and looked up after generic validation in `validate`
      (`validate` in [sofa.go](sofa.go)).
  - Acceptance: unknown conventions still pass through unchanged
    (back-compat); `TestUnknownConventionStillReads` passes.
  - (2026-09-25) — `conventionRegistry` + `validateConvention` in
    [`sofa_conventions.go`](sofa_conventions.go), called at the end of
    `validate` (so on `Save` only; `Open` never validates). Registry
    starts empty; B2/B3 add the first entries. Covered by
    `TestUnknownConventionStillReads` and `TestConventionRulesDispatch`.
- [x] **B2. BRIR rules.** Validator requires `RoomType` attribute and
      `ListenerView`/`ListenerUp` to be non-zero. Add typed accessor
      `(*File).IsBRIR() bool`.
  - Acceptance: `TestBRIRMissingRoomType` errors with a message
    containing `"RoomType"`; round-trip via `MIT_KEMAR_normal_pinna.sofa`
    or equivalent BRIR file remains green.
  - (2026-09-25) — [`sofa_brir.go`](sofa_brir.go): `IsBRIR()` and a
    validator registered for `SingleRoomDRIR` and `MultiSpeakerBRIR`.
    MIT KEMAR is HRIR, so the round-trip uses `testdata/OfficeII.sofa`
    (Kayser 2009 BRIR, `SingleRoomDRIR`, M=8, R=8) instead
    (`TestBRIRRoundTripOfficeII`). Also `TestBRIRMissingRoomType`,
    `TestBRIRZeroListenerOrientation`.
- [x] **B3. SRIR rules.** Validator checks for `RoomVolume` /
      `RoomTemperature` (warn, not error) and that `R` matches an
      Ambisonics order convention `(N+1)^2`.
  - Acceptance: `TestSRIRReadKnownFile` opens
    `testdata/SingleRoomSRIR_1.1.sofa`, no error, exposes detected
    Ambisonics order via a new `(*File).AmbisonicsOrder() (int, bool)`
    accessor.
  - (2026-09-25) — [`sofa_srir.go`](sofa_srir.go): `IsSRIR()`,
    `AmbisonicsOrder()`, and warnings-only rules for `SingleRoomSRIR` /
    `SingleRoomMIMOSRIR`, surfaced via the new
    `(*File).ConventionWarnings()` (printed by `sofainfo`). A non-square
    `R` warns rather than errors, since raw-capsule arrays are valid
    SRIR. New `File.RoomVolume` / `RoomTemperature` fields are read
    (variable, falling back to a root attribute) and written. The
    testdata file is a demo with R=1, so it reports order 0.
  - Follow-up: `SimpleFreeFieldSOS_1.0.sofa` (RoomVolume as attribute)
    cannot be opened — go-hdf5 fails with "only depth-0 B-trees
    supported" — so the attribute fallback is unit-tested only.
- [x] **B4. Directivity rules.** Document that `M` indexes source
      orientation; add `(*File).IsDirectivity() bool`. Skip validator
      until we have a test file.
  - Acceptance: README "Conventions" section lists Directivity with
    a "needs example file" note; godoc on the accessor explains the
    semantic difference.
  - (2026-09-25) — [`sofa_directivity.go`](sofa_directivity.go) with
    godoc; README "### Conventions" table covers BRIR, SRIR, and
    Directivity ("needs example file").
- [x] **B5. Coverage.** New per-convention code ≥ 80 % covered; total
      project coverage does not regress.
  - (2026-09-25) — `sofa_brir.go`, `sofa_conventions.go`,
    `sofa_directivity.go` 100 %, `sofa_srir.go` 94.4 % (only
    `writeRoomScalars` HDF5 error branches uncovered); total 83.1 % →
    83.7 %.

### Phase C — Streaming / partial reads

Hyperslab reads would let consumers stream a subset (e.g. one
measurement at a time) instead of loading the whole `[M][R][N]` array
into memory. Useful for very large HRTF databases.

Tasks:

- [ ] **C1. Upstream capability check.** Inspect the go-hdf5 API for
      hyperslab / partial-read support; if missing, file an upstream
      issue (link it back here) before continuing.
  - Acceptance: this PLAN cites either the supporting go-hdf5 API or
    the tracking issue URL.
- [ ] **C2. Lazy `File` mode.** Add
      `OpenLazy(path string) (*File, error)` that parses metadata but
      leaves audio datasets unloaded. Existing `Open` keeps eager
      semantics.
  - Acceptance: `TestOpenLazyDoesNotAllocateAudio` opens a >10 MB
    file and asserts `len(f.ImpulseResponses)==0` plus `runtime.MemStats`
    delta below an eager-open baseline by ≥ 50 %.
- [ ] **C3. Per-measurement reader.** Implement
      `(*File).ReadMeasurement(m int) ([][]float64, error)` shaped
      `[R][N]` for FIR, with sibling helpers for TF/SOS as needed.
  - Acceptance: `TestReadMeasurementMatchesEager` loads the same file
    eagerly and via `ReadMeasurement` for every `m`, asserts deep
    equality.
- [ ] **C4. Range callback.** Add
      `(*File).RangeMeasurements(func(m int, ir [][]float64) error) error`
      for ergonomic iteration.
  - Acceptance: callback returning a non-nil error short-circuits and
    propagates; covered by `TestRangeMeasurementsAbort`.
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
    `Data.IR(M, R, N)`. Left unticked until the go-hdf5 PR is merged.
- [ ] **E3. go-hdf5 encoder/test defects found during R1c.** Not needed by
      go-sofa, but wrong for other users: `EncodeCompoundDatatypeV3` /
      `parseCompoundV3` put the member count in the properties (spec: class
      bit field) and use 4-byte member offsets; `CreateBasicDatatypeMessage`
      encodes integer bit precision in the offset byte;
      `TestGZIPFilter_CompressionRatio_Comparison` and (under `-race`)
      `TestMetricsCollector_Performance` fail on ca6206a already.
  - Acceptance: upstream fixes merged; a compound dataset written by
    `CreateCompoundDataset` opens in h5py.

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
