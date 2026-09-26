# Design notes

Decisions and findings behind the read and write paths that are not obvious
from the code. `PLAN.md` tracks open work only; this file keeps what was learnt
while closing it.

## Write path (`Save`)

- **netCDF-4 layout.** Dimension scales `M R E N C I` (plus extra dimensions
  such as `S` from `File.Variables`) are written at full length with the
  netCDF-C `NAME`, `_Netcdf4Dimid` and a root `_NCProperties`. Every variable
  is created from named dimensions, so go-hdf5 writes `DIMENSION_LIST` /
  `REFERENCE_LIST`; `ncdump -h` must show no `phony_dim` (checked by
  `just interop`).
- **Durability.** `Save` writes a temp file in the target directory, `fsync`s
  it and renames it over the target; the HDF5 close error is returned.
- **Determinism.** Scales are written in a fixed order. Output is
  byte-identical across runs only when `DateCreated`/`DateModified` are set.
- **Defaults live in the file only.** Empty mandatory globals (`Title`,
  `DateCreated`, `DateModified`, `APIName`, `APIVersion`, `AuthorContact`,
  `Organization`, `License`, `RoomType`) and an unset single
  `ListenerView`/`ListenerUp` (`[1 0 0]`/`[0 0 1]`, spherical
  `(0,0,1)`/`(0,90,1)`) are written with SOFA Toolbox defaults; the `File`
  is not changed. Dates are UTC `YYYY-MM-DD HH:MM:SS`.
- **`Data.Delay` is always 2-D**: `[I,R]` or `[M,R]`, zeros when empty, SOS
  included. The M == R ambiguity is resolved with the layout `Open` found.
- **TF-E is written `[M,R,N,E]`**, the order of the SOFA Toolbox convention
  tables (`mrne` for GeneralTF-E and FreeFieldHRTF).
- **Per-measurement layouts**: `ReceiverPositionsM` `[R,C,M]`,
  `EmitterPositionsM` `[E,C,M]`, `ListenerViews`/`ListenerUps` `[M,C]`.
- **Validation runs on `Save` only** (`Open` never validates). Every error is
  a `*ValidationError{Field, Err}` naming the `File` field; convention rules
  come from the registry in `sofa_conventions.go`, and advisory findings go to
  `ConventionWarnings()` instead of failing.

## Read path (`Open`)

- **Eager.** All data is read up front and the HDF5 handle is closed before
  `Open` returns; `Close` is a no-op kept for API stability.
- **Untrusted input.** Dimensions are checked (> 0, finite, no `int` overflow,
  size cap) before any dataset is read. `FuzzOpen` (`fuzz_test.go`) guards
  against panics and OOMs; run it with `GOMEMLIMIT`.
- **Shapes, not element counts.** Each variable is matched against its allowed
  layouts by the dimension names from the scales' `REFERENCE_LIST`, falling
  back to sizes; anything else fails. Both TF-E orders are accepted because
  the SOFA Toolbox (2.2.1) writes `[M,R,N,E]`.
- **DataType.** Empty, unknown, `FIR-E` (GeneralFIR-E) and legacy `FIRE` are
  rejected with `ErrUnsupportedDataType`; a non-SOFA `Conventions` gives
  `ErrNotSOFA`.
- **No silent loss.** Unreadable mapped attributes and positions fail `Open`.
  Unmapped globals, variables and variable attributes land in `Attributes`,
  `Variables` and `VariableAttributes` and are written back; what cannot be
  kept (scalars, wide strings, compounds) is listed in `Dropped`. `Type` and
  `Units` keep their case; comparisons use `strings.EqualFold`.
- **Null-dataspace globals** (the Toolbox's empty `Title`) read as `""`.

## Spherical harmonics

Real SH files use `DataType=TF-E` with `E = (Lmax+1)²` SH coefficients; no
file in the public corpus uses `DataType=SH`. AES69 marks SH by
`EmitterPosition:Type = "spherical harmonics"`, which decides detection. Only
when it is empty do a convention name containing "SH" or a `History` like
"Converted to Spherical Harmonics" count; `SHWarnings` reports a heuristic
that contradicts a set Type.

## Fixtures

- `sofa20_sh_test.sofa` (local only) is misnamed: `SimpleFreeFieldHRTF`,
  `DataType=TF`, no SH content.
- `MIT_KEMAR_normal_pinna.sofa` is an HRIR; the BRIR round-trip uses
  `OfficeII.sofa`. `SingleRoomSRIR_1.1.sofa` is a demo with R=1 (order 0).
- `SimpleFreeFieldSOS_1.0.sofa` (local only, `RoomVolume` as a root
  attribute) could not be opened while go-hdf5 read only depth-0 v2 B-trees;
  go-hdf5#5 reads any depth, so it should open now (not re-tested, the file
  is not available). `testdata/sofar/SingleRoomSRIR_1.0.sofa` covers a
  depth-1 attribute B-tree (34 root attributes). The attribute fallback for
  `RoomVolume`/`RoomTemperature` is unit-tested only.
- Survey an unknown file with `go run ./cmd/sofaprobe <file>` or
  `h5dump -A -H <file>`.

## go-hdf5

go-sofa depends on the [CWBudde/go-hdf5](https://github.com/CWBudde/go-hdf5)
fork. v0.16.0 made the output readable by libhdf5/netCDF-C (root header past
the EOA), added `DatasetWriter.AttachDimensionScale`, dataset headers over 255
bytes and libhdf5-readable VLEN data; v0.16.1 fixed dense attributes with
12-byte names (`DateModified`, `Organization`). go-sofa uses v0.17.0
(merged [go-hdf5#5](https://github.com/CWBudde/go-hdf5/pull/5), `5753c09`)
for reader/writer entry points (`OpenReader`, `CreateForWriteTo`), dataset
shapes, dimension-scale readers, correct hyperslab reads, growing root link
storage, dense link/attribute reads with v2 B-trees of any depth, and
scalar (NC_CHAR) string attributes. Remaining
upstream gaps are tracked as Phase E in `PLAN.md`.
