#!/usr/bin/env python3
"""Interoperability check for files written by go-sofa's Save.

Reads every file listed in DIR/expected.json (written by
`go run ./internal/interop/gen DIR`) with both h5py and netCDF4 (netCDF-C),
and compares global attributes, dataset shapes and values against the
expected values. It also checks the netCDF-4 dimension model: netCDF must
report exactly the expected dimensions (M, R, E, N, C, I) with their
lengths, every variable must use named dimensions (no phony_dim_*), and
h5py must resolve each dataset's attached dimension scales. Exits non-zero
on any mismatch or open/read error.

    python3 scripts/interop_check.py DIR

Self-test of this script (no Go involved): write reference files with h5py
in the same layout as the generator's expectations, then check them:

    python3 scripts/interop_check.py --write-reference DIR   # DIR has expected.json
    python3 scripts/interop_check.py DIR
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import traceback

import h5py
import netCDF4
import numpy as np


def _as_str(v) -> str:
    if isinstance(v, bytes):
        return v.decode("utf-8")
    if isinstance(v, np.ndarray) and v.shape == ():
        return _as_str(v[()])
    if isinstance(v, np.ndarray) and v.size == 1:
        return _as_str(v.reshape(-1)[0])
    return str(v)


def _compare(errors: list[str], where: str, name: str, got, spec: dict) -> None:
    got = np.asarray(got, dtype=np.float64)
    want_shape = tuple(spec["shape"])
    want = np.asarray(spec["values"], dtype=np.float64).reshape(want_shape)
    if got.shape != want_shape:
        errors.append(f"{where}: {name}: shape {got.shape}, want {want_shape}")
        return
    if not np.array_equal(got, want):
        idx = np.argwhere(got != want)[0]
        errors.append(
            f"{where}: {name}: value mismatch at {tuple(int(i) for i in idx)}: "
            f"got {got[tuple(idx)]!r}, want {want[tuple(idx)]!r}"
        )


def check_h5py(path: str, exp: dict, errors: list[str]) -> None:
    where = f"h5py    {os.path.basename(path)}"
    with h5py.File(path, "r") as f:
        for key, want in exp["attributes"].items():
            if key not in f.attrs:
                errors.append(f"{where}: missing global attribute {key}")
            elif _as_str(f.attrs[key]) != want:
                errors.append(f"{where}: attribute {key} = {_as_str(f.attrs[key])!r}, want {want!r}")
        for name, spec in exp["datasets"].items():
            if name not in f:
                errors.append(f"{where}: missing dataset {name}")
                continue
            _compare(errors, where, name, f[name][()], spec)
            dims = spec.get("dims")
            if not dims or dims == [name]:
                continue  # coordinate variables are scales, not attached
            got = []
            for axis in f[name].dims:
                scales = list(axis.values())
                got.append(scales[0].name.lstrip("/") if scales else None)
            if got != dims:
                errors.append(f"{where}: {name}: attached dimension scales {got}, want {dims}")


def check_netcdf(path: str, exp: dict, errors: list[str]) -> None:
    where = f"netCDF4 {os.path.basename(path)}"
    with netCDF4.Dataset(path, "r") as nc:
        nc.set_auto_mask(False)
        attrs = nc.ncattrs()
        for key, want in exp["attributes"].items():
            if key not in attrs:
                errors.append(f"{where}: missing global attribute {key}")
            elif _as_str(nc.getncattr(key)) != want:
                errors.append(f"{where}: attribute {key} = {_as_str(nc.getncattr(key))!r}, want {want!r}")
        for name, spec in exp["datasets"].items():
            if name not in nc.variables:
                errors.append(f"{where}: missing variable {name}")
                continue
            _compare(errors, where, name, nc.variables[name][:], spec)
            if "dims" in spec and list(nc.variables[name].dimensions) != spec["dims"]:
                errors.append(f"{where}: {name}: dimensions {list(nc.variables[name].dimensions)}, want {spec['dims']}")
        want_dims = exp.get("dimensions", {})
        got_dims = {k: len(v) for k, v in nc.dimensions.items()}
        if want_dims and got_dims != want_dims:
            errors.append(f"{where}: dimensions {got_dims}, want {want_dims}")
        for name, var in nc.variables.items():
            phony = [d for d in var.dimensions if d.startswith("phony_dim")]
            if phony:
                errors.append(f"{where}: {name}: unnamed dimensions {list(var.dimensions)}")


def write_reference(directory: str) -> None:
    """Write each expected file with h5py, in the layout the generator uses."""
    with open(os.path.join(directory, "expected.json"), encoding="utf-8") as fh:
        expected = json.load(fh)
    for fname, exp in expected.items():
        path = os.path.join(directory, fname)
        with h5py.File(path, "w", track_order=True) as f:
            for key, val in exp["attributes"].items():
                f.attrs[key] = np.bytes_(val)
            scales = {}
            for dim, size in exp.get("dimensions", {}).items():
                spec = exp["datasets"].get(dim)
                if spec is not None:  # coordinate variable
                    ds = f.create_dataset(dim, data=np.asarray(spec["values"], dtype=np.float64))
                    ds.make_scale(dim)
                else:
                    ds = f.create_dataset(dim, data=np.zeros(size, dtype=np.float32))
                    ds.make_scale(f"This is a netCDF dimension but not a netCDF variable.{size:10d}")
                scales[dim] = ds
            for name, spec in exp["datasets"].items():
                if name in scales:
                    continue
                data = np.asarray(spec["values"], dtype=np.float64).reshape(spec["shape"])
                ds = f.create_dataset(name, data=data)
                for i, dim in enumerate(spec.get("dims", [])):
                    ds.dims[i].attach_scale(scales[dim])
        print(f"wrote reference {path}")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("dir", help="directory containing expected.json and the .sofa files")
    ap.add_argument("--write-reference", action="store_true", help="write reference files with h5py and exit")
    args = ap.parse_args()

    if args.write_reference:
        write_reference(args.dir)
        return 0

    with open(os.path.join(args.dir, "expected.json"), encoding="utf-8") as fh:
        expected = json.load(fh)

    print(f"h5py {h5py.__version__} (HDF5 {h5py.version.hdf5_version}), "
          f"netCDF4 {netCDF4.__version__} (netCDF-C {netCDF4.__netcdf4libversion__}, "
          f"HDF5 {netCDF4.__hdf5libversion__})")

    failed = False
    for fname in sorted(expected):
        path = os.path.join(args.dir, fname)
        for label, check in (("h5py", check_h5py), ("netCDF4", check_netcdf)):
            errors: list[str] = []
            try:
                check(path, expected[fname], errors)
            except Exception as exc:  # noqa: BLE001 - report every reader failure
                last = traceback.format_exception_only(type(exc), exc)[-1].strip()
                errors.append(f"{label:7} {fname}: cannot read: {last}")
            if errors:
                failed = True
                print(f"FAIL {label:7} {fname}")
                for e in errors:
                    print(f"     {e}")
            else:
                n = len(expected[fname]["datasets"])
                dims = " ".join(f"{k}={v}" for k, v in expected[fname].get("dimensions", {}).items())
                print(f"ok   {label:7} {fname} ({n} datasets, {len(expected[fname]['attributes'])} attributes; dims {dims})")

    if failed:
        print("interop check FAILED", file=sys.stderr)
        return 1
    print("interop check passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
