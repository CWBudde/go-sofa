#!/usr/bin/env python3
"""Generate the committed SOFA fixtures in testdata/sofar/ with sofar.

sofar (https://github.com/pyfar/sofar) writes SOFA files through
netCDF4-python and netCDF-C, independently of go-sofa's writer. These small
files stand in, in CI, for the reference files that only sofacoustics.org
hosts (see testdata/PROVENANCE.md): one file per convention/DataType that the
tests could otherwise only exercise locally.

All content is synthetic and deterministic: values come from a seeded
generator, and the date and application attributes are fixed, so the same
sofar/netCDF-C/HDF5 versions write byte-identical files. The expected values
are pinned in sofa_sofar_fixtures_test.go; regenerating with other library
versions may change the bytes (update the hashes in testdata/PROVENANCE.md)
but must not change the values.

    pip install sofar==1.3.0
    python3 scripts/make_sofar_fixtures.py [DEST_DIR]   # default testdata/sofar
"""

from __future__ import annotations

import hashlib
import os
import sys
import warnings

import numpy as np
import sofar as sf
from sofar.io import _write_sofa

DATE = "2026-09-26 12:00:00"
LICENSE = "MIT (synthetic test data generated for go-sofa)"


def _new(convention: str, version: str, title: str) -> sf.Sofa:
    with warnings.catch_warnings():
        # MultiSpeakerBRIR 0.3 and SingleRoomDRIR 0.3 are preliminary/deprecated.
        warnings.simplefilter("ignore")
        sofa = sf.Sofa(convention, version=version)
    sofa.protected = False
    sofa.GLOBAL_DateCreated = DATE
    sofa.GLOBAL_DateModified = DATE
    sofa.GLOBAL_ApplicationName = "go-sofa scripts/make_sofar_fixtures.py"
    sofa.GLOBAL_ApplicationVersion = "1"
    sofa.protected = True
    sofa.GLOBAL_Title = title
    sofa.GLOBAL_AuthorContact = "go-sofa maintainers"
    sofa.GLOBAL_Organization = "go-sofa"
    sofa.GLOBAL_License = LICENSE
    return sofa


def _rng(seed: int) -> np.random.Generator:
    return np.random.default_rng(seed)


def general_tf(rng: np.random.Generator) -> sf.Sofa:
    """GeneralTF 2.0, TF, M=4 R=4 E=1 N=8."""
    s = _new("GeneralTF", "2.0", "GeneralTF 2.0 fixture")
    s.N = np.array([125.0, 250, 500, 1000, 2000, 4000, 8000, 16000])
    s.Data_Real = np.round(rng.standard_normal((4, 4, 8)), 6)
    s.Data_Imag = np.round(rng.standard_normal((4, 4, 8)), 6)
    s.ListenerPosition = [0.0, 0.0, 0.0]
    s.ReceiverPosition = np.array(
        [[0.0, 0.09, 0.0], [0.0, -0.09, 0.0], [0.05, 0.0, 0.0], [-0.05, 0.0, 0.0]]
    )
    s.SourcePosition = np.array(
        [[0.0, 0.0, 1.5], [90.0, 0.0, 1.5], [180.0, 0.0, 1.5], [270.0, 30.0, 1.5]]
    )
    return s


def general_tfe(rng: np.random.Generator) -> sf.Sofa:
    """GeneralTF-E 1.0, TF-E, M=4 R=2 E=3 N=6, Data [M,R,N,E]."""
    s = _new("GeneralTF-E", "1.0", "GeneralTF-E 1.0 fixture")
    s.N = np.array([100.0, 200, 400, 800, 1600, 3200])
    s.Data_Real = np.round(rng.standard_normal((4, 2, 6, 3)), 6)
    s.Data_Imag = np.round(rng.standard_normal((4, 2, 6, 3)), 6)
    s.ReceiverPosition = np.array([[0.0, 0.09, 0.0], [0.0, -0.09, 0.0]])
    s.SourcePosition = np.array(
        [[0.0, 0.0, 2.0], [45.0, 0.0, 2.0], [90.0, 10.0, 2.0], [135.0, -10.0, 2.0]]
    )
    s.EmitterPosition = np.array([[0.0, 0.0, 0.0], [0.1, 0.0, 0.0], [0.0, 0.1, 0.0]])
    s.EmitterPosition_Type = "cartesian"
    s.EmitterPosition_Units = "metre"
    return s


