# Test data provenance

The `*.sofa` files directly in this directory are third-party reference files. They are
**not committed** (most carry no licence, or a licence that does not clearly
permit redistribution in this repository) and are fetched on demand:

```sh
just fetch-testdata      # or: scripts/fetch-testdata.sh
```

`scripts/fetch-testdata.sh` is the authoritative manifest (URL + SHA-256).
Tests that need a required file fail with a pointer to that command when it is
absent; tests for the optional and local-only files below skip instead
(`optionalTestdata` in `testhelpers_test.go`).
Keep this table and the script in sync.

"Producer" is taken from the file's own `APIName` / `APIVersion` global
attributes (and `_NCProperties` where present). "Licence" is quoted as stated by
the source; it is not legal advice.

## libmysofa test corpus

Pinned to libmysofa commit
[`648eed0`](https://github.com/hoene/libmysofa/tree/648eed03472e6720a1ea45d1a1f86c4efb569ff9)
(2026-09-08). libmysofa itself is BSD-3-Clause; per-file licences are in
`tests/LICENSE.*` of that repository.

| File                                | Source URL                                                                                                                         | Producer                                                                     | Licence as stated by the source                                                                                                                                                   | Size (bytes) | SHA-256                                                            |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -----------: | ------------------------------------------------------------------ |
| `tester.sofa`                       | https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9/tests/tester.sofa                       | ARI SOFA API for Matlab/Octave 0.4.2 (SimpleFreeFieldHRIR 0.4)               | No dedicated licence file; file attribute `License`: "No license provided, ask the author for permission" (Organization: Acoustics Research Institute). Distributed in libmysofa. |        91409 | `38abbb518e4e43ec2efbd4765e763e764776f18ae0e0181f3684bff644d9c58b` |
| `MIT_KEMAR_normal_pinna.sofa`       | https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9/share/MIT_KEMAR_normal_pinna.sofa       | ARI SOFA API for Matlab/Octave 1.1.1 via netCDF-C 4.6.1 / HDF5 1.8.12        | `tests/LICENSE.MIT_KEMAR_pinnae`: "Copyright 1994 by the MIT Media Laboratory. It is provided free with no restrictions on use, provided the authors are cited".                  |      1173158 | `2768ac841213a7ae11d1ea7fd0f25a69b39216102dc5dd913ea6ba0f0dc57e28` |
| `CIPIC_subject_003_hrir_final.sofa` | https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9/tests/CIPIC_subject_003_hrir_final.sofa | ARI SOFA API for Matlab/Octave 0.4.0 (SimpleFreeFieldHRIR 0.4)               | `tests/LICENSE.CIPIC_subject_003_hrir_final`: Copyright (c) 2001 The Regents of the University of California; permission to reproduce/use for any purpose, keeping the notice.    |      3568629 | `0d31149c9893a209642fd65ea64cfe81f3ffd5c6cea74ce374936f2edd5628a1` |
| `Mesh2HRTF.sofa`                    | https://raw.githubusercontent.com/hoene/libmysofa/648eed03472e6720a1ea45d1a1f86c4efb569ff9/tests/Mesh2HRTF.sofa                    | sofar SOFA API for Python (pyfar.org) 0.3.1 via netCDF-C 4.8.1 / HDF5 1.10.7 | No dedicated licence file; file attribute `License`: "No license provided, ask the author for permission" (ApplicationName: Mesh2HRTF 1.0.0). Distributed in libmysofa.           |      8020794 | `e4ceee243e445e6bc873db41e61faec7f5ce75ecd9fe174310f1c8936f6b64aa` |

Coverage of producers (R3b): SOFA API for Matlab/Octave (`tester`, `MIT_KEMAR`,
`CIPIC`), Python/netCDF-C (`Mesh2HRTF`, written by sofar through netCDF4-python),
all taken from the libmysofa test set. `TestReadReferenceValues` pins sample
values and positions of these files as read by h5py 3.11 / HDF5 1.14.

## Mesh2HRTF test resources

Pinned to Mesh2HRTF commit
[`e45d043`](https://github.com/Any2HRTF/Mesh2HRTF/tree/e45d0436a6fbeca3db13828cbae23ca109225be3)
(2026-03-20). The repository is licensed EUPL-1.2 (`LICENSE.txt`); the file
itself carries no licence of its own.

| File                                           | Source URL                                                                                                                                                        | Producer                                                                                                  | Licence as stated by the source                                                                      | Size (bytes) | SHA-256                                                            |
| ---------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- | -----------: | ------------------------------------------------------------------ |
| `Mesh2HRTF_HRTF_FourPointHorPlane_r100cm.sofa` | https://raw.githubusercontent.com/Any2HRTF/Mesh2HRTF/e45d0436a6fbeca3db13828cbae23ca109225be3/tests/resources/SHTF/Output2HRTF/HRTF_FourPointHorPlane_r100cm.sofa | sofar SOFA API for Python (pyfar.org) 0.3.1 via netCDF-C 4.8.1 / HDF5 1.10.7 (ApplicationName: Mesh2HRTF) | Repository EUPL-1.2; file attribute `License`: "No license provided, ask the author for permission". |        50647 | `e209f2de064f1b82558644e06f5535033839062cf9e1102f53fc2b3e556a2fdf` |

SimpleFreeFieldHRTF 1.0, DataType TF, M=4 R=2 E=1 N=60 (100–6000 Hz). It
**replaces** the former `FreeFieldHRTF_2.0.sofa` in `TestReadRealTFFile`: that
file was expected to be a SimpleFreeFieldHRTF / TF file too (M=2354), only
reachable from sofacoustics.org (whose provenance was never confirmed). Same
convention and DataType, so it exercises the same read path;
`TestReadRealTFValues` additionally pins frequencies, Data.Real/Data.Imag
samples and positions as read by h5py.

## Generated with sofar (committed)

`testdata/sofar/` holds small synthetic files that **are committed**: they
stand in, in CI, for the sofacoustics.org example files below, which CI
cannot fetch. `scripts/make_sofar_fixtures.py` writes them with
[sofar](https://github.com/pyfar/sofar) 1.3.0 (netCDF4-python 1.7.3,
netCDF-C 4.9.3, HDF5 1.14.6), an implementation independent of go-sofa's
writer. The values are seeded pseudo-random numbers rounded to six decimals,
and the dates and application attributes are fixed, so the same library
versions write byte-identical files; other versions may change the bytes
(update the hashes here) but not the values pinned in
`sofa_sofar_fixtures_test.go`.

Licence: the content is generated by this repository's script and carries
no third-party data; it is MIT like the rest of go-sofa (file attribute
`License`: "MIT (synthetic test data generated for go-sofa)").

| File                            | Convention                | DataType | Dimensions                                   | Size (bytes) | SHA-256                                                            |
| ------------------------------- | ------------------------- | -------- | -------------------------------------------- | -----------: | ------------------------------------------------------------------ |
| `GeneralTF_2.0.sofa`            | GeneralTF 2.0             | TF       | M=4 R=4 E=1 N=8                              |        39570 | `b11dec559d377d6abfdae5a792be9f0cf066ccf16e29ff640a14885e304da443` |
| `GeneralTF-E_1.0.sofa`          | GeneralTF-E 1.0           | TF-E     | M=4 R=2 E=3 N=6                              |        40838 | `dc386bca0d1aeca08f61985ad3767d5805f683a8d420e060d1f5a749f124f5b8` |
| `FreeFieldHRTF_1.0.sofa`        | FreeFieldHRTF 1.0         | TF-E     | M=3 R=2 E=1 N=5, cartesian emitter           |        44907 | `c7bb41fab8a67323e4bc3e772de663c3e78f736ec53197330160207189acf372` |
| `FreeFieldHRTF_1.0_SH_L2.sofa`  | FreeFieldHRTF 1.0         | TF-E     | M=1 R=2 E=9 N=4, spherical harmonics order 2 |        47968 | `6990351e4b3fd793283e463f4089f80351f5a7b76d3c071e6c6511cd85a839e9` |
| `SimpleFreeFieldHRSOS_1.0.sofa` | SimpleFreeFieldHRSOS 1.0  | SOS      | M=6 R=2 E=1 N=12                             |        44438 | `202c7e1b6006a69be8f423064f5b5d84ff7876026e42f23892ae6796c4be33db` |
| `SingleRoomSRIR_1.0.sofa`       | SingleRoomSRIR 1.0        | FIR      | M=3 R=4 (Ambisonics order 1) E=1 N=64        |        93838 | `66361229cad67647a95cef8d744682492278b33458a32164929458882b2fcaf8` |
| `SingleRoomDRIR_0.3.sofa`       | SingleRoomDRIR 0.3 (BRIR) | FIR      | M=2 R=2 E=1 N=48                             |        52153 | `d826fd36bea65e4ac27812f005dc99da4ecb79fc00c4ba97d7a947e35a6c4dde` |

SingleRoomSRIR omits sofar's empty optional short-name and description
attributes, see `docs/design-notes.md` ("Fixtures"). Even so, go-sofa cannot
yet re-save it with all its variables and misses its `ReceiverUp` variable
(PLAN.md E6, E7); the tests pin both as known failures.

## sofacoustics.org (optional, not reachable from CI)

Hosted by the Acoustics Research Institute at https://sofacoustics.org/data/
(same server as `www.sofaconventions.org/data/`). The `examples/` files are the
example files linked from the per-convention pages of the SOFA conventions wiki;
the `sofatoolbox_test/` files are written by the SOFA Toolbox demos
(`demo_FreeFieldHRTF.m`).

**Unreachable:** sofacoustics.org answers HTTP 403 both to the agent sandbox and
to GitHub Actions runners, and no GitHub-hosted copy of these files was found
(checked: libmysofa, libsofa, SOFAtoolbox, sofar, pyfar, pysofaconventions,
python-sofa, Mesh2HRTF, spaudiopy, SUpDEq, sound_field_analysis-py,
libspatialaudio, IoSR MatlabToolbox, and the PyPI sdists of sofar, pyfar,
pysofaconventions, python-sofa, spaudiopy, sofa, sound-field-analysis). The
fetch script marks them `optional`: a failed download is a warning, and the
tests that need them skip. The same conventions are covered in CI by the
generated files in `testdata/sofar/` above. Hash, size, producer
and licence are unverified; pin them on the first successful fetch.

| File                            | Source URL                                                                  | Needed by                               | Notes                                                       |
| ------------------------------- | --------------------------------------------------------------------------- | --------------------------------------- | ----------------------------------------------------------- |
| `GeneralTF_2.0.sofa`            | https://sofacoustics.org/data/examples/GeneralTF_2.0.sofa                   | `TestReadRealTFFile/GeneralTF_2.0.sofa` | Tests expect GeneralTF / TF, M=4 R=4 E=1.                   |
| `GeneralTF-E_1.0.sofa`          | https://sofacoustics.org/data/examples/GeneralTF-E_1.0.sofa                 | `TestReadRealTFEAndSOS`                 | Tests expect GeneralTF-E / TF-E, M≥4 R=4800.                |
| `FreeFieldHRTF_1.0.sofa`        | https://sofacoustics.org/data/examples/FreeFieldHRTF_1.0.sofa               | `TestReadRealTFEAndSOS`                 | Tests expect FreeFieldHRTF / TF-E, R=2.                     |
| `SimpleFreeFieldHRSOS_1.0.sofa` | https://sofacoustics.org/data/examples/SimpleFreeFieldHRSOS_1.0.sofa        | `TestReadRealTFEAndSOS`                 | Tests expect SimpleFreeFieldHRSOS / SOS, R=2, N≥6.          |
| `demo_FreeFieldHRTF_4_SH.sofa`  | https://sofacoustics.org/data/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa | `TestReadSHEncodedTFE`                  | Tests expect FreeFieldHRTF / TF-E, E=1156 (Lmax=33), N=129. |

## Local-only (no known download URL)

Used by the BRIR/SRIR convention tests (Phase B). Both come from the
sofacoustics.org data server, but no stable URL could be confirmed (the server
answers 403, including to browser user agents), so `scripts/fetch-testdata.sh`
does not list them and their tests skip when they are absent. Copy them into
`testdata/` by hand; the hashes below
identify the expected files.

| File                      | Needed by                   | Producer (from the file)                                                                                                     | Licence as stated by the file                                                                                                                      | Size (bytes) | SHA-256                                                            |
| ------------------------- | --------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | -----------: | ------------------------------------------------------------------ |
| `OfficeII.sofa`           | `TestBRIRRoundTripOfficeII` | ARI SOFA API for Matlab/Octave 1.0.3; Kayser et al. 2009 Oldenburg in-ear/BTE BRIR database, SingleRoomDRIR, M=8 R=8 N=22000 | "Permission to use this database for purely research or educational purposes is granted. No commercial exploitation of this database is permitted" |     10099121 | `0491f6eb95f8c8768726c43b0f15e3194f1d57dabe1d380db283ca1a8949b8e8` |
| `SingleRoomSRIR_1.1.sofa` | `TestSRIRReadKnownFile`     | SOFA Toolbox for Matlab/Octave 2.2.1 demo script, SingleRoomSRIR, M=4800 R=1 N=1                                             | "No license provided, ask the author for permission"                                                                                               |        95529 | `60e965aa7e71675391b442376d059b6095f587cfbb7cb018c391c6bdafc4f614` |
