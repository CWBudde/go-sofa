# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- `Save` and `WriteTo` deflate the audio data at level 4
  (`DefaultDeflateLevel`) by default, so re-saved files are about the size
  of their netCDF-C originals instead of up to 57× larger. Read with go-hdf5
  v0.18.1 or later, HDF5/netCDF-C and libmysofa. `WithDeflate(0)` writes
  the uncompressed, contiguous layout of earlier versions. Deflated data
  never takes more than 64 chunks, the most libmysofa can index: big files
  get chunks above 4 MiB, and files with more than 64 receivers store
  several receivers per chunk.

- **Breaking:** `Save` is `Save(path string, opts ...SaveOption) error`
  (see `WithDeflate` below). Calls compile unchanged, but an interface
  declaring `Save(string) error` no longer matches `*File`, and the method
  value `f.Save` has a new type.

- `Open`, `OpenReader`, `OpenLazy` and `OpenLazyReader` refuse, before
  reading any data, a file whose variables together declare more than
  64 Mi elements or eight elements per byte of the file, whichever is more,
  or one of which declares more than 2^30 elements, with an error wrapping
  the new `ErrTooLarge`. A crafted file of a few
  kilobytes with chunked, never-written variables used to open and pin
  gigabytes. The lazy functions do not count the audio variables they leave
  in the file. Every real file tested stays at least 52× below the budget.
- **Breaking:** `IsBRIR` reports true only for `SingleRoomDRIR` and
  `IsSRIR` only for `SingleRoomSRIR`; neither matches `MultiSpeakerBRIR` or
  `SingleRoomMIMOSRIR` any more, and the BRIR/SRIR rules for those two
  conventions are dropped. They use DataType `FIRE` and `FIR-E`, which `Open` and
  `Save` reject, so the rules could never apply. README "Limitations" lists
  FIR-E as unsupported.
- **Breaking:** `Save` requires the one DataType each official convention's
  SOFA Toolbox table allows for GeneralFIR (FIR), GeneralTF (TF),
  GeneralTF-E (TF-E), SimpleHeadphoneIR (FIR), SingleRoomSRIR (FIR) and
  SingleRoomDRIR (FIR), and returns a `*ValidationError` for field
  `DataType` otherwise; these used to accept any DataType. The legacy SOFA
  1.0 name SimpleFreeFieldSOS is validated like SimpleFreeFieldHRSOS
  (DataType SOS, E = 1) and gets its mandatory global attributes.
- `Save` no longer rejects a SimpleFreeFieldHRIR, SimpleFreeFieldHRTF or
  SimpleFreeFieldHRSOS file whose `R` is not 2: neither the SOFA Toolbox
  tables nor sofar fix the receiver count (the SOFA wiki allows any), and
  real databases such as Kayser2009 (R = 8) could not be re-saved.
  `ConventionWarnings` reports it instead, since libmysofa rejects such
  SimpleFreeFieldHRIR files.
- `Save` writes an empty `RoomType` as the convention's default instead of
  `free field` for every convention: `reverberant` for SingleRoomDRIR, and
  `shoebox` for SingleRoomSRIR when `Variables` holds both `RoomCornerA`
  and `RoomCornerB` (sofar requires the corners for a shoebox room; without
  them SingleRoomSRIR keeps `free field`). An explicit `RoomType` is written
  as is, and the `File` is not modified.
- `Save` no longer rejects a SingleRoomDRIR file with an empty `RoomType`;
  it writes the `reverberant` default instead.
- **Breaking:** `Save` and `WriteTo` record their own provenance, as the
  SOFA Toolbox's `SOFAsave` does: the written `APIName` is always
  `go-sofa`, `APIVersion` go-sofa's module version and `DateModified` the
  save time; they used to keep the `File`'s values, so a re-saved file
  still claimed the API that first wrote it (libmysofa keys a workaround on
  those two). `DateCreated` is kept (stamped only when empty). When the
  `File` names another API or go-sofa version, `History` gains a line
  `resaved by go-sofa <version> from <APIName> <APIVersion>`. The `File`
  is not modified. Output is byte-identical across saves only with
  `SOURCE_DATE_EPOCH` set (see Added).
- `Save` no longer writes `Type` and `Units` on `ListenerUp`: the SOFA
  convention tables define them on `ListenerView` only, and sofar reports
  them as custom entries. `Open` still accepts files that have them (go-sofa
  v0.2.0 wrote them) and drops them, so a re-save cleans the file.

### Added

