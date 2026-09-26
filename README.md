# go-sofa

A pure-Go library for reading and writing SOFA files (Spatially Oriented Format for Acoustics, AES69-2015).

SOFA is a file format for storing spatially oriented acoustic data like head-related transfer functions (HRTFs), binaural room impulse responses (BRIRs), and directional room impulse responses (DRIRs). The format is based on HDF5 and follows the netCDF-4 conventions.

## Features

- **Pure Go implementation** — No C dependencies
- **Full AES69 support** — Reads and writes all standard SOFA metadata and data arrays
- **DataTypes** — `FIR`, `TF`, `TF-E` (including spherical-harmonics HRTFs) and `SOS`
- **Interoperable output** — Written files are netCDF-4 with named dimensions and open in h5py, netCDF4 and `ncdump`
- **Built on go-hdf5** — Leverages [cwbudde/go-hdf5](https://github.com/cwbudde/go-hdf5) for HDF5 file access
- **Command-line tools** — Includes `sofainfo`, `sofa2json` and `sofaprobe` utilities
- **Well-tested** — Validated against reference SOFA files from sofaconventions.org

## Installation

### Library

```bash
go get github.com/cwbudde/go-sofa
```

### Command-line tools

```bash
go install github.com/cwbudde/go-sofa/cmd/sofainfo@latest
go install github.com/cwbudde/go-sofa/cmd/sofa2json@latest
go install github.com/cwbudde/go-sofa/cmd/sofaprobe@latest
```

## Library Usage

### Basic example

```go
package main

import (
    "fmt"
    "log"

    "github.com/cwbudde/go-sofa"
)

func main() {
    // Open a SOFA file
    f, err := sofa.Open("example.sofa")
    if err != nil {
        log.Fatal(err)
    }
    defer f.Close()

    // Print basic information
    fmt.Printf("Title: %s\n", f.Title)
    fmt.Printf("Measurements: %d\n", f.M)
    fmt.Printf("Receivers: %d\n", f.R)
    fmt.Printf("Samples: %d\n", f.N)
    if sr, err := f.SamplingRateScalar(); err == nil { // not for TF, or varying rates
        fmt.Printf("Sample Rate: %.0f Hz\n", sr)
    }
    if d, err := f.Duration(); err == nil { // FIR only
        fmt.Printf("Duration: %.3f seconds\n", d)
    }
}
```

### Accessing impulse responses

```go
// Get impulse response for measurement 0, receiver 0 (left ear)
ir, err := f.IRAt(0, 0) // ErrUnsupportedDataType for non-FIR files
if err != nil {
    log.Fatal(err)
}
peak, _ := f.IRPeakdB(0, 0)
fmt.Printf("IR samples: %d, peak level: %.1f dB\n", len(ir), peak)

// Access all impulse responses
for m := 0; m < f.M; m++ {
    for r := 0; r < f.R; r++ {
        ir := f.ImpulseResponses[m][r]
        // Process IR data...
    }
}
```

### Reading spatial data

```go
// Listener position for first measurement
if len(f.ListenerPositions) > 0 {
    pos := f.ListenerPositions[0]
    fmt.Printf("Listener at (%.2f, %.2f, %.2f) meters\n", pos.X, pos.Y, pos.Z)
}

// Receiver positions (e.g., left and right ear)
for i, recv := range f.ReceiverPositions {
    fmt.Printf("Receiver %d: (%.3f, %.3f, %.3f)\n", i, recv.X, recv.Y, recv.Z)
}

// Source positions for each measurement
for i, src := range f.SourcePositions {
    fmt.Printf("Source %d: (%.2f, %.2f, %.2f)\n", i, src.X, src.Y, src.Z)
}
```

Position components mean different things in different files, so check the
coordinate system before interpreting them. `SimpleFreeFieldHRIR` stores source
positions as spherical `(azimuth, elevation, radius)`, not as `(X, Y, Z)`:

```go
switch strings.ToLower(f.SourcePositionType) {
case sofa.CoordinateSpherical:
    // src.X is azimuth, src.Y elevation, src.Z radius.
    // f.SourcePositionUnits names the angular units, e.g. "degree, degree, metre".
case sofa.CoordinateCartesian:
    // src.X, src.Y, src.Z are metres.
case "":
    // The file omits the Type attribute; fall back to the convention's default.
}
```

The same `…PositionType` and `…PositionUnits` pair exists for the listener,
receiver, and emitter datasets. Values are trimmed on read but keep the file's
spelling (`"Spherical"`, `"meter"`), so compare them case-insensitively; `Save`
writes them back unchanged.

### Accessing metadata

```go
// AES69 global attributes
fmt.Printf("SOFA Version: %s\n", f.Version)
fmt.Printf("Convention: %s %s\n", f.SOFAConventions, f.SOFAConventionsVersion)
fmt.Printf("Data Type: %s\n", f.DataType)
fmt.Printf("Room Type: %s\n", f.RoomType)
fmt.Printf("Author: %s\n", f.AuthorContact)
fmt.Printf("Organization: %s\n", f.Organization)
fmt.Printf("License: %s\n", f.License)
fmt.Printf("Date Created: %s\n", f.DateCreated)
```

### Transfer-function (TF) files

go-sofa reads and writes both FIR (impulse response, time domain) and
TF (transfer function, frequency domain) SOFA files. TF files store a
frequency vector and complex transfer functions instead of impulse
responses:

```go
f, err := sofa.Open("hrtf.sofa")
if err != nil {
    log.Fatal(err)
}
defer f.Close()

if f.DataType == sofa.DataTypeTF {
    fmt.Printf("Frequencies: %d points (%.1f Hz – %.1f Hz)\n",
        len(f.Frequencies),
        f.Frequencies[0],
        f.Frequencies[len(f.Frequencies)-1])

    // Complex TF for measurement 0, receiver 0
    re := f.TFReal[0][0] // []float64, length N
    im := f.TFImag[0][0] // []float64, length N
    _ = re
    _ = im
}
```

### Writing SOFA files

`File.Save(path)` writes a `*File` back out as a netCDF-4/HDF5-based
SOFA file. All required AES69 fields and array shapes are validated
before any bytes are written, so a failed `Save` leaves the target
path untouched.

#### Creating a file from scratch

```go
package main

import (
    "log"

    "github.com/cwbudde/go-sofa"
)

func main() {
    const M, R, E, N = 1, 2, 1, 64
    f := &sofa.File{
        Conventions:            "SOFA",
        Version:                "1.0",
        SOFAConventions:        "SimpleFreeFieldHRIR",
        SOFAConventionsVersion: "1.0",
        DataType:               sofa.DataTypeFIR,
        Title:                  "Synthetic HRIR",
        M:                      M, R: R, E: E, N: N,
        SamplingRate: []float64{48000},
        Delay:        []float64{0},
        ListenerPositions: []sofa.Vector3{{X: 0, Y: 0, Z: 0}},
        ReceiverPositions: []sofa.Vector3{
            {X: 0, Y: 0.09, Z: 0},  // left ear
            {X: 0, Y: -0.09, Z: 0}, // right ear
        },
        SourcePositions:  []sofa.Vector3{{X: 1, Y: 0, Z: 0}},
        EmitterPositions: []sofa.Vector3{{X: 0, Y: 0, Z: 0}},
    }

    // [M][R][N] impulse responses
    f.ImpulseResponses = make([][][]float64, M)
    for m := range M {
        f.ImpulseResponses[m] = make([][]float64, R)
        for r := range R {
            f.ImpulseResponses[m][r] = make([]float64, N)
            f.ImpulseResponses[m][r][0] = 1.0 // unit impulse at t=0
        }
    }

    if err := f.Save("synthetic_hrir.sofa"); err != nil {
        log.Fatal(err)
    }
}
```

#### Round-trip: open, modify, save

```go
f, err := sofa.Open("input.sofa")
if err != nil {
    log.Fatal(err)
}
defer f.Close()

// Update some metadata.
f.Title = "Modified copy"
f.History = f.History + "\nResaved by my-tool"

// Halve every impulse response in place.
for m := range f.M {
    for r := range f.R {
        for n := range f.N {
            f.ImpulseResponses[m][r][n] *= 0.5
        }
    }
}

if err := f.Save("output.sofa"); err != nil {
    log.Fatal(err)
}
```

A round trip keeps what go-sofa does not interpret. `Open` collects global
attributes without a field of their own in `Attributes`, variables such as
`SourceView` or `RoomCornerA` in `Variables`, and further attributes of the
variables `Save` writes in `VariableAttributes`; `Save` writes all three back.
They are plain data you can inspect, edit or add to:

```go
for _, a := range f.Attributes {
    fmt.Printf("%s = %v\n", a.Name, a.Value) // e.g. DatabaseName = CIPIC
}
f.Attributes = append(f.Attributes, sofa.Attribute{Name: "ListenerShortName", Value: "subject_003"})
if len(f.Dropped) > 0 {
    log.Printf("not preserved: %v", f.Dropped) // unsupported data or attribute types
}
```

## Command-line Tools

### sofainfo

Displays a metadata summary for SOFA files. Similar to PasSofa's SofaReader utility.

**Usage:**

```bash
# One or more files
sofainfo myfile.sofa other.sofa

# All .sofa files in the current directory
sofainfo
```

With several files each summary is preceded by a `==> file <==` header.
Errors go to stderr; the exit status is 1 if any file could not be read
and 2 on a usage error (`sofainfo -h` prints the usage).

**Example output:**

```text
Conventions: SOFA
Version: 0.6
SOFAConventions: SimpleFreeFieldHRIR
SOFAConventionsVersion: 0.4
DataType: FIR
RoomType: free field
DateCreated: 2014-03-20 17:35:22
DateModified: 2014-03-20 17:35:22
APIName: ARI SOFA API for Matlab/Octave
APIVersion: 0.4.0
License: No license provided, ask the author for permission
ApplicationName: Demo of the SOFA API
ApplicationVersion: 0.4.0
History: Converted from the CIPIC file format

Number of Measurements: 1250
Number of Receivers: 2
Number of Emitters: 1
Number of DataSamples: 200
SampleRate: 44100
Delay: 2 values [I,R], min 0, max 0: [0 0]
```

The `Delay` line gives the number of values, their netCDF dimensions as
stored in the file (`File.DelayDimensions`: `[I]`, `[I,R]`, `[M]`, `[R]` or
`[M,R]`, so `[M]` and `[R]` are told apart even when M == R), the range and,
for up to 8 values, the values.

### sofa2json

Exports SOFA files to JSON. Enhanced version of PasSofa's SOFA2JSON utility.

**Usage:**

```bash
# Export metadata only (default)
sofa2json myfile.sofa other.sofa

# Include impulse response data (FIR files)
sofa2json --include-ir myfile.sofa

# Include complex transfer-function data (TF / TF-E files)
sofa2json --include-tf myfile.sofa

# Include second-order-section coefficients (SOS files)
sofa2json --include-sos myfile.sofa

# Overwrite existing .json files
sofa2json -f myfile.sofa

# All .sofa files in the current directory
sofa2json --include-ir
```

**Output:** Creates `<filename>.json` next to each input and refuses to
overwrite an existing one unless `-f` is given. Progress and errors go to
stderr; the exit status is 1 if any file failed and 2 on a usage error.

JSON keys are the field names of `sofa.File`: the global attributes
(`Conventions`, `Version`, `SOFAConventions`, `SOFAConventionsVersion`,
`DataType`, `Title`, …), the dimensions `M`, `R`, `E`, `N`,
`SamplingRate`, `Delay`, the positions (`ListenerPositions`,
`ReceiverPositions`, `SourcePositions`, `EmitterPositions`,
`ListenerView`, `ListenerUp`) as `[x, y, z]` triples together with their
`…Type` and `…Units`, and — when requested — `ImpulseResponses`,
`TFReal`/`TFImag` (TF-E: `TFRealE`/`TFImagE`) or `SOSCoefficients`. For
TF and TF-E files `Frequencies` is always included (it is small). NaN
and ±Inf values are written as `null`. The output is streamed, so large
exports need no in-memory copy of the JSON document.

### sofaprobe

Development tool: dumps the HDF5 structure, attributes and dimension
scales of each file, then previews `Data.IR`, `Data.Real`, `Data.Imag` or
`Data.SOS` (shape, first and last values) without reading the whole
dataset. Attribute values go-hdf5 cannot decode are shown inline as
`(unreadable: …)`; failures to read the structure or the data go to stderr
and make the exit status 1.

```bash
sofaprobe myfile.sofa
```

## API Reference

### Types

#### `File`

Holds the contents of a SOFA file — attributes, positions and audio data —
fully loaded by `Open` (which closes the file before returning) or built in
memory for `Save`. It holds no open file handle.

**Fields:**

- `M, R, E, N int` — Dimensions (measurements, receivers, emitters, samples)
- `ImpulseResponses [][][]float64` — The actual IR data `[M][R][N]`
- `SamplingRate []float64` — Sampling rate in Hz (may be per-measurement)
- `Delay []float64` — Delay in samples per measurement
- `ListenerPositions []Vector3` — Listener positions `[M]`
- `ReceiverPositions []Vector3` — Receiver positions `[R]`
- `SourcePositions []Vector3` — Source positions `[M]`
- `EmitterPositions []Vector3` — Emitter positions `[E]`
- `SourcePositionType, SourcePositionUnits string` — Coordinate system of `SourcePositions`, from the dataset's `Type` and `Units` attributes; empty when the file omits them. Same pair for `ListenerPosition…`, `ReceiverPosition…`, and `EmitterPosition…`
- `ListenerUp, ListenerView Vector3` — Listener orientation vectors
- `Frequencies []float64` — Frequency vector `[N]` (TF files only)
- `TFReal, TFImag [][][]float64` — Complex transfer functions `[M][R][N]` (TF files only)
- `Title, DataType, RoomType, License, ...` — AES69 metadata attributes
- `Attributes []Attribute` — Global attributes without a field of their own (sorted by name)
- `Variables []Variable` — Variables go-sofa does not interpret: `Name`, `Dims`, `Shape`, and row-major `Values` (numeric, as float64) or `Chars` (char arrays), plus their `Attributes`
- `VariableAttributes map[string][]Attribute` — Further attributes of the variables `Save` writes, by variable name
- `Dropped []string` — What `Open` could not preserve

**Methods:**

- `Open(path string) (*File, error)` — Reads a SOFA file completely and closes it again
- `Close() error` — Does nothing and returns nil (`Open` holds no open file); kept so existing `defer f.Close()` code compiles
- `Save(path string) error` — Validates the `File` and writes it to disk as a SOFA file
- `SamplingRateScalar() (float64, error)` — Returns the single sampling rate;
  `ErrNoSamplingRate` when none is stored, `ErrVaryingSamplingRate` when the
  per-measurement rates differ
- `SamplingRateAt(m int) (float64, error)` — Sampling rate of measurement m
  (`[I]` broadcast or `[M]`)
- `SourcePositionAt(m int) (Vector3, error)` — Source position of measurement m
  (`[I,C]` broadcast or `[M,C]`)
- `DelayAt(m, r int) (float64, error)` — Delay in samples of measurement m,
  receiver r, for `Data.Delay` stored as `[I]`, `[I,R]`, `[R]`, `[M]` or
  `[M,R]`; 0 when the file has no delay
- `Duration() (float64, error)` — Returns IR duration in seconds (FIR only)
- `IRAt(m, r int) ([]float64, error)` — Returns impulse response for measurement m, receiver r
- `IRPeakdB(m, r int) (float64, error)` — Returns peak level in dB for measurement m, receiver r

The IR accessors fail with `ErrUnsupportedDataType` on non-FIR files, and all
accessors fail with `ErrIndexOutOfRange` for indices outside the file's
dimensions; `Open` fails
with `ErrUnsupportedDataType` for an empty, unknown, `FIR-E` or `FIRE`
`DataType`, and with `ErrNotSOFA` when the `Conventions` attribute is not
`SOFA`. Test for these with `errors.Is`.

`Save` returns every validation failure as a `*ValidationError`. Its `Field`
names the `File` field at fault (`"M"`, `"ImpulseResponses"`,
`"SourcePositionType"`, `"Variables"`, …), and its message starts with that
field:

```go
var ve *sofa.ValidationError
if errors.As(err, &ve) {
    fmt.Printf("fix %s: %v\n", ve.Field, ve.Err)
}
```

`DataTypeFIR`, `DataTypeTF`, `DataTypeTFE` and `DataTypeSOS` are the
`DataType` values the package reads and writes.

#### `Vector3`

One coordinate triplet of a position or orientation. Its units are those the
variable's `Type` and `Units` attributes name: metres for `cartesian`;
azimuth, elevation (degrees or radians) and radius in metres for `spherical`
and `spherical harmonics`.

**Fields:**

- `X, Y, Z float64` — the three coordinates

### Functions

#### `Open(path string) (*File, error)`

Reads a SOFA file. Checks that it is a SOFA file (`Conventions == "SOFA"`, else `ErrNotSOFA`), reads all data and metadata into the returned `File` and closes the file before returning.

**Returns:**

- `*File` — The opened SOFA file
- `error` — Error if file cannot be opened or is not a valid SOFA file

**Example:**

```go
f, err := sofa.Open("myfile.sofa")
if err != nil {
    log.Fatal(err)
}
defer f.Close()
```

#### `(*File).Save(path string) error`

Validates the `File` against AES69 requirements (required attributes,
positive dimensions, consistent array shapes) and its values (every number
finite, sampling rates above zero, frequencies ascending from zero or above,
all for the active `DataType` only; per-measurement `ListenerViews`/`ListenerUps` non-zero), and writes it as a
new SOFA file at `path`. An unset `ListenerView`/`ListenerUp` is written as
the conventions' default, `[1 0 0]`/`[0 0 1]` (spherical: `(0, 0, 1)` /
`(0, 90, 1)`, elevation π/2 when `ListenerViewUnits` is in radians). The destination is created from scratch on
each call; an existing file is overwritten only after validation
succeeds. Works for every supported `DataType` (FIR, TF, TF-E, SOS).

