# Design notes

Decisions and findings behind the read and write paths that are not obvious
from the code. Open work is tracked in GitHub issues; the history is in
`git log` and CHANGELOG.md.

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
  `Open` returns; `Close` is a no-op kept for API stability. `OpenLazy` is
  the streaming alternative (below).
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

## Lazy reads (`OpenLazy`)

- **Same parser.** `OpenLazy` shares `open` with `Open`: everything but the
  audio variables is loaded, the audio layouts are resolved and checked
  without reading values, and the HDF5 handle stays open until `Close`
  (idempotent). Later audio reads fail with `fs.ErrClosed`; `IRAt`,
  `IRPeakdB`, `Save` and `WriteTo` fail with `ErrNotLoaded` while the audio
  fields are empty. On an 11.5 MB file `Open` allocates 23.4 MB and
  `OpenLazy` 0.35 MB.
- **One `ReadSlice` per measurement.** Every read selects `[m, 0:…, 0:…]`,
  spanning all trailing axes, so contiguous data (what `Save` writes) is one
  linear run. The per-measurement readers work on eager files too, returning
  the loaded slices. TF-E is returned `[R][E][N]` whichever of the two file
  orders was read.
- **No go-sofa cache.** Chunked, deflated files rely on go-hdf5's
  per-dataset cache (parsed header, chunk index, recently used decompressed
  chunks). A go-sofa cache of whole chunk rows along M would save at most
  ~10 % on top of it and was removed.

## API shape

The parallel per-DataType fields (`ImpulseResponses`, `TFReal`/`TFImag`,
`TFRealE`/`TFImagE`, `SOSCoefficients`) stay; a `Data` interface or tagged
union was rejected. The typed accessors, the `DataType*` constants and the
per-measurement readers give the ergonomics without breaking every caller.

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

## Known gaps

- MultiSpeakerBRIR has no CI fixture: sofar only has version 0.3 with
  `DataType=FIRE`, which go-sofa rejects.
- A Directivity validator needs an example file; today there is only
  `IsDirectivity` and the registry's DataType/layout rule.
- `Save`'s chmod/rename/fsync failure branches are untested; they need a
  filesystem seam.

## go-hdf5

go-sofa depends on the [CWBudde/go-hdf5](https://github.com/CWBudde/go-hdf5)
fork. v0.16.0 made the output readable by libhdf5/netCDF-C (root header past
the EOA), added `DatasetWriter.AttachDimensionScale`, dataset headers over 255
bytes and libhdf5-readable VLEN data; v0.16.1 fixed dense attributes with
12-byte names (`DateModified`, `Organization`). go-sofa uses v0.17.0
(merged [go-hdf5#5](https://github.com/CWBudde/go-hdf5/pull/5), `5753c09`;
`go.mod` pins that commit as a pseudo-version until the tag exists) for
reader/writer entry points (`OpenReader`, `CreateForWriteTo`), dataset
shapes, dimension-scale readers, correct hyperslab reads at any offset, the
per-dataset read cache, dense storage for more than eight dataset
attributes, growing root link storage, dense link/attribute reads with v2
B-trees of any depth, and scalar (NC_CHAR) string attributes.

The fork is pre-1.0 with a single maintainer, so go-sofa pins an exact
version and fuzzes `Open`. No upstream gap is known to affect go-sofa; report
new ones as issues on the fork and note any go-sofa workaround here.

## Performance baseline

`go test -tags largefiles -run '^$' -bench . -benchtime 5x -count 3`
(2026-09-26, go-hdf5 `34395b7`, Go 1.25.0, linux/amd64, 4 × Intel Xeon @
2.10 GHz, shared machine, so expect ±30 % noise). File: 6400 × 2 × 1024
float64 = 104.9 MB of `Data.IR`, contiguous (as `Save` writes it); MB/s is
over that size, median of three runs. The weekly
`.github/workflows/largefiles.yml` runs the same suite once per benchmark.

| Benchmark                        |       ns/op | MB/s |    B/op | allocs/op |
| -------------------------------- | ----------: | ---: | ------: | --------: |
| `BenchmarkReadLarge` (`Open`)    | 112,922,830 |  929 | 210.9 M |    11,794 |
| `BenchmarkWriteLarge` (`Save`)   | 352,037,968 |  298 | 210.3 M |     3,288 |
| `StreamVsEager/eager` (`Open`)   | 115,980,542 |  904 | 210.9 M |    11,794 |
| `StreamVsEager/stream` (all `M`) | 110,991,560 |  945 | 211.6 M |    56,614 |
| `StreamVsEager/stream-one`       |      18,404 |  890 |  33,248 |        15 |

`stream` reads all 6400 measurements through `OpenLazy` +
`RangeMeasurements` in about the time of `Open`, while the live heap stays at
a few MB instead of 210 MB. `stream-one` is one random `ReadMeasurement`
(R × N × 8 = 16 KB; MB/s counts that) on an open lazy file.

`BenchmarkStreamChunked` (default build) streams every measurement of the
chunked, deflate-compressed CI fixtures, `OpenLazy` included:

| Fixture (`Data.IR` shape, chunks)          | ns/op      | B/op   | allocs/op |
| ------------------------------------------ | ---------- | ------ | --------: |
| CIPIC (1250 × 2 × 200, 1250 × 1 × 200)     | 45,913,542 | 21.4 M |    37,807 |
| MIT_KEMAR (710 × 2 × 512, 355 × 1 × 256)   | 47,086,432 | 26.6 M |    35,553 |
| Mesh2HRTF (1850 × 2 × 320, 1850 × 1 × 320) | 68,215,675 | 49.5 M |    56,201 |
