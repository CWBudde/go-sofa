# go-sofa — Plan after the 2026-09-26 review

## Context

A six-area review of v0.2.0 (API, spec correctness, robustness, performance,
tests/CI, docs/hygiene) rated the library **≈5/10 overall**:

| Area                  | Score | Headline                                                              |
| --------------------- | ----: | --------------------------------------------------------------------- |
| SOFA spec correctness |     4 | libmysofa cannot open any file we write; `Save` accepts invalid files |
| Performance / memory  |     4 | `OpenLazy` 127× slower than eager on chunked files (cache not sized)  |
| Docs / CLIs / hygiene |     5 | v0.2.0 never tagged; README write example fails at runtime            |
| API design            |     6 | `File` invariants only checked in `Save`; 2×2 `Open*` matrix          |
| Robustness            |     6 | go-hdf5 panics on a 1-byte mutation; no total memory budget           |
| Tests / CI            |     7 | 17 real-file tests always skip in CI; fuzz target barely executes     |

The read path is solid. The problems are **interop of written files**,
**validation that does not match the README's claims**, and a few missing
knobs. Phases are ordered by impact; within a phase, tasks are ordered by
dependency. Goal: a solid library, not a gold-plated one — items marked
_(optional)_ may be dropped.

Conventions for this file: tick `[x]` when merged, add a dated one-line note
under the task if the outcome differs from the plan. History stays in
`git log` / CHANGELOG.md; design decisions go to
[docs/design-notes.md](docs/design-notes.md).

### Useful tooling for this plan

- **libmysofa check harness** (needed for Phase 1): clone
  `https://github.com/hoene/libmysofa`, `cmake -B build -DBUILD_TESTS=OFF &&
cmake --build build`, then compile this against the static lib:

  ```c
  #include <stdio.h>
  #include "mysofa.h"
  int main(int argc, char **argv) {
    for (int i = 1; i < argc; i++) {
      int err = 0;
      struct MYSOFA_HRTF *h = mysofa_load(argv[i], &err);
      if (!h) { printf("%s: load err %d\n", argv[i], err); continue; }
      err = mysofa_check(h);
      printf("%s: check %d\n", argv[i], err);
      mysofa_free(h);
    }
    return 0;
  }
  ```

  Baseline on 2026-09-26: `testdata/MIT_KEMAR_normal_pinna.sofa` → `check 0`;
  the same file after `Open`+`Save` → `load err 10001`
  (`MYSOFA_UNSUPPORTED_FORMAT`).

- `ncdump -h file.sofa` (Homebrew `netcdf`) shows dims per variable; use it
  to compare original vs. resaved files.
- go-hdf5 local clone: `../go-hdf` (remote `cwbudde/go-hdf5`). It is at
  v0.16.1 while go-sofa requires **v0.17.0** — pull before working in it.

---

## Phase 0 — Release hygiene (do first, ~1 hour) — ✅ DONE (2026-09-26)

The documented install commands are broken today; nothing else matters to a
new user until this is fixed.

- [x] **P0a. Tag and release v0.2.0.** (2026-09-26) — tagged `v0.2.0` on
      `fbf29c0` and published the GitHub release with the CHANGELOG section;
      the proxy resolves `@latest` to v0.2.0 and
      `go install github.com/CWBudde/go-sofa/cmd/sofainfo@latest` works.
      Original task: only `v0.1.0` exists (remote and proxy).
      v0.1.0's `go.mod` declares `github.com/cwbudde/go-sofa`, so
      `go install github.com/CWBudde/go-sofa/cmd/sofainfo@latest` fails with
      "module declares its path as …". Push the `v0.2.0` tag on `fbf29c0`
      (or later), create a GitHub release with the CHANGELOG section, and
      verify `GOPROXY=https://proxy.golang.org go list -m
github.com/CWBudde/go-sofa@latest` returns v0.2.0.
- [x] ~~**P0b. Retire the lowercase module path.**~~ (2026-09-26) — won't
      do: the proxy lists both casings from the same tags (`go list -m
-versions` for either path shows v0.1.0, whose go.mod says lowercase), so
      once v0.2.0 exists, `@latest` on the lowercase path stops at v0.2.0's
      path mismatch and never reads a retract in v0.1.1. That mismatch error
      already names the right path. Original task: Users on
      `github.com/cwbudde/go-sofa` stay on v0.1.0 (which CHANGELOG itself calls
      broken) with no signal. On a throwaway branch from the v0.1.0 commit, set
      in `go.mod`:
      `// Deprecated: use github.com/CWBudde/go-sofa` plus `retract v0.1.0`,
      tag `v0.1.1`. Check with
      `go list -m -retracted -versions github.com/cwbudde/go-sofa`.
      _Caveat:_ GitHub resolves both casings to the same repo, so the tag lives
      in the same repo; the proxy caches per module path — that's fine.
- [x] **P0c. Decide the go-hdf5 casing.** (2026-09-26) — kept
      `cwbudde/go-hdf5`; both README links now use it, and the feature bullet
      names it as our maintained fork of scigolib/hdf5. Original task: go-sofa
      is `CWBudde/…`, the
      dependency is `cwbudde/go-hdf5`, README.md:13 links `CWBudde/go-hdf5`.
      Pick one (recommend: leave go-hdf5 as is, fix the README link) and write
      the relationship (fork of scigolib/hdf5, maintained by us, upstreaming
      plan or not) in one README sentence.
