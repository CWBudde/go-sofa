# Test data provenance

The `*.sofa` files in this directory are third-party reference files. They are
**not committed** (most carry no licence, or a licence that does not clearly
permit redistribution in this repository) and are fetched on demand:

```sh
just fetch-testdata      # or: scripts/fetch-testdata.sh
```

`scripts/fetch-testdata.sh` is the authoritative manifest (URL + SHA-256).
Tests that need a file fail with a pointer to that command when it is absent.
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

## sofacoustics.org (SOFA Toolbox examples and demo output)

Hosted by the Acoustics Research Institute at https://sofacoustics.org/data/
(same server as `www.sofaconventions.org/data/`). The `examples/` files are the
example files linked from the per-convention pages of the SOFA conventions wiki;
the `sofatoolbox_test/` files are written by the SOFA Toolbox demos
(`demo_FreeFieldHRTF.m`).

**Not yet pinned:** the host was unreachable from the environment in which this
manifest was written, so the SHA-256, size, producer and licence of these files
have not been verified. `scripts/fetch-testdata.sh` downloads them without
hash verification and prints their hash; pin it in the script and here on the
first successful fetch.

| File                            | Source URL                                                                                   | Notes                                                                                                                                                                          |
| ------------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `GeneralTF_2.0.sofa`            | https://sofacoustics.org/data/examples/GeneralTF_2.0.sofa                                    | Tests expect GeneralTF / TF, M=4 R=4 E=1.                                                                                                                                      |
| `GeneralTF-E_1.0.sofa`          | https://sofacoustics.org/data/examples/GeneralTF-E_1.0.sofa                                  | Tests expect GeneralTF-E / TF-E, M≥4 R=4800.                                                                                                                                   |
| `FreeFieldHRTF_1.0.sofa`        | https://sofacoustics.org/data/examples/FreeFieldHRTF_1.0.sofa                                | Tests expect FreeFieldHRTF / TF-E, R=2.                                                                                                                                        |
| `SimpleFreeFieldHRSOS_1.0.sofa` | https://sofacoustics.org/data/examples/SimpleFreeFieldHRSOS_1.0.sofa                         | Tests expect SimpleFreeFieldHRSOS / SOS, R=2, N≥6.                                                                                                                             |
| `FreeFieldHRTF_2.0.sofa`        | https://sofacoustics.org/data/sofatoolbox_test/demo_FreeFieldHRTF_2_SimpleFreeFieldHRTF.sofa | Local name kept for the existing tests. Source is inferred (tests expect SimpleFreeFieldHRTF / TF, M=2354, which matches this demo's HRIR_L2354 input); verify on first fetch. |
| `demo_FreeFieldHRTF_4_SH.sofa`  | https://sofacoustics.org/data/sofatoolbox_test/demo_FreeFieldHRTF_4_SH.sofa                  | Tests expect FreeFieldHRTF / TF-E, E=1156 (Lmax=33), N=129.                                                                                                                    |