**Returns:**

- `error` — a `*ValidationError` when the `File` is invalid, else an I/O error from the underlying HDF5 writer.

**Example:**

```go
if err := f.Save("output.sofa"); err != nil {
    log.Fatal(err)
}
```

## File Format Support

This library supports SOFA files (AES69-2015) based on HDF5 with netCDF-4 conventions:

- **Conventions:** SimpleFreeFieldHRIR, SimpleFreeFieldHRTF, SimpleFreeFieldHRSH, SimpleFreeFieldSOS, GeneralTF, GeneralTF-E, SingleRoomDRIR, etc. (free-form — any AES69 convention name is accepted)
- **DataTypes:** FIR, TF, TF-E, SOS
- **Storage formats:** Contiguous and chunked datasets
- **Compression:** Deflate-compressed datasets
- **Dimensions:** Standard M, R, E, N dimensions and dimension scales
- **Attributes:** Dense (fractal heap) and compact attribute storage

### Conventions

Any AES69 convention name is accepted and written unchanged. A few
conventions get extra behaviour: `Save` enforces their required
metadata, and `(*File).ConventionWarnings() []string` reports
advisory findings that never block `Save` (`sofainfo` prints them).

| Convention                                   | Accessors                                   | Checks                                                                                    |
| -------------------------------------------- | ------------------------------------------- | ----------------------------------------------------------------------------------------- |
| BRIR: `SingleRoomDRIR`, `MultiSpeakerBRIR`   | `IsBRIR()`                                  | `Save` errors without a `RoomType` or with a zero `ListenerView`/`ListenerUp`             |
| SRIR: `SingleRoomSRIR`, `SingleRoomMIMOSRIR` | `IsSRIR()`, `AmbisonicsOrder() (int, bool)` | Warns when `RoomVolume` or `RoomTemperature` is missing, or when `R` is not `(order+1)²`  |
| `SimpleFreeFieldHRIR`/`HRTF`/`HRSOS`         | —                                           | `Save` requires `DataType` FIR/TF/SOS, `R = 2` and `E = 1`                                |
| `FreeFieldHRTF`                              | `SHOrder()` for SH-encoded files            | `Save` requires `DataType` TF-E                                                           |
| Directivity: e.g. `FreeFieldDirectivityTF`   | `IsDirectivity()`                           | `Save` requires `DataType` TF; more needs an example file. `M` indexes source orientation |