def free_field_hrtf(rng: np.random.Generator) -> sf.Sofa:
    """FreeFieldHRTF 1.0, TF-E, M=3 R=2 E=1 N=5, plain (not SH) emitter."""
    s = _new("FreeFieldHRTF", "1.0", "FreeFieldHRTF 1.0 fixture")
    s.N = np.array([500.0, 1000, 2000, 4000, 8000])
    s.Data_Real = np.round(rng.standard_normal((3, 2, 5, 1)), 6)
    s.Data_Imag = np.round(rng.standard_normal((3, 2, 5, 1)), 6)
    s.SourcePosition = np.array(
        [[0.0, 0.0, 1.2], [120.0, 0.0, 1.2], [240.0, 45.0, 1.2]]
    )
    s.EmitterPosition = [0.0, 0.0, 0.0]
    s.EmitterPosition_Type = "cartesian"
    s.EmitterPosition_Units = "metre"
    return s


def free_field_hrtf_sh(rng: np.random.Generator) -> sf.Sofa:
    """FreeFieldHRTF 1.0, TF-E, SH order 2: M=1 R=2 E=9 N=4."""
    s = _new("FreeFieldHRTF", "1.0", "FreeFieldHRTF 1.0 spherical-harmonics fixture")
    s.N = np.array([250.0, 1000, 4000, 16000])
    s.Data_Real = np.round(rng.standard_normal((1, 2, 4, 9)), 6)
    s.Data_Imag = np.round(rng.standard_normal((1, 2, 4, 9)), 6)
    s.SourcePosition = [0.0, 0.0, 1.0]
    s.EmitterPosition = np.zeros((9, 3))
    s.EmitterPosition_Type = "spherical harmonics"
    s.EmitterPosition_Units = "degree, degree, metre"
    return s


def simple_hrsos(rng: np.random.Generator) -> sf.Sofa:
    """SimpleFreeFieldHRSOS 1.0, SOS, M=6 R=2 E=1 N=12 (two biquads)."""
    s = _new("SimpleFreeFieldHRSOS", "1.0", "SimpleFreeFieldHRSOS 1.0 fixture")
    sos = np.round(rng.uniform(-0.9, 0.9, (6, 2, 12)), 6)
    sos[:, :, 3] = 1.0  # a0 of each biquad
    sos[:, :, 9] = 1.0
    s.Data_SOS = sos
    s.Data_SamplingRate = 44100.0
    s.Data_Delay = np.zeros((1, 2))
    s.SourcePosition = np.array(
        [[az, 0.0, 1.0] for az in range(0, 360, 60)], dtype=float
    )
    return s


def single_room_srir(rng: np.random.Generator) -> sf.Sofa:
    """SingleRoomSRIR 1.0, FIR, first-order Ambisonics: M=3 R=4 E=1 N=64."""
    s = _new("SingleRoomSRIR", "1.0", "SingleRoomSRIR 1.0 fixture")
    s.GLOBAL_RoomType = "shoebox"
    s.RoomVolume = 120.5
    s.RoomTemperature = 293.15
    ir = np.round(rng.standard_normal((3, 4, 64)) * np.exp(-np.arange(64) / 16.0), 6)
    s.Data_IR = ir
    s.Data_SamplingRate = 48000.0
    s.Data_Delay = np.zeros((1, 4))
    s.ListenerPosition = np.array([[2.0, 3.0, 1.2], [2.5, 3.0, 1.2], [3.0, 3.0, 1.2]])
    s.ReceiverPosition = np.zeros((4, 3))
    s.ReceiverDescriptions = np.array(["W", "Y", "Z", "X"])
    s.ReceiverView = np.tile([1.0, 0.0, 0.0], (4, 1))
    s.ReceiverUp = np.tile([0.0, 0.0, 1.0], (4, 1))
    s.SourcePosition = np.array([[5.0, 1.0, 1.5], [5.0, 1.0, 1.5], [5.0, 1.0, 1.5]])
    s.MeasurementDate = np.array([1.0e9, 1.0e9 + 60, 1.0e9 + 120])
    return s