- [x] **P0d. Clean the working tree root.** (2026-09-26) — deleted the
      artefacts locally (they were gitignored, not untracked);
      `just test-coverage` writes `bin/coverage.{out,html}`; dropped the
      `sofaprobe` treefmt exclude; `.gitignore` keeps only `/coverage.*`.
      Original task: delete the February
      artefacts `sofaprobe` (Linux ELF), `coverage.html`, `coverage.out`, and
      `.trunk/` (local-only, pins go@1.21 which cannot build a go 1.25 module).
      Make `just test-coverage` write to `bin/` or a temp dir, not the root;
      drop the `sofaprobe` exclude from `treefmt.toml`; narrow `.gitignore`'s
      global `*.html`/`*.out` to `/coverage.*`.

---

## Phase 1 — Written files must open in libmysofa (release blocker)

libmysofa is the most widely deployed SOFA reader (ffmpeg `sofalizer`, Steam
Audio, many plugins). Two **independent** causes, both must be fixed. Finish
with a CI gate so it cannot regress.

### P1.1 — go-hdf5: write new-style groups (in `../go-hdf`)

Root cause: go-hdf5 writes a **superblock v2** whose root object header
(OHDR v2) carries a **Symbol Table message (0x11)** — an old-style group
(`group_write.go:248`, `createGroupStructures`). libmysofa only understands
v2 object headers with **Link Info (0x02) / Group Info (0x0A) / Link (0x06)**
messages, and only superblock v2/v3 (forcing
`WithSuperblockVersion(V0)` fails with "cannot read signature"). netCDF-C
(HDF5 ≥1.8, `H5F_LIBVER_LATEST`-style) writes exactly those new-style groups;
SOFA roots have ~20–30 links, so netCDF-C typically ends up with **dense link
storage** (fractal heap + v2 B-tree name index) at the root.

- [ ] **P1.1a. Emit new-style root group for superblock v2.** Building blocks
      exist: `internal/core/link_message.go`, `internal/core/linkinfo.go`,
      `internal/writer/densegroup_writer.go`, `FileWriter.CreateDenseGroup`
      (`group_write.go:770`). Make `CreateForWrite`/`CreateForWriteTo` with
      superblock v2 build the root as Link Info + Group Info + compact Link
      messages (≤8 links) and switch to dense storage above the threshold
      (HDF5 default `max_compact = 8`). Keep the symbol-table path for
      superblock v0 only.
- [ ] **P1.1b. Track attribute creation order** (Attribute Info message
      flags, `attribute_write.go`) so `ncdump` lists globals in write order
      (`Conventions` first). Cosmetic but cheap once P1.1a touches the OHDR.
- [ ] **P1.1c. Tests in go-hdf5:** read back with h5py (`c_compat_h5py_test.go`
      pattern) and with `h5dump`; assert the root OHDR contains no 0x11
      message; add a libmysofa-load test if the harness is available (skip
      otherwise).
- [ ] **P1.1d. Include the P3.1a dataspace fix**, release go-hdf5 **v0.18.0**,
      bump go-sofa's `go.mod`.

### P1.2 — go-sofa: correct position variable dimensions

`ReceiverPosition` is written `(R,C)` and `EmitterPosition` `(E,C)`
(`sofa.go:829-841` → `writePositionDataset`, `sofa_netcdf.go:220`, row dim
from `rowDim`, `sofa_netcdf.go:212`). Every convention allows only
`rCI`/`rCM` and `eCI`/`eCM`; SOFA API, SOFA Toolbox and sofar write
`(R,C,I)` / `(E,C,I)`. Verified: KEMAR `ReceiverPosition(R, C, I)` becomes
`(R, C)` after a resave. libmysofa's `mysofa_check` rejects this
(`MYSOFA_RECEIVERS_WITH_RCI_SUPPORTED`,
`MYSOFA_ONLY_EMITTER_WITH_ECI_SUPPORTED`) even once P1.1 is fixed.

- [ ] **P1.2a.** Write Receiver/Emitter shared positions as `[X, C, I]`
      (`[I, C, I]` when broadcast, i.e. one row). The per-measurement variants
      (`ReceiverPositionsM`, `EmitterPositionsM`) are already `[X, C, M]` —
      check `writePositionDatasetPerM` for the same axis order.
- [ ] **P1.2b.** Read side: confirm `(R,C)`-shaped files written by go-sofa
      ≤0.2.0 still open (keep accepting both; add a test fixture written the
      old way via a small helper, not a binary blob).
- [ ] **P1.2c.** Update `TestRoundTrip*` / interop checks to assert
      `ReceiverPosition` has dims `("R","C","I")` via h5py/netCDF4
      (`scripts/interop_check.py`).

### P1.3 — Regression gate

- [ ] **P1.3a.** Add libmysofa to `just interop` and `test-interop.yaml`:
      build libmysofa at a pinned commit (cache the build), run the harness
      above over every file `internal/interop/gen` writes plus a resave of
      MIT_KEMAR and the `testdata/sofar/*` fixtures; fail on any non-zero
      load/check code. Only convention/DataType combinations libmysofa supports
      (SimpleFreeFieldHRIR FIR) must `check 0`; for others require
      `load` success only.