- `Save` takes options. `WithDeflate(level)` sets the deflate level of the
  audio data (`Data.IR`, `Data.Real`/`Data.Imag`, `Data.SOS`), stored
  shuffled in chunks of one receiver across all measurements (at most
  4 MiB, see Changed for the 64-chunk limit), as netCDF-C files usually
  are; 0 stores it uncompressed. Level 4 re-saves MIT KEMAR in 1.10 MB
  instead of 5.85 MB (original: 1.17 MB) and `tester.sofa` in 73 KB
  instead of 5.2 MB. libmysofa loads the deflated files with go-hdf5
  v0.18.1, whose chunk index it can read (CWBudde/go-hdf5#13).
  `just interop` checks every generated and re-saved file both deflated
  and uncompressed.
- `Save` and `WriteTo` honour the `SOURCE_DATE_EPOCH` environment variable
  (seconds since the Unix epoch, the reproducible-builds convention): it
  replaces the current time as the save time, so output is reproducible. A
  malformed value, the empty string included, is an error.
- `ErrTooLarge`, returned by the Open functions for a file declaring more
  data than its size makes plausible.
- `Save` writes the `SourceView` and `SourceUp` variables that
  SingleRoomSRIR, SingleRoomDRIR and FreeFieldDirectivityTF make mandatory
  when `Variables` lacks them, with the table's default: `[I,C]`,
  `SourceView` `[1 0 0]` (SingleRoomDRIR: `[-1 0 0]`) cartesian in metres,
  and `SourceUp` `[0 0 1]` without `Type`/`Units` (the tables define none on
  it), plus the attributes `VariableAttributes` holds for
  that name. A `SourceView` or `SourceUp` in `Variables` is written as is. The `File` is not modified.

- `Save` writes the global attributes a convention's SOFA Toolbox table
  makes mandatory beyond the generic ones, as the empty default the table
  gives them, when `Attributes` lacks them: `DatabaseName` and
  `ListenerShortName` (SimpleFreeFieldHRIR/HRTF/HRSOS, FreeFieldHRTF),
  `DatabaseName`, `SourceType` and `SourceManufacturer`
  (FreeFieldDirectivityTF), `DatabaseName`, `ListenerShortName`,
  `ReceiverDescription` and `EmitterDescription` (SimpleHeadphoneIR),
  `DatabaseName` (SingleRoomSRIR), and `RoomDescription` and `DatabaseName`
  (SingleRoomDRIR). The `File` is not modified.

- `ConventionWarnings` reports a `SOFAConventionsVersion` that is not a
  known version of the file's official convention (current and deprecated
  SOFA Toolbox and pyfar tables; the legacy SimpleFreeFieldSOS is checked
  against its own 1.0; custom conventions are not checked), and SOFA 2.x
  features in a file declaring `Version` below 2.0: the FreeFieldHRTF
  convention, DataType TF-E and a position `Type` of `spherical harmonics`.
  These are warnings only; `Save` still writes such files.

- `ConventionWarnings` reports a `RoomType` other than `free field` in a
  SimpleFreeFieldHRIR/HRTF/HRSOS or FreeFieldHRTF file (sofar rejects
  those; the SOFA Toolbox tables do not restrict the value). A warning
  only; `Save` still writes such files.

- `Save` writes the `Reference` attribute FreeFieldDirectivityTF's table
  makes mandatory on `SourcePosition`, `SourceView` and `SourceUp`, as `""`
  where neither `VariableAttributes` nor the variable's own `Attributes`
  set one. The `File` is not modified.

- libmysofa regression check: `just interop` (and the `test-interop` CI
  workflow) builds libmysofa's loader at a pinned commit (`just libmysofa`)
  and loads every generated file and every re-saved reference file with it.
  SimpleFreeFieldHRIR files must pass `mysofa_check`; re-saved files must
  get the same result as their originals.

### Fixed

- `ReadMeasurement`, `ReadMeasurementTF`, `ReadMeasurementTFE`,
  `ReadMeasurementSOS` and the `RangeMeasurements` functions no longer
  panic on a `File` from `OpenLazy` whose `DataType`, `M`, `R`, `N` or `E`
  was changed afterwards: they return `ErrUnsupportedDataType` when the file
  holds no audio variable for the new DataType and `ErrIndexOutOfRange`
  when the stored shape no longer matches, instead of dereferencing nil or
  reshaping the stored data to the new sizes. The `RangeMeasurements`
  functions check the shape before the first callback, so an `M` changed to
  0 or less fails instead of returning nil, and after `Close` a shape
  mismatch no longer masks `fs.ErrClosed`.
