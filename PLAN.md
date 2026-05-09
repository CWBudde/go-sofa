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
alongside FIR / TF.

Concrete tasks:

- Decide how to surface SH on the `File` struct: extend `Vector3` vs.
  a parallel `[]float64` "coefficients" slot per role, plus an `Order`
  field. Probably a separate `ReceiverSHCoefficients [R][C]` etc.
- Add `DataType == "SH"` (and any sub-variants) to the read / write /
  validate dispatcher; introduce a `Phase`-style audio reader if the
  data layout differs from `[M][R][N]`.
- Convention bindings: `SimpleFreeFieldHRSH`, `GeneralSOS` SH variants,
  whatever AES69-2022 names finally emerge.
- Tests: synthetic round-trip; real testdata once we have a small
  example file.

References: AES69-2022, sofaconventions.org SH page.

### Phase B — Specialised SOFA convention behaviour

Today only the convention name is stored as a string; behaviour is
generic across all conventions. Specialise where it would catch real
errors or add useful structure:

- **BRIR** (Binaural Room Impulse Response) — listener-relative IR
  with optional room metadata.
- **SRIR** (Spatial Room Impulse Response) — A/B-format or Ambisonics
  impulse responses.
- **Directivity** — single-source radiation patterns; `M` indexes
  source orientation, not measurement position.

Each would likely add: a small validator for required metadata, a
typed accessor or two on `File`, and one round-trip test.

### Phase C — Streaming / partial reads

Hyperslab reads would let consumers stream a subset (e.g. one
measurement at a time) instead of loading the whole `[M][R][N]` array
into memory. Useful for very large HRTF databases.

Concrete tasks:

- Confirm go-hdf5 exposes hyperslab / partial-read support, or add it
  there first.
- Define an iterator API on `File`: e.g. `ReadMeasurement(m int)
  ([R][N]float64, error)` or a `Range(func(m int, ir [R][N]float64))`
  callback.
- Keep the existing eager `ImpulseResponses` / `TFReal` arrays; new
  API is purely additive.
- Benchmark: streaming vs. eager on a >100 MB file.

### Phase D — Extended testing & cross-validation

- Large-file integration tests (>100 MB), gated behind a build tag so
  CI stays fast.
- MATLAB SOFA Toolbox cross-validation: round-trip a go-sofa-written
  file through the toolbox and back, diff the result.
- Benchmarks for large-file creation and read paths (`go test -bench`).

### Phase E — Cross-repo follow-ups in go-hdf5 (non-blocking)

- **Dense storage for dataset attributes.** `WithAttribute` (go-hdf5
  v0.15.0) caps at 8 attributes per dataset using compact storage.
  SOFA dimension scales never need more than 3, so this is fine in
  practice; extend the dense-storage path that already exists for root
  attributes if a future use case demands it.
- **`DIMENSION_LIST` attribute on data datasets.** For full netCDF-4
  parity, `Data.IR` / `Data.Real` etc. should carry a
  `DIMENSION_LIST` attribute that is a variable-length array of
  references to the dimension-scale datasets. Requires VLA + object
  reference support in go-hdf5 attribute encoding. Not needed for the
  readers we care about today.

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