- [ ] **P1.3b.** Document the libmysofa result in README "Interoperability".

---

## Phase 2 — `Save` must write conformant files

README claims "All required AES69 fields … are validated". Today a
SimpleFreeFieldHRIR with no Listener/Receiver/Emitter positions saves fine,
a `SourcePosition` with `Type` but no `Units` is written without `Units`
(`positionAttributes` drops empties, `sofa_netcdf.go:268`), and
`GeneralTF-E` with DataType `FIR` saves. Fix validation _and_ defaults, so
simple use stays simple.

**Source of truth:** the per-convention CSV tables shipped with SOFAtoolbox
(`SOFAtoolbox/conventions/*.csv`, columns: name, default, flags `m`/`r`/`o`,
dimensions, type, comment). Transcribe the needed rows by hand into
`sofa_conventions.go`; do not add a code generator.

### P2.1 — Mandatory variables and attributes

- [ ] **P2.1a. Positions are mandatory.** `validate` (`sofa.go:994-1009`)
      allows length 0 for all four positions. Require ≥1 row for
      `ListenerPosition`, `ReceiverPosition`, `SourcePosition`,
      `EmitterPosition`; **default** `EmitterPosition` to `[0 0 0]` cartesian
      and `ListenerPosition` to `[0 0 0]` cartesian when empty, as the
      conventions do (same pattern as `listenerOrientation()` for
      ListenerView/Up). Receiver and Source have no sensible default → error.
- [ ] **P2.1b. Type/Units are mandatory on every position.** Default Units
      from Type when empty: cartesian → `metre`, spherical →
      `degree, degree, metre`. Reject an empty Type. Remove the "empty units
      are omitted" branch in `positionAttributes`.
- [ ] **P2.1c. Fix `UnitsCartesianMetres`** (`sofa.go:60`): the conventions'
      value is `metre`, not `metre, metre, metre`. Keep accepting both on read.
      Note it in CHANGELOG (value change of an exported constant).
- [ ] **P2.1d. Validate Units vocabulary** (low cost): accept `metre`/`meter`/
      `metres`/`meters` and `degree`/`degrees` in the comma-separated
      positions; reject garbage such as `furlong, parsec, cubit`. Put the
      accepted aliases in one table with a comment citing sofar/SOFAtoolbox.
- [ ] **P2.1e. Per-convention mandatory globals.** Add a `mandatoryGlobals`
      list to each `conventionRegistry` entry (e.g. SimpleFreeFieldHRIR:
      `DatabaseName`, `ListenerShortName` — verify against the CSV). Default
      where the CSV has a default; otherwise return a `*ValidationError`.
- [ ] **P2.1f. Fix the README "create from scratch" example** (README.md:231-265):
      set the four `…PositionType` fields and use a spherical source
      `{0, 0, 1}`. Moved into an `Example` test in P6.2 so it cannot rot.

### P2.2 — Convention rules

- [ ] **P2.2a. Rules for every official convention.** `layoutRules`
      currently cover only a few. Add DataType + required-variable rules for
      GeneralFIR, GeneralTF, GeneralTF-E, SimpleFreeFieldHRIR/HRTF/HRSOS,
      FreeFieldHRTF, SimpleHeadphoneIR, SingleRoomSRIR, SingleRoomDRIR,
      FreeFieldDirectivityTF. Minimum per rule: allowed DataType(s), required
      variables, `RoomType` default. Legacy name `SimpleFreeFieldSOS` maps to
      SimpleFreeFieldHRSOS for validation.
- [ ] **P2.2b. Per-convention `RoomType` default.** `defaultRoomType =
"free field"` (`sofa.go:898`) is used for every convention. Take it from
      the registry: SingleRoomSRIR → `shoebox` (then `RoomCornerA/B` become
      mandatory per the CSV), SingleRoomDRIR → `reverberant` (verify).
- [ ] **P2.2c. Remove or implement dead registry entries.** MultiSpeakerBRIR
      and SingleRoomMIMOSRIR have rules (`sofa_conventions.go:26,28`) but their
      DataType (FIR-E / FIRE) is rejected on read (`sofa_accessors.go:23`), so
      the rules can never apply. Decision: **drop them now** and list FIR-E in
      README "Limitations"; FIR-E support is P7.3 _(optional)_.
- [ ] **P2.2d. Version gating.** Reject (or warn via `ConventionWarnings`) SOFA
      2.x-only features in a file declaring `Version < 2.0`: FreeFieldHRTF,
      TF-E, `EmitterPosition:Type = "spherical harmonics"`. Accept only known
      `SOFAConventionsVersion` values per convention; unknown → warning, not
      error (be liberal for custom conventions).

### P2.3 — Provenance on save

- [ ] **P2.3a.** Always stamp `DateModified = now`, `APIName = "go-sofa"`,
      `APIVersion = moduleVersion()` on `Save`/`WriteTo` (`sofa.go:927-929`
      currently keep the originals). Keep `DateCreated`. Append a line to
      `History` recording the previous API (`"resaved by go-sofa X from
<APIName> <APIVersion>"`) when it differed. This is what SOFA Toolbox's
      `SOFAsave` does, and libmysofa keys a receiver-position workaround on
      APIName/APIVersion. Document on `Save`; the `File` itself stays
      unchanged (only the written file).
