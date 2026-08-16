# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and
this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