- `Save` writes `ReceiverPosition` and `EmitterPosition` as `[R,C,I]` and
  `[E,C,I]` (`[I,C,I]` for a single shared row), the only layouts the SOFA
  conventions allow; they were `[R,C]` and `[E,C]`, which libmysofa's
  `mysofa_check` rejects. `Open` still reads the old layout. Together with
  go-hdf5 v0.18.0, SimpleFreeFieldHRIR files written by go-sofa now pass
  libmysofa's `mysofa_load` and `mysofa_check`.
- `OpenLazy` sizes go-hdf5's cache of decompressed chunks to the chunks one
  measurement spans (up to 256 MiB per audio variable) when chunks span
  several measurements, instead of leaving it at 8 chunks / 16 MiB. With a file whose measurements span more chunks
  than that, reading every measurement decompressed every chunk again for
  each measurement: Kayser2009 (`Data.IR` [584,8,4800] in chunks of
  [146,1,1200], 32 per measurement) streamed in 78 s and allocated 75.7 GB;
  now 0.6 s and 885 MB, as much as `Open`. Files with one chunk per
  receiver, the common layout, read as before. The `OpenLazy` godoc
  explains when lazy reading saves memory.
- `Open` and `OpenLazy` read a TF-E file without dimension labels (as h5py
  writes it) whose E equals N in the AES69 order `[M,R,N,E]`; they used to
  read it as `[M,R,E,N]`, transposing every emitter/frequency block.
  Consequence: a TF-E file saved by go-sofa v0.1.0 (`[M,R,E,N]`, no labels)
  with E == N now reads transposed; nothing in such a file tells the two
  orders apart. Labelled files, including every file saved by v0.2.0 and
  later, and files with E ≠ N are unaffected.

### Changed

- Requires go-hdf5 v0.18.0, which writes new-style groups (the layout
  libmysofa reads), text attributes with the ASCII character set, and
  tracks attribute creation order as netCDF-C does.
- **Breaking:** `Save` requires `ReceiverPositions` and `SourcePositions`,
  which the SOFA conventions make mandatory without a usable default, and
  writes an empty `ListenerPositions` or `EmitterPositions` as the
  conventions' default, `[0 0 0]` cartesian in metres. The `File` is not
  modified.
- `Save` always writes the mandatory `Units` attribute of the four position
  variables; empty `…PositionUnits` are written as the default for the
  `Type`, `metre` for cartesian and `degree, degree, metre` for spherical
  and spherical harmonics. An empty `Units` used to be left out.
- **Breaking:** `UnitsCartesianMetres` is `"metre"`, the value of the SOFA
  convention tables, instead of `"metre, metre, metre"`. `Open` still reads
  either as it is stored.
- **Breaking:** `Save` checks the `Units` of every position and of
  `ListenerView`: each comma-separated part must be `metre` or `degree`
  (also `meter`, `metres`, `meters`, `degrees`; any case), the names sofar
  and the SOFA Toolbox accept. Other units, radians included, are a
  `*ValidationError`; the default spherical `ListenerUp` for radian units
  (elevation π/2) is gone. `Open` still reads such files.

## [v0.2.0] - 2026-09-26

### Changed

- Requires go-hdf5 v0.17.0 (merged
  [CWBudde/go-hdf5#5](https://github.com/CWBudde/go-hdf5/pull/5),
  `5753c09`). Dataset shapes and dimension-scale `REFERENCE_LIST`s now
  come from its public API instead of parsing `Dataset.Info()` text and
  attribute bytes; files read the same.
- Streaming reads rely on go-hdf5's per-dataset cache (parsed header, chunk
  index, recently used decompressed chunks) instead of go-sofa's own
  chunk-row cache, which is gone. Streaming every measurement of a lazy
  file now takes about as long as `Open` (105 MB contiguous file: 1.0×,
  was 2.4×), and of the chunked CI fixtures 33–42 % less time than before.
- `sofaprobe` reads only the three previewed values at each end of the
  audio data instead of whole rows, and shows the decoded `DIMENSION_LIST`
  and `REFERENCE_LIST` attributes instead of `(unreadable: …)`.
- **Breaking:** the module path is `github.com/CWBudde/go-sofa`, matching
  the repository's name on GitHub. Go module paths are case-sensitive, so
  imports and `go get`/`go install` lines using `github.com/cwbudde/go-sofa`
  must be updated.
