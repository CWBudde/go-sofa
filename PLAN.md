# go-sofa — SOFA (AES69) File Reader/Writer in Go

## Goal

A pure-Go library for reading and writing SOFA files (AES69 — Spatially
Oriented Format for Acoustics), built on top of
[MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5)
(our fork of scigolib/hdf5). The Go equivalent of
`PasSofa/SofaFile.pas`.

## Status

Read, write, CLI tools, CI, lint, ≥80 % test coverage, and both `FIR`
and `TF` `DataType`s are complete. See `git log` for history; this
file tracks only what's still open.

## Prior Art & Key Resources

| Resource | Notes |
| -------- | ----- |
| `../PasSofa/Source/SofaFile.pas` | Our own SOFA reader (~430 LOC Pascal). Behavioural reference. |
| `../go-hdf` (go-hdf5 fork) | Pure-Go HDF5 read+write. Provides the low-level file access. |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based). |
| [libmysofa](https://github.com/hoene/libmysofa) | Lightweight C SOFA reader — useful as behavioural reference. |

---

## Open work

### A. Cross-repo work in go-hdf5

*All previously open items are now landed and tagged. Section kept for
the record; new cross-repo work would be added here.*

Optional follow-ups, none blocking:

- **Dense storage for dataset attributes.** `WithAttribute` (shipped
  in go-hdf5 v0.15.0) currently caps at 8 attributes per dataset
  using compact storage. SOFA dimension scales never need more than
  3, so this is fine in practice; if a future use case demands it,
  extend the dense-storage path that already exists for root
  attributes.
- **`DIMENSION_LIST` attribute on data datasets.** For full netCDF-4
  parity, `Data.IR` / `Data.Real` etc. should carry a
  `DIMENSION_LIST` attribute that is a variable-length array of
  references to the dimension-scale datasets. Requires VLA + object
  reference support in go-hdf5 attribute encoding. Not needed for
  the readers we care about today.

### B. Additional `DataType` values surfaced by real files

Reading SOFA files produced by the upstream MATLAB toolbox now
works (V2 continuation chunks are followed and contiguous datasets
with `HADDR_UNDEF` are tolerated), which exposed two `DataType`
strings beyond `FIR`/`TF`:

1. **`TF-E`** — TF with an active emitter dimension. Same as TF but
   the complex arrays are `[M][R][E][N]` instead of `[M][R][N]`.
   Used by `GeneralTF-E` and the more recent `FreeFieldHRTF` files.

2. **`SOS`** — Second-Order Section filter coefficients
   (`Data.SOS` instead of `Data.IR`/`Data.Real`). Used by
   `SimpleFreeFieldHRSOS`. Storage shape is dictated by the number
   of biquad sections; `Data.SamplingRate` and `Data.Delay` are
   still present.

Both are unsupported today; `Open` rejects them with
`unsupported DataType …`. Add when needed.

### C. TF edge cases (low priority)

1. **TF with `N == 1`.** A frequency vector of length 1 has an
   on-disk shape identical to a scalar `/N` (FIR). `readDimensions`
   currently disambiguates by checking whether `/N` carries a
   netCDF "coordinate variable" `NAME` attribute (single label like
   `"N"`). That heuristic covers MATLAB-produced files; go-sofa-
   written TF files with `N == 1` may still misread until we add a
   sentinel attribute or always write `/N` as a vector when
   `DataType == "TF"`.

### D. Phase 6 — future / optional

1. **SOFA 2.0 receiver representations** — spherical harmonic
   coefficients (AES69-2022). New `File` fields and a third audio
   path alongside FIR/TF.

2. **Additional SOFA conventions.** Today only convention names are
   stored; behaviour is generic. Specialise for BRIR, SRIR, and
   directivity if/when needed.

3. **Performance: partial reads.** Hyperslab reads for IR / TF would
   let consumers stream a subset (e.g. one measurement at a time)
   instead of loading the whole `[M][R][N]` array. Useful for very
   large HRTF databases.

4. **Extended testing (optional).** Larger files (>100 MB), MATLAB
   SOFA Toolbox cross-validation once A.1 lands, benchmarks for
   large-file creation/read.

---

## Risk Register

| Risk | Impact | Mitigation |
| ---- | ------ | ---------- |
| go-hdf5 API changes | Medium | Pin dependency version; coordinate with fork. |
| SOFA files using unsupported `DataType` values | Medium | B above (`TF-E`, `SOS`); add support when a use case appears. |
| SOFA convention evolution (2.0+) | Low | D.1 above. |

---

## References

- [SOFA Specifications](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications)
- [netCDF-4/HDF5 File Format](https://docs.unidata.ucar.edu/netcdf-c/current/file_format_specifications.html)
- [HDF5 Object Header Specification](https://docs.hdfgroup.org/hdf5/develop/_s_p_e_c.html#OHDRLayout)
- [Detailed write-support implementation notes](/home/christian/.claude/plans/sofa-write-implementation.md)