- [ ] **P2.3b.** Stop writing `Type`/`Units` attributes on `ListenerUp`
      (`sofa.go:850-866`): the spec defines them on `ListenerView` only. Keep
      reading them.

---

## Phase 3 — Robustness for untrusted input

### P3.1 — Crashes

- [ ] **P3.1a. go-hdf5 dataspace panic.**
      `internal/core/dataspace.go:30` checks `len(data) < 2`, then reads
      `data[2]` (flags) → "index out of range [2] with length 2" on a 1–4 byte
      mutation of a Save-produced seed (found by fuzzing in 9.6 s). Check the
      real header size for the version (v1: 8 bytes incl. reserved, v2: 4)
      before indexing, and bounds-check the per-dimension reads that follow.
      Add the crasher to go-hdf5's fuzz corpus and to
      `testdata/fuzz/FuzzOpen/` here. Ships with P1.1d.
- [ ] **P3.1b. Grep go-hdf5 `internal/core/*.go` for the same pattern**
      (`len(data) < k` followed by `data[k]` or larger fixed offsets) and fix
      them in the same PR.
- [ ] **P3.1c. Lazy nil-pointer.** `readMeasurement` (`sofa_stream.go`) does
      `v := l.vars[name]` without `ok`. Changing the exported `f.DataType` on a
      lazy File (TF → FIR) makes `v.shape` panic. Check `ok`, return an error.

### P3.2 — Memory budget

A 5.5 KB crafted file with 16 chunked, never-written "extra" variables of
2^25 float64 each opens successfully and pins **4 GiB** in `File.Variables`.
`maxDataElements = 1<<30` (`sofa.go:366`) is per dataset; go-hdf5's own cap is
per read (max(256 MiB, fileSize × ratio)). There is no budget across one
`Open`.

- [ ] **P3.2a.** Add one element budget per `Open`/`OpenReader`, shared by
      audio, positions and extras: default
      `max(64 Mi elements, 64 × fileSize/8)` (tune on the largest real file,
      Kayser2009 ~179 MB data); exceeding it returns a wrapped
      `ErrTooLarge` (new sentinel). Correct the "8 GiB" comment at
      `sofa.go:362`.
- [ ] **P3.2b.** Char variables: `ReadStrings` builds a `[]string` of 1-byte
      strings (~18× amplification; a 3.4 KB file → 4.6 GiB transient). Read
      char arrays as raw bytes and split rows in go-sofa (or add a byte-level
      read to go-hdf5 if none exists).
- [ ] **P3.2c.** _(optional)_ `OpenOptions{MaxElements int; SkipExtras bool}`
      if P7.1 introduces an options type anyway — do not add a new API only
      for this.

### P3.3 — Silent wrong data

- [ ] **P3.3a. TF-E transposition for unlabeled files.** When a file has no
      dimension labels (no `REFERENCE_LIST`, e.g. written by h5py) and E == N,
      `resolveLayout` tries `layoutMREN` (old go-sofa order) before
      `layoutMRNE` (AES69) at `sofa.go:657,678` and `sofa_stream.go:102`.
      Verified: an AES69-ordered `[1,1,2,2]` file returned
      `TFRealE[0][0][1] = [10 11]` instead of `[1 11]`. Put `layoutMRNE`
      first everywhere; add a crafted-file test for E == N unlabeled.
- [ ] **P3.3b. Room scalars.** `readRoomScalars` (`sofa_srir.go:~95`) ignores
      the `ds.Read()` error, takes `data[0]` of any shape and accepts NaN/Inf
      (which `Save` then rejects → Open→Save breaks). Return read errors, check
      shape `[I]` via `resolveLayout`, and route non-finite values to
      `Dropped`. `parseRoomAttribute` (`sofa_srir.go:72`) turns an unparsable
      global into 0 silently — record it in `Dropped` instead.
- [ ] **P3.3c. `AmbisonicsOrder` heuristic** (`sofa_srir.go:44`) returns
      `(1, true)` for the sofar SingleRoomSRIR_1.0 fixture, which has R=4 raw
      capsules with `ReceiverPosition:Type = "spherical"`. Gate on
      `ReceiverPositionType == "spherical harmonics"`; keep the square-R check
      only as a consistency check (warning if it fails). Expose the
      AES69-2022 `Data.IR:ChannelOrdering` / `:Normalization` attributes as
      plain strings if present (read via `Attributes`, no new SH math).
- [ ] **P3.3d. NaN in accessors.** `IRPeakdB` (`sofa_accessors.go:234`)
      returns -Inf for an all-NaN IR; `SamplingRateScalar` reports
      `[NaN, NaN]` as `ErrVaryingSamplingRate`. Return an error for non-finite
      input instead. Covered generally by P7.1a (`Validate()` usable after
      `Open`).

### P3.4 — Save edge cases (low)

- [ ] **P3.4a.** Remove the temp file on panic too (`sofa_io.go:77-81`): use
      a `committed bool` set after rename and `defer` cleanup when false.
- [ ] **P3.4b.** Document that `Save` replaces a symlink with a regular file
      (it `os.Stat`s and renames over the link). Do not change the behaviour.

---

## Phase 4 — Performance

### P4.1 — Lazy reads (high)