- `Open` reads everything and closes the file before it returns, so a `File`
  holds no open handle; `Close` does nothing and returns nil.
- Validation errors from `Save` name the `File` field first, capitalised as
  the field is (`M: must be > 0, got 0`, `SamplingRate: length 3 must be M=2
or 1`, `ImpulseResponses[0] length 1 does not match R=2`).
- **Breaking:** `Open` keeps the case of the `Type` and `Units` attributes
  (`"Spherical"`, `"meter"`) instead of lowercasing them, so a round trip no
  longer rewrites them; values are still trimmed. Compare them with
  `strings.EqualFold`, as go-sofa does internally.
- **Breaking:** `Save` validates values: it rejects NaN and ±Inf anywhere it
  writes, sampling rates ≤ 0, negative or non-ascending `Frequencies`, and
  zero rows in `ListenerViews`/`ListenerUps` (radius 0 when spherical).
  `SimpleFreeFieldHRIR`/`HRTF`/`HRSOS` files must have `DataType` FIR/TF/SOS,
  `R = 2` and `E = 1`; `FreeFieldHRTF` must be TF-E and
  `FreeFieldDirectivityTF` TF. An unset `ListenerView`/`ListenerUp` is written
  as the conventions' default (`[1 0 0]`/`[0 0 1]`) instead of zeros; the
  `File` is not modified.
- **Breaking:** `Save` rejects a file without `SOFAConventionsVersion`, and
  every written position (listener, receiver, source, emitter) needs a
  `Type` of `cartesian`, `spherical` or `spherical harmonics`.
- **Breaking (file layout):** TF-E `Data.Real`/`Data.Imag` are written as
  `[M,R,N,E]`, the order of the AES69 convention tables and the SOFA
  Toolbox (was `[M,R,E,N]`). `Open` reads both.
- **Breaking (file layout):** `Data.Delay` is always written 2-D, `[I,R]` or
  `[M,R]`: `[I]`/`[R]` delays are expanded to `[I,R]`, `[M]` to `[M,R]`, and
  an empty Delay is written as zeros `[I,R]` (SOS files included).
- `Save` always writes the mandatory global attributes `Title`,
  `DateCreated`, `DateModified`, `APIName`, `APIVersion`, `AuthorContact`,
  `Organization`, `License` and `RoomType`. Empty ones get defaults in the
  file only: `go-sofa`, the module version, the current UTC time as
  `YYYY-MM-DD HH:MM:SS`, and the SOFA Toolbox's License and RoomType
  (`free field`) defaults. The `File` is not modified.
- `Save` writes `Data.SamplingRate:Units = hertz`, `LongName = frequency` and
  `Units = hertz` on the TF/TF-E `N` variable, and `Type`/`Units` on
  `ListenerView` and `ListenerUp`.
- **Breaking:** `IRAt`, `IRPeakdB` and `Duration` return an error:
  `ErrUnsupportedDataType` on non-FIR files (where `Duration` used to divide
  a frequency-bin or coefficient count by the sampling rate) and
  `ErrIndexOutOfRange` for indices outside the file's dimensions.
- **Breaking:** `SamplingRateScalar` returns `(float64, error)`:
  `ErrNoSamplingRate` when no rate is stored and `ErrVaryingSamplingRate` when
  the per-measurement rates differ, instead of silently returning the first.
- `Open` rejects an empty, unknown, `FIR-E` or `FIRE` `DataType` with
  `ErrUnsupportedDataType` instead of reading it as FIR.
- `Open` checks every variable's shape, not only its element count, using the
  file's netCDF dimension names where present; an axis permutation or any
  other layout AES69 does not allow is an error instead of a silent misread.
- `Open` fails when a global attribute go-sofa maps to a field, a position or
  orientation dataset, or its `Type`/`Units` attribute is present but cannot
  be read, instead of leaving the field empty. Global attributes go-sofa does
  not interpret are no longer decoded.
- SH detection (`SHOrder`, `IsSHEncoded`, `SHCoefficientCount`) follows
  AES69: `EmitterPositionType == "spherical harmonics"` decides when set, and
  the convention-name / History heuristics apply only when it is empty.
  `DataType` must be `TF-E`, and `E = 1` is order 0. A heuristic that
  contradicts a set Type is reported by `SHWarnings`.