`RoomVolume` (cubic metres) and `RoomTemperature` (kelvin) are
read from their variables, or from root attributes of the same
name, and written as variables when non-zero.

### Spherical-harmonic (SH) HRTFs

AES69-2022 introduced spherical-harmonic representations such as
`SimpleFreeFieldHRSH`. These are stored using the existing `TF-E`
DataType with the emitter dimension `E` repurposed as the SH
coefficient index (`E = (Lmax+1)²`). go-sofa reads and writes such
files via the standard TF-E path; use the helpers below to detect
and inspect SH encoding:

- `(*File).IsSHEncoded() bool` — true when `DataType` is `TF-E`,
  `EmitterPositionType` is `"spherical harmonics"` (AES69's marker; only
  when it is empty do a convention name containing "SH" or a History
  mentioning spherical harmonics count instead) **and** `E` is `(L+1)²`
  for some `L ≥ 0`
- `(*File).SHOrder() (lmax int, ok bool)` — returns `Lmax`
- `(*File).SHCoefficientCount() int` — returns `E` for SH files, 0 otherwise
- `(*File).SHWarnings() []string` — advisory diagnostics for
  ambiguous or malformed SH metadata

To **write** an SH-encoded file, populate a `File` with
`DataType:"TF-E"`, `EmitterPositionType: sofa.CoordinateSphericalHarmonics`,
a convention such as `FreeFieldHRTF`, and `E = (Lmax+1)²` SH
coefficients per (measurement, receiver, frequency) tuple, then call
`Save`.

