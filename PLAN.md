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

| Resource | Notes |
| -------- | ----- |
| `../PasSofa/Source/SofaFile.pas` | Our own SOFA reader (~430 LOC Pascal). Behavioural reference. |
| `../go-hdf` (go-hdf5 fork) | Pure-Go HDF5 read+write. Provides the low-level file access. |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based). |
| [libmysofa](https://github.com/hoene/libmysofa) | Lightweight C SOFA reader — useful as behavioural reference. |

---

## Open work

All work below is optional / future — nothing is blocking shipping. Each
phase is independent and can be picked up on demand when a real use case
appears.

### Phase A — SOFA 2.0 spherical-harmonic receiver representations

AES69-2022 introduced spherical-harmonic (SH) representations for
listener / receiver / source / emitter geometry, plus a `SH` family of
`DataType`s for SH-domain HRTFs. Today go-sofa only models Cartesian /
spherical Vector3 positions; SH coefficients are a third audio path
alongside FIR / TF. The repository does not yet contain a real SH
testdata file (see `#### SH layout reference` below for why).

#### SH layout reference

**Headline:** despite its filename, `testdata/sofa20_sh_test.sofa` is
**not** a spherical-harmonic file — it is a `SimpleFreeFieldHRTF` /
`DataType=TF` ARI HRTF measurement, fully covered by the existing TF
read/write path. Survey produced with
[`cmd/sofaprobe`](cmd/sofaprobe/main.go) and `h5dump -A -H`.

Root attributes:

| Attribute | Value |
| --------- | ----- |
| `Conventions` | `SOFA` |
| `SOFAConventions` | `SimpleFreeFieldHRTF` |
| `SOFAConventionsVersion` | `1.0` |
| `DataType` | `TF` |
| `Version` | `2.1` |
| `Title` | `HRTF` |
| `RoomType` | `free field` |
| `DatabaseName` | `ARI` |
| `ListenerShortName` | `nh4` |
| `Organization` | `Acoustics Research Institute, Austrian Academy of Sciences` |
| `DateCreated` | `2013-08-28` |
| `DateModified` | `2024-12-04` |

Dimension scales: `M=1550`, `R=2`, `N=4`, `C=3`, `E=1`, `I=1`,
`S=0` (unlimited).

Datasets:

| Path | Shape | Dtype | Key attributes |
| ---- | ----- | ----- | -------------- |
| `Data.Real` | `[1550][2][4]` | `float64` | dim refs `M, R, N` |
| `Data.Imag` | `[1550][2][4]` | `float64` | dim refs `M, R, N` |
| `SourcePosition` | `[1550][3]` | `float64` | `Type=spherical`, `Units=spherical` |
| `ReceiverPosition` | `[2][3][1]` | `float64` | `Type=cartesian`, `Units=metre` |
| `ListenerPosition` | `[1][3]` | `float64` | `Type=cartesian`, `Units=metre` |
| `EmitterPosition` | `[1][3][1]` | `float64` | `Type=cartesian`, `Units=metre` |
| `ListenerView` | `[1][3]` | `float64` | `Type=cartesian`, `Units=metre` |
| `ListenerUp` | `[1][3]` | `float64` | (no `Units`) |
| `N` | `[4]` | `float64` | `CLASS=DIMENSION_SCALE`, `Units=degree`, `LongName=Order` |

**Implication:** there is no SH coefficient dataset, no `SHOrder`
attribute, no SH-specific dimension in this file. To re-run this
survey on any candidate file:
`go run ./cmd/sofaprobe <file.sofa>` or
`h5dump -A -H <file.sofa>`.

##### `testdata/demo_FreeFieldHRTF_4_SH.sofa` (sourced by A1b)

Downloaded 2026-05-09 from
`https://www.sofaconventions.org/data/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa`
(SOFA Toolbox demo step 4, License: CC 3.0 BY-SA).

| Attribute | Value |
| --------- | ----- |
| `Conventions` | `SOFA` |
| `SOFAConventions` | `FreeFieldHRTF` |
| `SOFAConventionsVersion` | `1.0` |
| `DataType` | **`TF-E`** (not `SH`) |
| `History` | `…/ Converted to TF / Converted to TFE / Converted to Spherical Harmonics` |
| `RoomType` | `free field` |
| `Organization` | `ARI/ÖAW` |
| `License` | `CC 3.0 BY-SA` |

Dimension scales: `M=1`, `R=2`, `N=129` (frequency bins),
**`E=1156` (= 34² SH coefficients, so SH order Lmax=33)**, `C=3`,
`I=1`, `S=…`.

Datasets of interest:

| Path | Shape | Dtype |
| ---- | ----- | ----- |
| `Data.Real` | `[1][2][129][1156]` | `float64` |
| `Data.Imag` | `[1][2][129][1156]` | `float64` |
| `EmitterPosition` | `[1][3][1]` | `float64` |
| `ReceiverPosition` | `[2][3][1]` | `float64` |

**Critical scope finding for Phase A.** AES69's public test corpus
does *not* expose a `DataType="SH"` value. The SH representation is
achieved by:

1. Keeping `DataType=TF-E`,
2. Reusing the `E` (emitter) dimension as the SH coefficient index
   `c ∈ [0, (Lmax+1)²)`,
3. Letting the convention name (here `FreeFieldHRTF`) plus
   `History` text declare the SH semantic.

This contradicts the original Phase A design (which assumed a
separate `DataType=SH`). Practical consequence: tasks A2/A3/A4/A6
need re-scoping before implementation. See the **scope review** note
just above the Tasks list below.

> **Scope reframed (2026-05-09).** A1b found that real-world SH SOFA
> files use `DataType=TF-E` with the `E` dimension as SH coefficient
> index, not a separate `DataType=SH`. Phase A is therefore reframed
> as **convention-aware SH support layered on existing TF-E I/O**: no
> new DataType, no new wire format, no new write path — just typed
> accessors that interpret an SH-flavoured TF-E file as SH
> coefficients. The originally-added `dataTypeSH` constant has been
> reverted.

Tasks (each is an independent commit; tick as completed):

- [x] **A1. Survey misnamed reference file.** Inspected
      `testdata/sofa20_sh_test.sofa` (turned out to be plain TF);
      results recorded in the "SH layout reference" subsection above.
- [x] **A1b. Source real SH testdata file.** Downloaded
      `testdata/demo_FreeFieldHRTF_4_SH.sofa` (4.2 MB, CC 3.0 BY-SA)
      from sofaconventions.org/data/sofatoolbox_test/; structure
      documented in the SH layout reference subsection above. Found
      it uses `DataType=TF-E`, which drove the Phase A reframe.
- [x] **A2. ~~Add `DataType` constants.~~** Reverted. Reason: AES69
      does not define `DataType=SH` in practice. SH is a convention
      semantic over TF-E, not a top-level DataType.
- [x] **A3. SH detection helpers.** Added `IsSHEncoded()`,
      `SHOrder()`, `SHCoefficientCount()` in
      [sofa_sh.go](sofa_sh.go); table-driven tests in
      [sofa_sh_test.go](sofa_sh_test.go) cover plain HRTF, Lmax=1,
      Lmax=33, non-perfect-square E, non-SH convention, and
      case-insensitive matching.
- [x] **A4. Read test against the SH testdata file.**
      `TestReadSHEncodedTFE` in
      [sofa_sh_test.go](sofa_sh_test.go) opens
      `testdata/demo_FreeFieldHRTF_4_SH.sofa` (skipped when absent)
      and confirms `DataType=TF-E`, `E=1156=34²`, `N=129`, and that
      `TFRealE` is shaped `[M][R][E][N]`. No new read code needed —
      validated A2's reframe decision. Note: the demo file uses
      convention `FreeFieldHRTF` (not `*HRSH*`), so
      `IsSHEncoded()` returns `false` on it; A7 will add the HRSH
      convention name and at that point a sibling round-trip test
      can assert true.
- [x] **A5. SH-aware validation warnings.** Added
      `(*File).SHWarnings() []string` in
      [sofa_sh.go](sofa_sh.go) covering three cases: claims SH but
      DataType ≠ TF-E; claims SH but E ≠ (L+1)²; perfect-square E on
      TF-E with no SH claim (false-positive flag). Wired into
      `cmd/sofainfo` ([cmd/sofainfo/main.go:130-137](cmd/sofainfo/main.go))
      after the existing dimension printout. Detection in `SHOrder()`
      was extended to also match the History attribute (case-insensitive
      "spherical harmonic"), so files like the demo that carry SH
      semantics in History rather than the convention name are picked
      up. `go run ./cmd/sofainfo testdata/demo_FreeFieldHRTF_4_SH.sofa`
      prints `SH-encoded HRTF: Lmax=33, 1156 coefficients` and zero
      warnings; plain-FIR files (e.g. `MIT_KEMAR_normal_pinna.sofa`)
      print neither. False-positive path covered by
      `TestSHWarnings/false_positive...` in
      [sofa_sh_test.go](sofa_sh_test.go).
- [x] **A6. ~~SH write path~~ (no new write code).**
      `TestWriteSHEncodedRoundTrip` in
      [sofa_sh_test.go](sofa_sh_test.go) builds a
      `File{DataType:"TF-E", SOFAConventions:"FreeFieldHRSH", E:9, N:4, …}`,
      saves via the existing TF-E writer, reopens, and confirms
      `IsSHEncoded()==true`, `SHOrder()==(2, true)`,
      `SHCoefficientCount()==9`, no warnings, plus bit-exact
      `TFRealE`/`TFImagE` (Δ < 1e-12) across all 72 coefficients.
      No code changes outside the test — the convention-aware reframe
      from A2/A3 means the TF-E writer already handles this case.
      README pointer for users still TODO (small follow-up under A7).
- [ ] **A7. Convention table entry.** Add `SimpleFreeFieldHRSH` (and
      any other `*HRSH` variants the upstream SOFA Toolbox lists) to
      whatever convention enumeration exists in this repo; flag E as
      "SH coefficient index" in godoc.
  - Acceptance: `grep -rn HRSH .` shows the new convention name in
      the convention table; opening the A1b file does not produce an
      "unknown convention" warning.
- [ ] **A8. Coverage check.** `go test -cover ./...` ≥ 80 % overall;
      new SH-helper functions individually ≥ 90 % covered (small
      pure functions — should be trivial).

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

| Risk | Impact | Mitigation |
| ---- | ------ | ---------- |
| go-hdf5 API changes | Medium | Pin dependency version; coordinate with fork. |
| SOFA files using unsupported `DataType` values | Low | FIR/TF/TF-E/SOS covered; SH tracked in Phase A. |
| SOFA convention evolution (2.0+) | Low | Phase A. |

---

## References

- [SOFA Specifications](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications)
- [netCDF-4/HDF5 File Format](https://docs.unidata.ucar.edu/netcdf-c/current/file_format_specifications.html)
- [HDF5 Object Header Specification](https://docs.hdfgroup.org/hdf5/develop/_s_p_e_c.html#OHDRLayout)
- [Detailed write-support implementation notes](/home/christian/.claude/plans/sofa-write-implementation.md)
