# go-sofa — SOFA (AES69) File Reader/Writer in Go

## Goal

A pure-Go library for reading and writing SOFA files (AES69 — Spatially
Oriented Format for Acoustics), built on top of
[MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5)
(our fork of scigolib/hdf5). The Go equivalent of
`PasSofa/SofaFile.pas`.

## Status

Read, write, CLI tools, CI, lint, ≥80 % test coverage, and the `FIR`,
`TF`, `TF-E`, and `SOS` `DataType`s are complete. See `git log` for
history; this file tracks only what's still open.

## Prior Art & Key Resources

| Resource                                                                                | Notes                                                         |
| --------------------------------------------------------------------------------------- | ------------------------------------------------------------- |
| `../PasSofa/Source/SofaFile.pas`                                                        | Our own SOFA reader (~430 LOC Pascal). Behavioural reference. |
| `../go-hdf` (go-hdf5 fork)                                                              | Pure-Go HDF5 read+write. Provides the low-level file access.  |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based).                |
| [libmysofa](https://github.com/hoene/libmysofa)                                         | Lightweight C SOFA reader — useful as behavioural reference.  |

---

## Open work

All work below is optional / future — nothing is blocking shipping. Each
phase is independent and can be picked up on demand when a real use case
appears.

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

### Phase B — Specialised SOFA convention behaviour

Today only the convention name is stored as a string; behaviour is
generic across all conventions. Specialise where it would catch real
errors or add useful structure. We have testdata for SRIR
(`SingleRoomSRIR_1.1.sofa`) and BRIR-like files; Directivity needs an
example file before starting.

Tasks (one sub-bullet per convention; pick whichever has demand first):

- [ ] **B1. Convention dispatcher.** Introduce a small registry
      `map[string]conventionRules` keyed by `SOFAConventions` attribute
      and looked up after generic validation in `validate`
      ([sofa.go:755](sofa.go#L755)).
  - Acceptance: unknown conventions still pass through unchanged
    (back-compat); `TestUnknownConventionStillReads` passes.
- [ ] **B2. BRIR rules.** Validator requires `RoomType` attribute and
      `ListenerView`/`ListenerUp` to be non-zero. Add typed accessor
      `(*File).IsBRIR() bool`.
  - Acceptance: `TestBRIRMissingRoomType` errors with a message
    containing `"RoomType"`; round-trip via `MIT_KEMAR_normal_pinna.sofa`
    or equivalent BRIR file remains green.
- [ ] **B3. SRIR rules.** Validator checks for `RoomVolume` /
      `RoomTemperature` (warn, not error) and that `R` matches an
      Ambisonics order convention `(N+1)^2`.
  - Acceptance: `TestSRIRReadKnownFile` opens
    `testdata/SingleRoomSRIR_1.1.sofa`, no error, exposes detected
    Ambisonics order via a new `(*File).AmbisonicsOrder() (int, bool)`
    accessor.
- [ ] **B4. Directivity rules.** Document that `M` indexes source
      orientation; add `(*File).IsDirectivity() bool`. Skip validator
      until we have a test file.
  - Acceptance: README "Conventions" section lists Directivity with
    a "needs example file" note; godoc on the accessor explains the
    semantic difference.
- [ ] **B5. Coverage.** New per-convention code ≥ 80 % covered; total
      project coverage does not regress.

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
- [ ] **C2. Lazy `File` mode.** Add `OpenLazy(path string) (*File,
  error)` that parses metadata but leaves audio datasets unloaded.
      Existing `Open` keeps eager semantics.
  - Acceptance: `TestOpenLazyDoesNotAllocateAudio` opens a >10 MB
    file and asserts `len(f.ImpulseResponses)==0` plus `runtime.MemStats`
    delta below an eager-open baseline by ≥ 50 %.
- [ ] **C3. Per-measurement reader.** Implement
      `(*File).ReadMeasurement(m int) ([][]float64, error)` shaped
      `[R][N]` for FIR, with sibling helpers for TF/SOS as needed.
  - Acceptance: `TestReadMeasurementMatchesEager` loads the same file
    eagerly and via `ReadMeasurement` for every `m`, asserts deep
    equality.
- [ ] **C4. Range callback.** Add `(*File).RangeMeasurements(func(m
  int, ir [][]float64) error) error` for ergonomic iteration.
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

---

## Risk Register

| Risk                                           | Impact | Mitigation                                      |
| ---------------------------------------------- | ------ | ----------------------------------------------- |
| go-hdf5 API changes                            | Medium | Pin dependency version; coordinate with fork.   |
| SOFA files using unsupported `DataType` values | Low    | FIR/TF/TF-E/SOS covered; SH tracked in Phase A. |
| SOFA convention evolution (2.0+)               | Low    | Phase A.                                        |

---

## References

- [SOFA Specifications](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications)
- [netCDF-4/HDF5 File Format](https://docs.unidata.ucar.edu/netcdf-c/current/file_format_specifications.html)
- [HDF5 Object Header Specification](https://docs.hdfgroup.org/hdf5/develop/_s_p_e_c.html#OHDRLayout)
- [Detailed write-support implementation notes](/home/christian/.claude/plans/sofa-write-implementation.md)