go-hdf5 keeps at most 8 decompressed chunks / 16 MiB per dataset;
`prepareLazyAudio` (`sofa_stream.go:95-150`) never calls
`ds.SetChunkCacheSize` (exists: `go-hdf5 dataset_read_cache.go:70`).
Kayser2009 (`Data.IR` [584,8,4800], chunks [146,1,1200]) touches 32 chunks
per measurement → every `ReadMeasurement` evicts and re-inflates all of them.

| Kayser2009                     | time   | allocated |
| ------------------------------ | ------ | --------- |
| eager `Open`                   | 0.64 s | 876 MB    |
| `OpenLazy` + full range (now)  | 81.4 s | 75.7 GB   |
| same, cache 32 chunks / 64 MiB | 0.70 s | 884 MB    |

- [ ] **P4.1a.** In `prepareLazyAudio`, after `resolveLayout`, read
      `ds.ChunkShape()`; for chunked datasets compute
      `k = ∏ ceil(shape[i]/chunk[i])` over all axes except M (the chunks one
      measurement row touches), and call
      `ds.SetChunkCacheSize(max(8, k), clamp(k × chunkBytes, 16 MiB, 256 MiB))`.
      Contiguous datasets: nothing to do.
- [ ] **P4.1b.** Extend `BenchmarkStreamChunked` with a file chunked along R
      and N (Kayser2009 when present, plus a synthetic go-hdf5-written file
      with chunks `[M/4, 1, N/4]` so CI covers it). Add a test asserting one
      full `RangeMeasurements` pass allocates < 2× the eager allocation.
- [ ] **P4.1c.** Document on `OpenLazy`: typical HRTF files are chunked as one
      chunk per receiver across all M (CIPIC `[1250,1,200]`), so the first
      `ReadMeasurement` inflates the whole receiver — lazy saves memory only
      for files chunked along M or stored contiguously.

### P4.2 — Copies (medium)

- [ ] **P4.2a. Write path.** `Save` goes nested `[][][]` → `flattenIR`
      (`sofa.go:1364`) → byte encoding in go-hdf5 → file; `WriteTo` buffers the
      whole HDF5 in memory too (Mesh2HRTF: 48 MB allocated for 9.5 MB data).
      After `Open`, the nested slices already alias one contiguous backing
      array (`reshapeIR`). Keep that backing slice in an unexported `File`
      field and pass it straight to `ds.Write` when the nested slices still
      alias it contiguously (cheap check: `&f.ImpulseResponses[0][0][0]` and
      lengths); otherwise flatten as today.
- [ ] **P4.2b. TF-E transpose in one pass.** `swapLastAxes` allocates a full
      extra copy on read (`sofa.go:670,691`) and `flatten4D` + `swapLastAxes`
      on write (`sofa.go:1346`). Transpose while reshaping/flattening into the
      single destination buffer.
- [ ] **P4.2c.** _(go-hdf5, optional)_ decode chunks directly into the output
      `[]float64` instead of materialising the full `[]byte` then converting
      (`ReadDatasetFloat64` → `convertToFloat64`). Eager read peak is 3–4× the
      data size today (Kayser 524 MB peak for 179 MB data).

### P4.3 — File size on save (medium, user-visible)

Writes are contiguous and uncompressed: `tester.sofa` 91 KB → 5.2 MB (57×),
MIT_KEMAR 1.17 MB → 5.85 MB.

- [ ] **P4.3a.** Change to `Save(path string, opts ...SaveOption)` and
      `WriteTo` keeps its `io.WriterTo` signature (add
      `WriteToWithOptions(w, opts...)` only if needed). First option:
      `WithDeflate(level int)` using go-hdf5's `WithChunkDims` +
      `WithGZIPCompression` (+ `WithShuffle`), one measurement row per chunk
      (`[1, R, N]`, capped). Variadic keeps `Save` source-compatible. Verify
      libmysofa and netCDF-C read the compressed output (P1.3 harness). Decide
      after measuring whether deflate level 4 becomes the default.

### P4.4 — Small items (low, optional)

- [ ] **P4.4a.** `readStringAttribute` (`sofa_spatial.go:135`) re-reads the
      full attribute list twice per position dataset — read once, look up both.
- [ ] **P4.4b.** One mutex per `lazyVariable` instead of one for all
      (`sofa_stream.go:170,186`), so TF Real/Imag reads can overlap.
- [ ] **P4.4c.** `ReadMeasurementInto(m int, dst [][]float64)` for
      allocation-free hot loops (70 µs / 28 allocs for a 3.2 KB CIPIC read
      today). Only if a real user asks.

---

## Phase 5 — Tests and CI

### P5.1 — Make CI test real files

17 tests skip in CI because `optionalTestdata` (`testhelpers_test.go:13`)
files come from sofacoustics.org (403 to GitHub runners) or exist only
locally (`OfficeII.sofa`, `SingleRoomSRIR_1.1.sofa` are not even in
`scripts/fetch-testdata.sh`). Every toolbox-produced TF-E/SOS/SH/SRIR/BRIR
case is therefore never run in CI.

- [ ] **P5.1a. Pin hashes now** for the 5 optional files in
      `scripts/fetch-testdata.sh:40-44` and `testdata/PROVENANCE.md` (you have
      them locally; `shasum -a 256`).