- **Breaking (CLI):** `sofa2json` writes the field names of `sofa.File` as
  JSON keys (`M`, `R`, `E`, `N`, `SamplingRate`, `ImpulseResponses`, … instead
  of `Measurements`, `Receivers`, `Emitters`, `DataSamples`, `SampleRate`,
  `IR`), adds `Conventions`, `Version`, `SOFAConventions`,
  `SOFAConventionsVersion`, all positions with their `Type`/`Units`,
  `ListenerView`/`ListenerUp`, and writes NaN/±Inf as `null`. It streams the
  JSON instead of building it in memory and refuses to overwrite an existing
  `.json` unless `-f` is given.
- **Breaking (CLI):** `sofa2json`, `sofainfo` and `sofaprobe` parse flags
  with the `flag` package (`-h` prints usage; unknown flags exit 2), process
  every file argument instead of only the first, report progress and errors
  on stderr and exit 1 if any file failed.
- `sofainfo` prints `Conventions`, `Version`, `SOFAConventions`,
  `SOFAConventionsVersion` and a full delay summary (count, dimensions as
  stored, range, values); `sofaprobe` previews `Data.Real`, `Data.Imag` and `Data.SOS` as
  well as `Data.IR`, reading two rows instead of the whole dataset.

### Added

- `OpenReader(r io.ReaderAt, size int64)` and
  `OpenLazyReader(r io.ReaderAt, size int64)` read a SOFA file from memory
  (e.g. a `bytes.Reader`) or any other `io.ReaderAt`, like `Open` and
  `OpenLazy`.
- `(*File).WriteTo(w io.Writer)` writes the file `Save` would write to any
  writer and returns the byte count (`io.WriterTo`). The file is assembled
  in memory and handed to `w` only when complete, so a validation or
  encoding error writes nothing.

- `DataTypeFIR`, `DataTypeTF`, `DataTypeTFE` and `DataTypeSOS` constants.
- `(*File).DelayDimensions()` returns the netCDF dimensions of `Delay` as
  `Open` read them (`[I,R]`, `[M]`, …), or the layout its length implies for
  a `File` built in memory.
- `ErrNotSOFA`, wrapped by `Open` when `Conventions` is not `SOFA`, and
  `*ValidationError{Field, Err}`, which `Save` returns for every validation
  failure; use `errors.Is` / `errors.As`. An unknown `DataType` is both a
  `ValidationError` and `ErrUnsupportedDataType`.
- `SamplingRateAt(m)`, `SourcePositionAt(m)` and `DelayAt(m, r)` resolve
  `[I]`- versus `[M]`-sized variables (and every `Data.Delay` layout, using
  the dimension names `Open` found, so `[M]` and `[R]` are told apart when
  M == R).
- `ReceiverPositionsM`, `EmitterPositionsM` (`[R,C,M]` / `[E,C,M]` read as
  `[M][R]` / `[M][E]`) and `ListenerViews`, `ListenerUps` (`[M,C]`), filled by
  `Open` for measurement-dependent layouts and written back by `Save`.
- `ListenerViewType` and `ListenerViewUnits` hold the coordinate system of
  `ListenerView`/`ListenerUp`; `Open` reads them and `Save` writes them
  (default `cartesian`/`metre`), so spherical orientations are no longer
  relabelled cartesian.
- Lossless round trip: `Attributes` (global attributes without a field, such
  as `DatabaseName`), `Variables` (variables go-sofa does not interpret, such
  as `SourceView`, `RoomCornerA` or the char array `ReceiverDescriptions`)
  and `VariableAttributes` (further attributes of the variables `Save`
  writes) are filled by `Open` and written back by `Save`, which validates
  them against the names, dimensions and attributes it writes itself.
  Numeric variables are kept as float64 and char arrays as bytes, with their
  netCDF dimensions (including new ones such as `S`); whatever `Open` cannot
  keep is listed in `Dropped`.
- Test fixtures written by sofar (pyfar) through netCDF-C, committed in
  `testdata/sofar/` (MIT, generated by `scripts/make_sofar_fixtures.py`), so
  CI tests GeneralTF 2.0, GeneralTF-E, FreeFieldHRTF (plain and
  spherical-harmonics), SimpleFreeFieldHRSOS, SingleRoomSRIR and
  SingleRoomDRIR files from an independent writer. `just interop` also
  re-saves them with go-sofa and compares the result in h5py and netCDF4.
- SOFA Toolbox cross-validation (MATLAB / GNU Octave):
  `scripts/matlab/roundtrip.m` and `internal/interop/toolbox`, documented in
  the README's "Cross-validation" section.
