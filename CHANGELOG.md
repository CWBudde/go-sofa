# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- **Breaking:** `IRAt`, `IRPeakdB` and `Duration` return an error:
  `ErrUnsupportedDataType` on non-FIR files (where `Duration` used to divide
  a frequency-bin or coefficient count by the sampling rate) and
  `ErrIndexOutOfRange` for indices outside the file's dimensions.
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

### Added

- `ReceiverPositionsM`, `EmitterPositionsM` (`[R,C,M]` / `[E,C,M]` read as
  `[M][R]` / `[M][E]`) and `ListenerViews`, `ListenerUps` (`[M,C]`), filled by
  `Open` for measurement-dependent layouts. `Save` does not write them yet.

### Fixed

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