- [ ] **P5.1b. Mirror redistributable files** as assets of a `testdata-v1`
      GitHub release of this repo; point the manifest at them with the pinned
      hashes, and remove them from `optionalTestdata`. Check each licence first;
      files without a redistribution licence stay optional.
- [ ] **P5.1c. OfficeII / SingleRoomSRIR_1.1:** add to the manifest with
      source URL + hash, or replace the 6 dependent tests with sofar-generated
      equivalents in `testdata/sofar/` (via `scripts/make_sofar_fixtures.py`).
- [ ] **P5.1d.** Add a CI step that fails if any test in the list of
      expected-to-run tests skipped (`go test -json | jq` over `"Action":"skip"`),
      so a vanishing fixture cannot turn CI vacuously green.

### P5.2 — Fuzzing that actually fuzzes

`go test -fuzz FuzzOpen -fuzztime 60s` ran 22–51 executions in 180 s: every
interesting 14 KB seed is minimised byte-by-byte, each attempt doing
`t.TempDir()` + file write + `Open`. With `-fuzzminimizetime 0`: ~5,000 exec/s.

- [ ] **P5.2a.** Rewrite `FuzzOpen` (`fuzz_test.go:16-30`) on
      `OpenReader(bytes.NewReader(data), int64(len(data)))` — no temp files.
- [ ] **P5.2b.** Seed with `testdata/tester.sofa` (small, chunked+deflate,
      toolbox-written) and `testdata/sofar/*.sofa`, not only `Save` output.
- [ ] **P5.2c.** Add `FuzzOpenLazy` (OpenLazyReader + ReadMeasurement +
      RangeMeasurements) and `FuzzRoundTrip` (Open → WriteTo → OpenReader
      must succeed and compare equal).
- [ ] **P5.2d.** `just fuzz` recipe with `-fuzzminimizetime=0 -fuzztime=2m`
      per target and `GOMEMLIMIT=2GiB`; run it in the weekly `largefiles.yml`.
      Commit crashers to `testdata/fuzz/`.

### P5.3 — Coverage gaps (add table rows, no new frameworks)

Use the existing crafted-file helper (`writeCraftedSpec`) and
`TestSaveValidation`:

- [ ] Legacy flat `Data.Delay` read (`sofa.go:582-587`).
- [ ] Read-side SOS: size mismatch and `N % 6 != 0` (`sofa.go:714-719`).
- [ ] TF-E `Data.Imag` missing / wrong size (`sofa.go:676-689`).
- [ ] `validate`: E ≤ 0, N ≤ 0, Receiver/Source/Emitter length mismatches
      (`sofa.go:965-1001`).
- [ ] SOS `SamplingRate`/`Delay` length checks (`sofa.go:1192-1199`).
- [ ] `Save` sync/chmod/rename failures (`sofa_io.go:90-102`) — inject via a
      read-only dir / existing directory at the target path.
- [ ] `prepareLazyAudio` error paths (`sofa_stream.go:105-146`).
- [ ] Tests that only assert "no error" — reopen and compare instead:
      `TestBRIRValidFileSaves` (`sofa_brir_test.go:86`),
      `TestSRIRWarningsDoNotBlockSave`, `TestCloseNil`.
- [ ] `TestOpenErrors`: assert `errors.Is(err, fs.ErrNotExist)` instead of an
      "open HDF5" substring; drop the hand-rolled `strings.Contains`
      (`sofa_test.go:375-387`).

### P5.4 — Test hygiene

- [ ] **P5.4a.** Delete `hdf5_validation_test.go` (tests go-hdf5, stale
      "Phase 1" header, mostly `t.Logf`); move anything still useful to
      go-hdf5.
- [ ] **P5.4b.** One valid-file builder per DataType in `testhelpers_test.go`
      (`newFIRFile(t, M, R, N)` etc.) replacing the ~12 parallel builders
      (`minimalFIRFile`, `robust*File`, `saveFixtures`, `streamFIRFile`,
      `brirTestFile`, `srirTestFile`, `coordinateTestFile`, …). Do this
      opportunistically when touching those tests, not as a big-bang PR.
- [ ] **P5.4c.** Merge `sofa_write_test.go`, `sofa_save_test.go` and
      `sofa_io_test.go` Save/WriteTo tests into `sofa_io_test.go`.
- [ ] **P5.4d.** `TestOpenLazyDoesNotAllocateAudio` (`sofa_stream_test.go:480`)
      and `TestOpenReleasesHandle` measure process-wide state; add a comment
      that they must never be `t.Parallel()`, or switch to
      `testing.AllocsPerRun` / per-file-handle checks.

### P5.5 — CI workflows

- [ ] **P5.5a. Double runs.** `test.yaml` and `test-interop.yaml` trigger on
      `[push, pull_request]` and the concurrency group uses `github.ref`, so
      every same-repo PR runs twice. Use
      `on: { push: { branches: [main] }, pull_request: {} }`.
- [ ] **P5.5b. OS matrix** for `test-unit.yaml`:
      `[ubuntu-latest, macos-latest, windows-latest]`, Go from `go.mod`.
      `Save` has platform code (`sofa_fsync.go:14`, rename-over, `Chmod`).
- [ ] **P5.5c. Pin interop deps**: `scripts/requirements-interop.txt`
      (h5py, netCDF4, numpy versions) instead of `pip install --upgrade`.
