# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Breaking:** the module path is `github.com/CWBudde/go-sofa`, matching
  the repository's name on GitHub. Go module paths are case-sensitive, so
  imports and `go get`/`go install` lines using `github.com/cwbudde/go-sofa`
  must be updated.
- `Open` reads everything and closes the file before it returns, so a `File`
  holds no open handle; `Close` does nothing and returns nil, except on a
  `File` from `OpenLazy`, whose file it closes.
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
- Requires go-hdf5 v0.16.1: with v0.16.0, libhdf5 and netCDF-C could not open
  the `DateModified` and `Organization` attributes.
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

- `OpenLazy` reads a file's metadata, positions, sampling rate and delay but
  leaves the audio data in the file, which it keeps open until `Close`.
- `(*File).ReadMeasurement(m)` returns the FIR impulse responses of one
  measurement as `[R][N]`, read from the file for a `File` from `OpenLazy`
  and copied from `ImpulseResponses` otherwise.
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

### Fixed

- Global attributes the SOFA Toolbox stores as empty (null dataspace), such
  as an empty `Title` or `Comment`, were read as `"[]"` and written back that
  way; they now read as `""`.

- TF-E files written by the SOFA Toolbox store `Data.Real`/`Data.Imag` as
  `[M,R,N,E]`; `Open` read them as `[M,R,E,N]`, scrambling every value. They
  are now transposed into `TFRealE`/`TFImagE`'s `[M][R][E][N]`.

## [v0.1.0]

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