## Related Projects

- [go-hdf5](https://github.com/cwbudde/go-hdf5) — Pure Go HDF5 library (fork)
- [PasSofa](../PasSofa) — Pascal SOFA reader (reference implementation)
- [SOFA Conventions](https://www.sofaconventions.org/) — Official SOFA specifications
- [libmysofa](https://github.com/hoene/libmysofa) — Lightweight C SOFA reader

## Development

### Building

```bash
# Install dependencies
go mod download

# Build library
go build

# Build CLI tools
go build ./cmd/sofainfo
go build ./cmd/sofa2json
```

### Testing

```bash
# Run tests
go test -v ./...

# Run tests with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Using just

This project includes a [justfile](https://github.com/casey/just) for common development tasks:

```bash
# Run all checks (format, lint, test, tidy)
just check

# Format code
just fmt

# Run linter
just lint

# Run tests
just test

# Run tests with coverage
just test-coverage

# Build CLI tools
just build
```

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please ensure that:

1. All tests pass (`just test`)
2. Code is formatted (`just fmt`)
3. Linter is clean (`just lint`)
4. `go.mod` is tidy (`just check-tidy`)

## References

- [AES69-2015: AES standard for file exchange - Spatial acoustic data file format](https://www.aes.org/publications/standards/search.cfm?docID=99)
- [SOFA Conventions Website](https://www.sofaconventions.org/)
- [NetCDF User's Guide](https://www.unidata.ucar.edu/software/netcdf/docs/)
- [HDF5 File Format Specification](https://portal.hdfgroup.org/display/HDF5/File+Format+Specification)