- [ ] **P5.5d.** Run the root suite once: `just test` writes a coverage
      profile; `coverage-check` reads it instead of re-running (currently
      without `-race`). Gate `cmd/` coverage too.
- [ ] **P5.5e.** `test-format.yaml`: `allow-missing-formatter: false`.
- [ ] **P5.5f.** Pin third-party actions by SHA (at least
      `isbecker/treefmt-action@v2`, which gets `GITHUB_TOKEN`).
- [ ] **P5.5g.** `largefiles.yml`: fix the "run it by hand" comment
      (it has a cron); add failure notification (open an issue via
      `gh issue create` on failure, or rely on GitHub's scheduled-workflow
      email — decide and document).
- [ ] **P5.5h.** _(optional)_ Weekly Octave + SOFAtoolbox job running
      `scripts/matlab/roundtrip.m` (`apt install octave octave-netcdf`, clone
      SOFAtoolbox at a pinned tag).

### P5.6 — Lint / format config

- [ ] **P5.6a.** One formatter set: `.golangci.toml` enables
      `gofmt`/`goimports`, `treefmt.toml` uses `gofumpt`/`gci`. Pick
      gofumpt + gci in golangci, have treefmt call golangci's formatters (or
      vice versa) — one config.
- [ ] **P5.6b.** Remove dead config: `tagliatelle`/`varnamelen` settings
      (not enabled), the G601 exclusion (obsolete since Go 1.22).
- [ ] **P5.6c.** Enable `nolintlint` (flags stale directives at
      `sofa_io.go:167`, `sofa_robustness_test.go:173`), `errorlint`,
      `thelper`, `usetesting`. Re-enable `revive` for `cmd/`. Lower
      `gocyclo` to ~20 and fix or annotate the offenders.
- [ ] **P5.6d.** justfile: drop `-v` from `just test`; align timeouts
      (test/coverage 300 s, lint 5 m like CI); make `check` include
      `coverage-check`.

---

## Phase 6 — Docs and CLIs

### P6.1 — README: honest and short

The README is a 759-line manual whose copied API reference already drifts
from godoc.

- [ ] **P6.1a. Remove overstated claims.** "Full AES69 support" (line 10) →
      list what is supported; "AES69-2015" (lines 3, 587, 756) → "AES69-2020 /
      SOFA 2.x (AES69-2022 conventions where noted)"; "Validated against
      reference SOFA files from sofaconventions.org" (line 15) → name the
      actual sources (libmysofa corpus, Mesh2HRTF, sofar-generated).
- [ ] **P6.1b. Verify** the "AES69-2022 introduced `SimpleFreeFieldHRSH`"
      statement (line 617) against the standard; the reviewer could not find
      such a convention. If it does not exist, fix the text and the "SH"
      substring heuristic in `sofa_sh.go` that relies on the name.
- [ ] **P6.1c. Add "Limitations"**: FIR-E/FIRE not supported; writes are
      uncompressed unless `WithDeflate` (P4.3); all sample data is `float64`
      (float32 files double in memory); extra integer variables round-trip as
      float64; `NC_STRING` variables are listed in `Dropped`; Directivity only
      partial; pre-1.0 API.