def single_room_drir(rng: np.random.Generator) -> sf.Sofa:
    """SingleRoomDRIR 0.3 (BRIR), FIR: M=2 R=2 E=1 N=48."""
    s = _new("SingleRoomDRIR", "0.3", "SingleRoomDRIR 0.3 BRIR fixture")
    s.GLOBAL_RoomType = "reverberant"
    s.Data_IR = np.round(
        rng.standard_normal((2, 2, 48)) * np.exp(-np.arange(48) / 12.0), 6
    )
    s.Data_SamplingRate = 48000.0
    s.Data_Delay = np.zeros((1, 2))
    s.ListenerPosition = [1.0, 2.0, 1.5]
    s.ListenerView = [0.0, 1.0, 0.0]
    s.ReceiverPosition = np.array([[0.0, 0.09, 0.0], [0.0, -0.09, 0.0]])
    s.SourcePosition = np.array([[3.0, 4.0, 1.5], [0.0, 4.0, 1.5]])
    return s


FIXTURES = [
    ("GeneralTF_2.0.sofa", general_tf),
    ("GeneralTF-E_1.0.sofa", general_tfe),
    ("FreeFieldHRTF_1.0.sofa", free_field_hrtf),
    ("FreeFieldHRTF_1.0_SH_L2.sofa", free_field_hrtf_sh),
    ("SimpleFreeFieldHRSOS_1.0.sofa", simple_hrsos),
    ("SingleRoomSRIR_1.0.sofa", single_room_srir),
    ("SingleRoomDRIR_0.3.sofa", single_room_drir),
]


def _verify(sofa: sf.Sofa) -> None:
    """Run sofar's write verification, tolerating only the deprecation of
    SingleRoomDRIR (still the convention of many BRIR files in the wild)."""
    issues = sofa.verify(issue_handling="return", mode="write")
    if not issues:
        return
    errors = issues.split("WARNINGS")[0]
    allowed = "GLOBAL_SOFAConventions is SingleRoomDRIR, which is deprecated"
    lines = [
        ln for ln in errors.splitlines() if ln.startswith("- ") and allowed not in ln
    ]
    if lines:
        raise ValueError(issues)


def main() -> int:
    repo = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    dest = sys.argv[1] if len(sys.argv) > 1 else os.path.join(repo, "testdata", "sofar")
    os.makedirs(dest, exist_ok=True)
    for seed, (name, make) in enumerate(FIXTURES, start=1):
        sofa = make(_rng(seed))
        path = os.path.join(dest, name)
        with warnings.catch_warnings():
            warnings.simplefilter("ignore")
            _verify(sofa)
            _write_sofa(path, sofa, compression=4, verify=False)
        with open(path, "rb") as fh:
            digest = hashlib.sha256(fh.read()).hexdigest()
        print(f"{digest}  {os.path.getsize(path):7d}  {name}")
    print(
        f"sofar {sf.__version__}, netCDF4-python {_nc().__version__}, "
        f"netCDF-C {_nc().__netcdf4libversion__}, HDF5 {_nc().__hdf5libversion__}"
    )
    return 0


def _nc():
    import netCDF4

    return netCDF4


if __name__ == "__main__":
    sys.exit(main())
