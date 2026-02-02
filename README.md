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

```
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

# Include impulse response data
sofa2json --include-ir myfile.sofa

# Batch process all .sofa files
sofa2json
sofa2json --include-ir
```

**Output:** Creates `<filename>.json` in the same directory.

**Note:** Metadata-only export produces ~700 bytes. With `--include-ir`, output can be several megabytes depending on IR size.

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
- `Title, DataType, RoomType, License, ...` — AES69 metadata attributes

**Methods:**

- `Open(path string) (*File, error)` — Opens a SOFA file for reading
- `Close() error` — Closes the file and releases resources
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

## File Format Support

This library supports SOFA files (AES69-2015) based on HDF5 with netCDF-4 conventions:

- **Conventions:** SimpleFreeFieldHRIR, SimpleFreeFieldSOS, SingleRoomDRIR, etc.
- **Storage formats:** Contiguous and chunked datasets
- **Compression:** Deflate-compressed datasets
- **Dimensions:** Standard M, R, E, N dimensions and dimension scales
- **Attributes:** Dense (fractal heap) and compact attribute storage

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
