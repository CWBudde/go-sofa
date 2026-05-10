# go-sofa

A pure-Go library for reading SOFA files (Spatially Oriented Format for Acoustics, AES69-2015).

SOFA is a file format for storing spatially oriented acoustic data like head-related transfer functions (HRTFs), binaural room impulse responses (BRIRs), and directional room impulse responses (DRIRs). The format is based on HDF5 and follows the netCDF-4 conventions.

## Features

- **Pure Go implementation** — No C dependencies
- **Full AES69 support** — Reads all standard SOFA metadata and data arrays
- **Built on go-hdf5** — Leverages [MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5) for HDF5 file access
- **Command-line tools** — Includes `sofainfo` and `sofa2json` utilities
- **Well-tested** — Validated against reference SOFA files from sofaconventions.org

## Installation

### Library

```bash
go get github.com/MeKo-Christian/go-sofa
```

### Command-line tools

```bash
go install github.com/MeKo-Christian/go-sofa/cmd/sofainfo@latest
go install github.com/MeKo-Christian/go-sofa/cmd/sofa2json@latest
```

## Library Usage

### Basic example

```go
package main

import (
    "fmt"
    "log"

    "github.com/MeKo-Christian/go-sofa"
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
    fmt.Printf("Sample Rate: %.0f Hz\n", f.SamplingRateScalar())
    fmt.Printf("Duration: %.3f seconds\n", f.Duration())
}
```

### Accessing impulse responses

```go
// Get impulse response for measurement 0, receiver 0 (left ear)
ir := f.IRAt(0, 0)
if ir != nil {
    fmt.Printf("IR samples: %d\n", len(ir))
    fmt.Printf("Peak level: %.1f dB\n", f.IRPeakdB(0, 0))
}

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

if f.DataType == "TF" {
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

    "github.com/MeKo-Christian/go-sofa"
)

func main() {
    const M, R, E, N = 1, 2, 1, 64
    f := &sofa.File{
        Conventions:            "SOFA",
        Version:                "1.0",
        SOFAConventions:        "SimpleFreeFieldHRIR",
        SOFAConventionsVersion: "1.0",
        DataType:               "FIR",
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

#### Known limitations

- Dataset attributes (`CLASS`, `NAME`) are not yet emitted, so written
  files are valid HDF5/SOFA for go-sofa but not fully netCDF-4
  compliant. Some third-party tools (e.g. the MATLAB SOFA Toolbox)
  may flag the missing dimension-scale metadata.

## Command-line Tools

### sofainfo

Displays metadata summary for SOFA files. Similar to PasSofa's SofaReader utility.

**Usage:**

```bash
# Process single file
sofainfo myfile.sofa

# Process all .sofa files in current directory
sofainfo
```

**Example output:**

```text
Title: CIPIC subject 003
DataType: FIR
DateCreated: 2013-10-17 15:30:00
APIName: ARI SOFA API for Matlab/Octave
APIVersion: 0.4.3
Organization: Acoustics Research Institute
License: Creative Commons Attribution-NonCommercial-ShareAlike 4.0

Number of Measurements: 1250
Number of Receivers: 2
Number of Emitters: 1
Number of DataSamples: 200
SampleRate: 44100
Delay: 0
```

### sofa2json

Exports SOFA files to JSON format. Enhanced version of PasSofa's SOFA2JSON utility.

**Usage:**

```bash
# Export metadata only (default)
sofa2json myfile.sofa

# Include impulse response data (FIR files)
sofa2json --include-ir myfile.sofa

# Include complex transfer-function data (TF files)
sofa2json --include-tf myfile.sofa

# Batch process all .sofa files
sofa2json
sofa2json --include-ir
sofa2json --include-tf
```

**Output:** Creates `<filename>.json` in the same directory.

**Note:** Metadata-only export produces ~700 bytes. With `--include-ir`
or `--include-tf`, output can be several megabytes depending on data
size. For TF files the `Frequencies` vector is always included (it is
small); `TFReal`/`TFImag` are emitted only with `--include-tf`.

## API Reference

### Types

#### `File`

Represents an open SOFA file with all its data and metadata.

**Fields:**

- `M, R, E, N int` — Dimensions (measurements, receivers, emitters, samples)
- `ImpulseResponses [][][]float64` — The actual IR data `[M][R][N]`
- `SamplingRate []float64` — Sampling rate in Hz (may be per-measurement)
- `Delay []float64` — Delay in samples per measurement
- `ListenerPositions []Vector3` — Listener positions `[M]`
- `ReceiverPositions []Vector3` — Receiver positions `[R]`
- `SourcePositions []Vector3` — Source positions `[M]`
- `EmitterPositions []Vector3` — Emitter positions `[E]`
- `ListenerUp, ListenerView Vector3` — Listener orientation vectors
- `Frequencies []float64` — Frequency vector `[N]` (TF files only)
- `TFReal, TFImag [][][]float64` — Complex transfer functions `[M][R][N]` (TF files only)
- `Title, DataType, RoomType, License, ...` — AES69 metadata attributes

**Methods:**

- `Open(path string) (*File, error)` — Opens a SOFA file for reading
- `Close() error` — Closes the file and releases resources
- `Save(path string) error` — Validates the `File` and writes it to disk as a SOFA file
- `SamplingRateScalar() float64` — Returns sampling rate as scalar (first value)
- `Duration() float64` — Returns IR duration in seconds
- `IRAt(m, r int) []float64` — Returns impulse response for measurement m, receiver r
- `IRPeakdB(m, r int) float64` — Returns peak level in dB for measurement m, receiver r

#### `Vector3`

Represents a 3D coordinate in meters.

**Fields:**

- `X, Y, Z float64` — Cartesian coordinates

### Functions

#### `Open(path string) (*File, error)`

Opens a SOFA file for reading. Validates that the file is a valid SOFA file (checks `Conventions == "SOFA"`) and reads all data and metadata.

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
positive dimensions, consistent array shapes) and writes it as a
new SOFA file at `path`. The destination is created from scratch on
each call; an existing file is overwritten only after validation
succeeds. Works for both `DataType == "FIR"` and `DataType == "TF"`.

**Returns:**

- `error` — Validation error or I/O error from the underlying HDF5 writer.

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

### Spherical-harmonic (SH) HRTFs

AES69-2022 introduced spherical-harmonic representations such as
`SimpleFreeFieldHRSH`. These are stored using the existing `TF-E`
DataType with the emitter dimension `E` repurposed as the SH
coefficient index (`E = (Lmax+1)²`). go-sofa reads and writes such
files via the standard TF-E path; use the helpers below to detect
and inspect SH encoding:

- `(*File).IsSHEncoded() bool` — true when convention name or
  History attribute declares SH **and** `E` is a perfect square ≥ 4
- `(*File).SHOrder() (lmax int, ok bool)` — returns `Lmax`
- `(*File).SHCoefficientCount() int` — returns `E` for SH files, 0 otherwise
- `(*File).SHWarnings() []string` — advisory diagnostics for
  ambiguous or malformed SH metadata

To **write** an SH-encoded file, populate a `File` with
`DataType:"TF-E"`, `SOFAConventions:"FreeFieldHRSH"` (or any
convention name containing "SH"), and `E = (Lmax+1)²` SH
coefficients per (measurement, receiver, frequency) tuple, then call
`Save`.

## Related Projects

- [go-hdf5](https://github.com/MeKo-Christian/go-hdf5) — Pure Go HDF5 library (fork)
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