- [ ] **P6.1d. Cut the "API Reference" section** (README.md:417-583) and link
      pkg.go.dev. Known drift today: `Delay` described "per measurement" (godoc:
      1/M/R/M×R); `TFRealE`, `SOSCoefficients`, `ReceiverPositionsM` missing;
      `DelayDimensions` listed as a field (it's a method).
- [ ] **P6.1e. Remove internal notes / run logs**: README.md:736-739 (run log
      "on the go-hdf5#5 writer"), README.md:643 (`../PasSofa` link, broken on
      GitHub, "reference implementation"); PROVENANCE.md:35 "(R3b)",
      :119 "(Phase B)", :98 "the agent sandbox"; CHANGELOG.md:195 "Phase R
      review fixes".
- [ ] **P6.1f.** README "Development" builds only two of three CLIs
      (lines 659-660) — fix or drop after P6.3e.

### P6.2 — Godoc

- [ ] **P6.2a. Package doc** (`sofa.go:1` → move to `doc.go`): 20-line
      overview with Open / OpenLazy / Save, error handling with
      `errors.Is`/`*ValidationError`, and the supported-DataType table.
- [ ] **P6.2b. `example_test.go`**: `ExampleOpen`, `ExampleOpenLazy`
      (+`RangeMeasurements`), `ExampleFile_Save` (the README "create from
      scratch" example, now correct — P2.1f), `ExampleValidationError`. Use
      `// Output:` where deterministic (e.g. printing M, R, N of
      `testdata/sofar/…`, which is committed and in CI).

### P6.3 — CLIs

- [ ] **P6.3a. Load metadata only.** `sofainfo` (`main.go:68`) and
      `sofa2json` (`main.go:107`, when no `--include-*`) call `sofa.Open` and
      read all audio. Use `OpenLazy`; stream with `RangeMeasurements` where
      data is emitted. Makes README's "streamed" claim (line 402) true.
- [ ] **P6.3b. `sofa2json` UX**: `-o -` writes to stdout (so `| jq` works);
      create output files `0644` not `0600` (`main.go:123,125`); include
      `Attributes`/`Variables` or fix README.md:392 which says the keys are
      the `sofa.File` fields.
- [ ] **P6.3c. Shared behaviour for all CLIs**: print usage and exit 2 on no
      arguments (today `sofainfo`/`sofa2json` glob `*.sofa` in CWD and
      `sofa2json` silently writes files); `-version` flag from
      `debug.ReadBuildInfo`; `sofainfo` prints all sampling rates when they
      vary (`main.go:122` prints `[0]` only).
- [ ] **P6.3d. Deduplicate** the 20-entry global-attribute list
      (`sofainfo/main.go:87-108`, `sofa2json/main.go:172-193`): export
      `(*File).GlobalAttributes() []Attribute` (ordered, mapped fields
      included) and use it in both.
- [ ] **P6.3e. `sofaprobe`** is a go-hdf5 debugging aid (its own doc says
      so). Move to `internal/cmd/sofaprobe` (still `go run`-able from the repo,
      not a public install target); fix its stale "(none — may use dense
      attribute storage)" message (`main.go:143`). Note in CHANGELOG.

### P6.4 — Other docs

- [ ] **P6.4a. CHANGELOG**: fix contradiction CHANGELOG.md:24 vs :101-102
      (sofaprobe "three values" vs "two rows"); list indentation at :35; add
      the v0.1.0 date and compare links.
- [ ] **P6.4b. design-notes.md**: :99-104 claims `SimpleFreeFieldSOS_1.0.sofa`
      is "not available" (it is local and opens, M=16022) — re-test and update;
      :138 benchmark baseline on go-hdf5 `34395b7` → re-run on the shipped
      version (after P4.1).
- [ ] **P6.4c.** GitHub repo description + topics (`sofa`, `aes69`, `hrtf`,
      `spatial-audio`, `netcdf`, `hdf5`, `go`).

---

## Phase 7 — API polish while still pre-1.0

Do these before tagging v1.0; each is a small, contained change.

### P7.1 — Consistency of `File`

- [ ] **P7.1a. Export `Validate() error`** (thin wrapper over `validate()`,
      `sofa.go:952`). Callers building a `File` by hand get feedback before
      `Save`, and it can be called after `Open` to reject non-finite data
      (P3.3d). Document that `Save`/`WriteTo` call it.
- [ ] **P7.1b.** Move all sentinels into `sofa_errors.go`:
      `ErrUnsupportedDataType`, `ErrNoSamplingRate`, `ErrVaryingSamplingRate`,
      `ErrIndexOutOfRange` (`sofa_accessors.go:13,34,39,167`), `ErrNotLoaded`
      (`sofa_stream.go:18`), plus the new `ErrTooLarge` (P3.2a).

### P7.2 — Open API shape (decide once)

Today: `Open`, `OpenLazy`, `OpenReader`, `OpenLazyReader` — two axes
(path vs. `io.ReaderAt`; eager vs. lazy) as a 2×2 name matrix. A third axis
(memory budget, skip extras) would double it again.

- [ ] **P7.2a. Decide:** keep `Open(path)` and `OpenReader(r, size)` as the
      simple entry points, and add `OpenOption`s (`Lazy()`,
      `MaxElements(n)`, `SkipExtras()`) as variadic parameters on both.
      `OpenLazy`/`OpenLazyReader` become deprecated one-liners. Record the
      decision in design-notes.md. If rejected, write down why.
- [ ] **P7.2b.** _(optional)_ context-aware variants — **not now**; SOFA
      files are local and bounded. Revisit only on request.

### P7.3 — FIR-E _(optional, only if a user needs it)_

- [ ] Implement DataType `FIR-E` (`Data.IR [M,R,E,N]`, `Delay [I|M,R,E]`) to
      unlock GeneralFIR-E, FreeFieldHRIR, SingleRoomMIMOSRIR and
      MultiSpeakerBRIR; re-add their convention rules (dropped in P2.2c).
      Fixtures: sofar can write MultiSpeakerBRIR 0.3 (DataType FIRE).

---

## Deliberately not doing

Recorded so the next review doesn't re-raise them:

- **Typed `DataType`/convention strings** — custom conventions are legal per
  AES69; plain `string` constants are fine.
- **Merging `ConventionWarnings()` and `SHWarnings()`** — only if a third
  warnings method appears.
- **float32 data path** — document (P6.1c) instead.
- **k-d tree / spatial index** — a linear scan over 16k positions is 8 µs.
  A `NearestMeasurement(az, el)` helper is a feature request, not a fix;
  add it only when asked.
- **Preserving integer types of extra variables** — documented float64
  round-trip is acceptable (P6.1c).
- **`NC_STRING` variables** — SOFA mandates char arrays; keep listing them in
  `Dropped`.

## Suggested order

1. Phase 0 (hours).
2. P3.1a + P1.1 in go-hdf5 → release v0.18.0.
3. P1.2, P1.3 → **v0.3.0** ("files now open in libmysofa").
4. P4.1a (one function) + P3.3a (one-line order swap) — can go into v0.3.0.
5. Phase 2 → v0.4.0 (validation becomes stricter: breaking for invalid
   input; call it out in CHANGELOG).
6. Phases 3.2–3.4, 5, 6 in parallel PRs.
7. Phase 7 decisions, then v1.0.
