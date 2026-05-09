# go-sofa — SOFA (AES69) File Reader in Go

## Goal

A pure-Go library for reading (and eventually writing) SOFA files
(AES69 — Spatially Oriented Format for Acoustics), built on top of
[MeKo-Christian/go-hdf5](https://github.com/MeKo-Christian/go-hdf5)
(our fork of scigolib/hdf5). The Go equivalent of
`PasSofa/SofaFile.pas`.

## Prior Art & Key Resources

| Resource                                                                                | Notes                                                                                  |
| --------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| `../PasSofa/Source/SofaFile.pas`                                                        | Our own SOFA reader (~430 LOC Pascal). Reads Data.IR, positions, all AES69 attributes. |
| `../go-hdf` (go-hdf5 fork)                                                              | Pure-Go HDF5 read+write. Provides the low-level file access.                           |
| [SOFA / AES69](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications) | SOFA convention specs (netCDF-4 / HDF5 based).                                         |
| [libmysofa](https://github.com/hoene/libmysofa)                                         | Lightweight C SOFA reader — useful as behavioural reference.                           |

## Repository Setup

```text
go-sofa/
├── sofa.go          # File type, Open(), data accessors
├── sofa_test.go     # Unit + integration tests
├── cmd/
│   ├── sofainfo/    # Metadata summary CLI
│   └── sofa2json/   # JSON export CLI
├── testdata/        # .sofa sample files
├── PLAN.md
├── go.mod           # module github.com/MeKo-Christian/go-sofa
└── go.sum
```

Dependency: `github.com/MeKo-Christian/go-hdf5` (or `github.com/scigolib/hdf5`
until the fork's module path is renamed).

---

## Phase 0 — Project Scaffold

- [x] `go mod init github.com/MeKo-Christian/go-sofa`
- [x] `go get` the HDF5 dependency (go-hdf5 fork).
- [x] Add a few real `.sofa` test files to `testdata/` (e.g. from
      <https://www.sofaconventions.org/>).
- [x] Skeleton `sofa.go` with package doc and placeholder types.

---

## Phase 1 — Validate HDF5 Reading for SOFA Needs

**Goal:** Confirm go-hdf5 can read everything a `.sofa` file contains.

SOFA files use these HDF5 features:

- Groups (root + nested `Data` group)
- Datasets with float64 arrays (1D, 2D, 3D)
- Global string attributes (Conventions, Title, DataType, …)
- Dataset attributes (`CLASS=DIMENSION_SCALE`, `NAME=…`)
- Contiguous and/or chunked storage with optional deflate

### Tasks

- [x] Open a real `.sofa` file with `hdf5.Open()`.
- [x] Walk groups and datasets, list attributes.
- [x] Read a multi-dimensional float64 dataset (`Data.IR`).
- [x] Read string attributes from root group.
      **Gap found:** Root group attributes use dense (fractal heap) storage
      in all tested SOFA files (netCDF4 creates >8 attributes, exceeding
      compact threshold). `Group.Attributes()` returns empty.
      See `TestReadRootGroupAttributes` (currently skipped).
- [x] Read dimension-scale attributes from dimension datasets.
- [x] Write a standalone integration test that exercises all of the above.
      `TestSOFAIntegration` runs across all 3 test files.
- [x] Document any gaps or bugs found in go-hdf5; file issues there.

### go-hdf5 Gaps Found

1. **Group.Attributes() returns empty for dense attribute storage.**
   **Blocker for Phase 2** — cannot read AES69 global attributes
   (Conventions, DataType, Title, …). Needs fix in go-hdf5.

2. **Dataset.Info() returns "missing required messages" for dimension-scale
   datasets (M, R, N, E).** Low impact — data is still readable, only
   metadata display affected. Cosmetic.

---

## Phase 2 — SOFA Package (Read Support) ✅ COMPLETE

**Goal:** Implement `sofa.Open()` and the `File` type — the Go
equivalent of `../PasSofa/SofaFile.pas`.

### Data Structures

```go
type Vector3 struct{ X, Y, Z float64 }

type File struct {
    // Dimensions
    M int // measurements
    R int // receivers
    E int // emitters
    N int // data samples

    // Spatial data
    ListenerPositions []Vector3   // [M]
    ListenerUp        Vector3
    ListenerView      Vector3
    ReceiverPositions []Vector3   // [R]
    SourcePositions   []Vector3   // [M]
    EmitterPositions  []Vector3   // [E]

    // Audio data
    ImpulseResponses [][][]float64 // [M][R][N]
    SampleRate       []float64     // [M] or scalar
    Delay            []float64     // [M]

    // Metadata (AES69 global attributes)
    Title, DataType, RoomType     string
    DateCreated, DateModified     string
    APIName, APIVersion           string
    AuthorContact, Organization   string
    License, Comment, History     string
    ApplicationName, ApplicationVersion string
    References, Origin            string
}
```

### Tasks

- [x] `sofa.Open(path) (*File, error)` — opens HDF5, validates
      `Conventions == "SOFA"`.
- [x] Parse dimension scales (M, R, E, N) from netCDF convention
      attributes (`CLASS=DIMENSION_SCALE`).
- [x] Read `Data.IR` → `[M][R][N]float64`.
- [x] Read `Data.SamplingRate`, `Data.Delay`.
- [x] Read listener/receiver/source/emitter positions.
- [x] Read all AES69 global attributes.
- [x] Validate dimensional consistency (same assertions as PasSofa).
- [x] `File.Close()` to release the underlying HDF5 file.
- [x] Tests with real `.sofa` files.
- [x] Compare output against PasSofa for the same input files.

### go-hdf5 Fixes Required & Completed

1. **✅ Fixed B-tree Type 8 record layout** — Corrected heap ID parsing in
   attribute name index B-trees.
2. **✅ Fixed Attribute Info Message encoding** — MaxCreationIndex now
   correctly encoded as uint64 instead of uint16.
3. **✅ Fixed fractal heap indirect block traversal** — Simplified offset
   calculation for heap ID parsing.
4. **✅ Added Group.ReadAttribute() convenience method** — Parity with
   Dataset API.
5. **✅ Cleaned up debug prints** — Removed all temporary debug output.

---

## Phase 3 — CLI Tools ✅ COMPLETE

**Goal:** Command-line utilities mirroring PasSofa examples.

### Tasks

- [x] `cmd/sofainfo/main.go` — prints SOFA metadata summary (like
      `PasSofa/SofaReader.dpr`).
- [x] `cmd/sofa2json/main.go` — reads `.sofa`, outputs JSON with `--include-ir`
      flag to optionally include impulse response data (enhanced from
      `PasSofa/SOFA2JSON.dpr`).

### Features Implemented

**sofainfo:**

- Prints human-readable metadata summary
- Shows all AES69 global attributes (if non-empty)
- Displays dimensions (M, R, E, N) and audio parameters (sample rate, delay)
- Single file mode: `sofainfo <file.sofa>`
- Batch mode: `sofainfo` (processes all .sofa in current directory)

**sofa2json:**

- Exports SOFA data to JSON format
- `--include-ir` flag controls whether IR data is exported (default: metadata only)
- Output written to `<filename>.json` (replaces .sofa extension)
- Single file mode: `sofa2json [--include-ir] <file.sofa>`
- Batch mode: `sofa2json [--include-ir]` (processes all .sofa in current directory)
- JSON includes: metadata, dimensions, SampleRate array, Delay array, IR data (if flag set)

---

## Phase 4 — Polish & CI

### Tasks

- [x] GoDoc for all exported types.
- [x] README with usage examples.
- [ ] `go vet` / `golangci-lint` clean.
- [x] GitHub Actions workflow: build, test, lint.
- [x] Test coverage target: ≥ 80 %.

### Test Coverage ✅

**Achieved: 84.2%** (Target: ≥80%)

The library has excellent test coverage with 20 comprehensive test cases:

**Coverage by Function:**

- `Close`: 100% (was 66.7%) - nil handling tested
- `parseDimensionSize`: 100% (was 80%) - error cases added
- `SamplingRateScalar`: 100% (was 66.7%) - empty array handling
- `Duration`: 100% (was 75%) - edge cases (zero rate, zero samples, empty)
- `IRAt`: 100% - out-of-range handling
- `reshapeIR`: 100% - array transformation
- `readSpatialData`: 100% - position/orientation reading
- `readGlobalAttributes`: 93.3% - AES69 attributes
- `readVector3s`: 90% - Vector3 parsing
- `IRPeakdB`: 90% - peak level calculation
- `readDimensions`: 75% - dimension reading
- `readAudioData`: 73.7% - audio data
- `Open`: 60% (was 56%) - error paths tested

**Test Suite:**

- **Integration tests:** 3 real SOFA files from sofaconventions.org
- **Unit tests:** Dimension parsing, attribute reading, data access
- **Error handling:** Invalid files, missing data, out-of-range access
- **Edge cases:** Empty arrays, nil values, zero dimensions
- **Files tested:** MIT_KEMAR_normal_pinna.sofa, CIPIC_subject_003_hrir_final.sofa, tester.sofa

**Improvements from 80.8% → 84.2%:**

- Added error path tests for `Open()` (nonexistent files, invalid paths)
- Added error tests for `parseDimensionSize()` (empty strings, invalid numbers)
- Added edge case tests for `Duration()` (zero rate, zero samples, empty arrays)
- Added nil handling test for `Close()`
- Added empty array test for `SamplingRateScalar()`

**Note:** CLI tools (sofainfo, sofa2json, sofaprobe) have 0% coverage as they are simple wrappers around the library and are manually tested.

### Documentation ✅

**GoDoc:** All exported types and functions have comprehensive documentation:

- `File` type — Complete field documentation for dimensions, spatial data, audio data, and metadata
- `Vector3` type — 3D coordinate representation
- `Open()` — Opens and validates SOFA files
- `Close()` — Resource cleanup
- `SamplingRateScalar()`, `Duration()` — Audio property accessors
- `IRAt()`, `IRPeakdB()` — Impulse response accessors

**README.md:** Comprehensive user guide with:

- Library installation and basic usage examples
- Advanced examples for accessing impulse responses, spatial data, and metadata
- Command-line tool documentation (sofainfo, sofa2json)
- Complete API reference for all public types and methods
- Development instructions with just commands
- Related projects and references

### CI Implementation ✅

GitHub Actions workflows have been set up mirroring the gll-tools project:

- **test.yaml** — Main workflow orchestrating all checks on every push
- **test-unit.yaml** — Runs `just test` for unit tests
- **test-lint.yaml** — Runs `golangci-lint` and verifies `go.mod` tidiness
- **test-format.yaml** — Checks code formatting with treefmt (gofumpt, gci, shfmt, prettier)

All workflows use concurrency control to cancel previous runs, leverage Go module caching, and integrate with existing project configuration (justfile, treefmt.toml, .golangci.toml).

---

## Phase 5 — SOFA Write Support ✅ COMPLETE

**Status**: ✅ Complete with limitations (dataset attributes not yet supported)

**Goal**: Enable full round-trip capability: read a SOFA file, modify it, and save it back.

### Design Decisions ✅

- **API**: `func (f *File) Save(path string) error` on existing `File` struct
- **Approach**: Create new file from scratch each time (simple, safe, no corruption risk)
- **Validation**: Strict validation of all required SOFA fields before writing
- **netCDF Compliance**: Full netCDF-4/HDF5 dimension-scale metadata for ecosystem compatibility

### Implementation Progress

#### ✅ Completed

1. **Validation Method** ([sofa.go:470-548](sofa.go))
   - `validate()` checks all required fields (Conventions, Version, SOFAConventions, DataType)
   - Validates dimensions M, R, E, N > 0
   - Validates IR array dimensions match M×R×N
   - Validates SamplingRate and Delay array lengths
   - Validates position array dimensions
   - **Tests**: `TestSaveValidation` (11 test cases, all passing)

2. **Helper Functions** ([sofa.go:670-767](sofa.go))
   - `flattenIR()` - converts [M][R][N]float64 to []float64
   - `flattenVector3s()` - converts []Vector3 to []float64
   - `writeDimensionScale()` - writes netCDF dimension-scale datasets
   - `writePositionDataset()` - writes position datasets (N×3 arrays)
   - `writeVector3Dataset()` - writes Vector3 datasets

3. **Core Save Method** ([sofa.go:407-466](sofa.go))
   - `Save()` method with validation, attribute writing, dataset creation
   - Writes dimension scales (M, R, E, N) with netCDF attributes
   - Writes spatial positions (Listener, Receiver, Source, Emitter)
   - Writes orientation vectors (ListenerUp, ListenerView)
   - Writes audio data (Data.IR, Data.SamplingRate, Data.Delay)

4. **Test Suite** ([sofa_write_test.go](sofa_write_test.go))
   - ✅ `TestSaveValidation` - 11 validation scenarios (all passing)
   - ✅ `TestSaveMinimal` - minimal file creation (passing)
   - ✅ `TestSaveRoundTrip` - full round-trip test (passing - all 3 test files)
   - ✅ `TestSaveModifyRoundTrip` - modify and save (passing)

5. **go-hdf5 Enhancement** ([/mnt/projekte/Code/go-hdf/group_write.go:186-199](../go-hdf/group_write.go))
   - Added `RootGroup()` method to `FileWriter`
   - Enables writing attributes to root "/" group

#### ✅ Root Group Attribute Support - COMPLETE

**Solution Implemented**: go-hdf5 now supports `WithRootAttribute()` options in `CreateForWrite()`

**Implementation**:

- Root attributes are specified during file creation
- go-hdf5 pre-allocates correct object header size
- Supports both compact (≤8 attributes) and dense (>8 attributes) storage
- SOFA files with 20+ attributes work correctly

**Remaining Limitation**: Dataset attributes

- Writing attributes to datasets after creation still causes corruption
- Dimension-scale attributes (CLASS, NAME) are currently skipped
- Files are valid HDF5 but not fully netCDF-4 compliant
- Future enhancement: Add `WithAttribute()` option to `CreateDataset()` in go-hdf5

### Implementation Summary

**Phase 1: go-hdf5 Enhancement** ✅ Complete

- Added `WithRootAttribute()` functional options to `CreateForWrite()`
- Implemented compact storage (≤8 attributes) in object header
- Implemented dense storage (>8 attributes) with Fractal Heap + B-tree
- Tested with 0, 1, 5, 8, 20, 50 attributes
- All round-trip tests pass

**Phase 2: go-sofa Integration** ✅ Complete

- Updated `Save()` to collect all root attributes upfront
- Pass attributes via `WithRootAttribute()` options to `CreateForWrite()`
- Removed `writeGlobalAttrs()` function (no longer needed)
- Updated dimension reading to handle files with/without NAME attributes
- Relaxed validation to match SOFA spec (scalar positions, various Delay dimensions)

**Phase 3: Testing & Validation** ✅ Complete

- All 4 write test suites pass (TestSaveValidation, TestSaveMinimal, TestSaveRoundTrip, TestSaveModifyRoundTrip)
- Round-trip tests with 3 real SOFA files successful
- Files can be created, saved, reopened, and modified
- Full test suite coverage: 100% of write tests passing

### Files Modified

**go-sofa**:

- ✅ [sofa.go](sofa.go) - Added Save(), validate(), helper functions (373 lines)
- ✅ [sofa_write_test.go](sofa_write_test.go) - Comprehensive test suite (476 lines, all passing)
- ⏳ [README.md](README.md) - Needs write examples
- ✅ [PLAN.md](PLAN.md) - This section (updated)

**go-hdf5**:

- ✅ [/mnt/projekte/Code/go-hdf/dataset_write.go](../go-hdf/dataset_write.go) - Added WithRootAttribute() and root attribute support
- ✅ [/mnt/projekte/Code/go-hdf/attribute_write.go](../go-hdf/attribute_write.go) - Dense attribute storage implementation
- ✅ [/mnt/projekte/Code/go-hdf/README.md](../go-hdf/README.md) - Added root attribute documentation
- ✅ [/mnt/projekte/Code/go-hdf/PLAN.md](../go-hdf/PLAN.md) - Phase 3 complete

### Success Criteria

1. ✅ **Validation**: All required fields validated before writing
2. ✅ **File Creation**: Create valid HDF5/SOFA files from scratch
3. ✅ **Round-Trip**: Open → Modify → Save → Reopen successfully (all 3 test files pass)
4. ⚠️ **Compliance**: Files readable by our library; h5dump compatibility pending investigation
5. ✅ **Test Coverage**: 100% of write tests passing (4/4 test suites)
6. ✅ **Performance**: Handles real SOFA files (up to 710 measurements)

### Future Enhancements

1. **Dataset Attributes** (Optional)
   - Add `WithAttribute()` option to `CreateDataset()` in go-hdf5
   - Enable dimension-scale attributes (CLASS, NAME) for full netCDF-4 compliance
   - Would allow better interoperability with MATLAB SOFA toolbox

2. **Documentation** (Recommended)
   - Add write examples to README.md
   - Document known limitations (dataset attributes)
   - Add example of creating SOFA file from scratch

3. **Extended Testing** (Optional)
   - Test with larger SOFA files (>100MB)
   - Validate with MATLAB SOFA toolbox if available
   - Performance benchmarks for large file creation

### References

- [Detailed Implementation Plan](/home/christian/.claude/plans/sofa-write-implementation.md)
- [SOFA Specifications](https://www.sofaconventions.org/mediawiki/index.php/SOFA_specifications)
- [netCDF-4/HDF5 File Format](https://docs.unidata.ucar.edu/netcdf-c/current/file_format_specifications.html)
- [HDF5 Object Header Specification](https://docs.hdfgroup.org/hdf5/develop/_s_p_e_c.html#OHDRLayout)

---

## Phase 5b — TF (Transfer Function) DataType ✅ COMPLETE

**Goal:** Support SOFA files where `DataType == "TF"` (complex
frequency-domain data) alongside the existing FIR path. TF
representations are used by conventions such as `GeneralTF`,
`FreeFieldDirectivityTF`, and `SimpleFreeFieldHRTF`.

### What's implemented

- **Data structure** ([sofa.go](sofa.go)) — `File` carries the FIR
  triple (`ImpulseResponses`, `SamplingRate`, `Delay`) and the TF
  triple (`Frequencies` `[N]`, `TFReal`/`TFImag` `[M][R][N]`); the
  active set is determined by `DataType`.
- **Read** — `readAudioData` dispatches FIR vs. TF;
  `readTFAudioData` reads `/N` as a frequency vector and
  `/Data.Real`/`/Data.Imag` as `[M][R][N]`. `readDimensions` accepts
  both scalar `/N` (FIR) and vector `/N` (TF).
- **Write** — `writeAudioDatasets` dispatches by `DataType`;
  `writeTFAudioDatasets` emits `Data.Real`/`Data.Imag` and
  `writeFrequencyDimension` writes `/N` as a vector.
- **Validation** — `validate()` switches on `DataType`; `validateTF`
  checks `len(Frequencies) == N` and `[M][R][N]` shape for both real
  and imaginary parts.
- **Tests** ([sofa_tf_roundtrip_test.go](sofa_tf_roundtrip_test.go)) —
  `TestTFRoundTrip` (synthetic 4×1×1×5 file, all values within
  1e-12) and `TestValidateTFRejectsMissingFields` (4 cases).
- **CLI** — `sofainfo` prints the frequency count and range when
  `DataType == "TF"`. `sofa2json` exposes `--include-tf` (parallel
  to `--include-ir`); the `Frequencies` vector is always included
  for TF files (small), while `TFReal`/`TFImag` are gated by the
  flag.

### Known limitations

- **Real-world TF testdata** — `testdata/GeneralTF_2.0.sofa`,
  `testdata/GeneralTF-E_1.0.sofa`, `testdata/FreeFieldHRTF_2.0.sofa`,
  etc. exist but currently fail to open with
  `data layout message not found`. This is a go-hdf5 issue unrelated
  to TF support; tracked separately.
- **Edge case** — if a TF file legitimately has `N == 1`, `/N`
  appears scalar on disk and `readDimensions` cannot disambiguate
  from FIR. Acceptable; deferred.
- **Dataset attributes** — same `CLASS`/`NAME` limitation as Phase 5
  applies to TF datasets.

---

## Phase 6 — Optional / Future

- [ ] **SOFA 2.0**: spherical harmonics receiver representation (AES69-2022).
- [ ] **Additional SOFA conventions**: BRIR, SRIR, directivity, etc.
- [ ] **Performance**: hyperslab reads for partial IR loading.

---

## Testing Strategy

| Level       | What                                       | How                       |
| ----------- | ------------------------------------------ | ------------------------- |
| Unit        | Dimension extraction, attribute parsing    | Table-driven Go tests     |
| Integration | Open real `.sofa` files, verify all fields | Golden output vs. PasSofa |
| Fuzz        | Attribute parsing, dimension validation    | `go test -fuzz`           |
| Benchmark   | Large SOFA file read throughput            | `testing.B`               |

---

## Risk Register

| Risk                                       | Impact | Mitigation                                    |
| ------------------------------------------ | ------ | --------------------------------------------- |
| go-hdf5 API changes                        | Medium | Pin dependency version; coordinate with fork. |
| SOFA files using unsupported HDF5 features | Medium | Phase 1 validation; fix in go-hdf5.           |
| SOFA convention evolution (2.0+)           | Low    | Start with SOFA 1.0; add 2.0 later.           |