- Streaming reads: `OpenLazy(path)` reads everything but the audio arrays
  and keeps the file open until `Close` (now meaningful for such a `File`;
  it is idempotent, and later audio reads fail with `fs.ErrClosed`).
  `ReadMeasurement(m)` (FIR, `[R][N]`), `ReadMeasurementTF`,
  `ReadMeasurementTFE` (`[R][E][N]`) and `ReadMeasurementSOS` read one
  measurement as a single HDF5 hyperslab, and `RangeMeasurements` /
  `RangeMeasurementsTF` iterate over all of them, stopping at the first
  error. They also work on files read with `Open`. `IRAt`, `IRPeakdB` and
  `Save` fail with the new `ErrNotLoaded` on a lazy `File` whose audio
  fields are empty. `OpenLazy` checks the datatype of every audio dataset,
  so it rejects the files `Open` rejects.
- Large-file suite behind the `largefiles` build tag (a synthetic 105 MB FIR
  file; `BenchmarkStreamVsEager`, `BenchmarkWriteLarge`,
  `BenchmarkReadLarge`), run weekly by `.github/workflows/largefiles.yml`.

### Fixed

- Files with many root variables, such as a SingleRoomSRIR with its
  descriptive variables, can be saved (go-hdf5's fixed 256-byte root link
  heap was full).
- `Open` no longer silently misses a root link of files netCDF-C wrote with
  dense link storage (`ReceiverUp` in the sofar SingleRoomSRIR fixture), and
  reads root attributes kept in a v2 B-tree of depth 1 or more (such as the
  34 root attributes sofar writes for SingleRoomSRIR). Errors in dense link or attribute storage are returned instead of
  skipped.
- `Save` writes variables with more than eight attributes (through
  `VariableAttributes` or `Variables`); it failed with "WithAttribute
  supports at most 8 attributes per dataset". go-hdf5 keeps them in dense
  storage, which h5py and netCDF-C read.
- String attributes are written as scalar fixed-length strings, which
  netCDF-C reads as text (NC_CHAR) instead of NC_STRING, so the SOFA Toolbox
  under Octave loads go-sofa files without conversion.

- Global attributes the SOFA Toolbox stores as empty (null dataspace), such
  as an empty `Title` or `Comment`, were read as `"[]"` and written back that
  way; they now read as `""`.

- TF-E files written by the SOFA Toolbox store `Data.Real`/`Data.Imag` as
  `[M,R,N,E]`; `Open` read them as `[M,R,E,N]`, scrambling every value. They
  are now transposed into `TFRealE`/`TFImagE`'s `[M][R][E][N]`.

## [v0.1.0]

> **Note:** `v0.1.0` was tagged at 24eebca, before the Phase R review fixes
> (unreadable HDF5 output, crashes on crafted input, non-atomic `Save`). It
> stays tagged for reproducibility; use `v0.2.0`, the first release with
> those fixes.

First tagged release. Everything below was already on `main`; this entry records
what that amounts to for a consumer.

### Added

- Reading and writing of SOFA (AES69) files over a pure-Go HDF5 backend
  (`github.com/cwbudde/go-hdf5`) — no cgo, so consumers stay cross-compilable.
- `Open`, `(*File).Save`, and `(*File).Close`, with AES69 validation on save.
- `DataType` support for `FIR` (time-domain impulse responses), `TF` (complex
  transfer functions), `TF-E` (transfer functions with an active emitter
  dimension), and `SOS` (second-order-section coefficients).
- Spherical-harmonic HRTF support: `IsSHEncoded`, `SHOrder`,
  `SHCoefficientCount`, and `SHWarnings`.
- Coordinate systems for every position dataset:
  `SourcePositionType`/`SourcePositionUnits` and the matching
  `ListenerPosition…`, `ReceiverPosition…`, and `EmitterPosition…` pairs, read
  from each dataset's `Type` and `Units` attributes and written back by `Save`.
  Values are lowercased and trimmed; an absent attribute yields `""`, so callers
  can tell "the file does not say" from "the file says cartesian". The
  `CoordinateSpherical`, `CoordinateCartesian`, `UnitsSphericalDegrees`, and
  `UnitsCartesianMetres` constants name the common values.

  This matters because `SimpleFreeFieldHRIR` stores source positions as
  spherical `(azimuth, elevation, radius)` rather than `(X, Y, Z)`. Reading
  those as cartesian silently places every measurement in the wrong direction,
  and before this release the file gave no way to tell the two apart.

- `sofainfo`, `sofa2json`, and `sofaprobe` command-line tools.
