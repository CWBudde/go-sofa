#!/usr/bin/env python3
"""Interoperability check for files written by go-sofa's Save.

Reads every file listed in DIR/expected.json (written by
`go run ./internal/interop/gen DIR`) with both h5py and netCDF4 (netCDF-C),
and compares global attributes, dataset shapes and values against the
expected values, and requires every string attribute to be a scalar
fixed-length string (netCDF NC_CHAR text, not NC_STRING). Exits non-zero on
any mismatch or open/read error.

    python3 scripts/interop_check.py DIR

Self-test of this script (no Go involved): write reference files with h5py
in the same layout as the generator's expectations, then check them:

    python3 scripts/interop_check.py --write-reference DIR   # DIR has expected.json
    python3 scripts/interop_check.py DIR

When DIR/resaved/ exists (the generator re-saves testdata/sofar/, files
written by sofar through netCDF-C), each re-saved file is also compared with
its original: same global attributes and, per variable, the same values,
read with both h5py and netCDF4.
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


def _chars(arr) -> str:
    """Join a char array's elements row-major; an empty element is a NUL byte."""
    return "".join(c.decode("latin-1") if c else "\0" for c in np.asarray(arr).reshape(-1))


def _compare(errors: list[str], where: str, name: str, got, spec: dict) -> None:
    want_shape = tuple(spec["shape"])
    if "chars" in spec:
        got = np.asarray(got)
        if got.shape != want_shape:
            errors.append(f"{where}: {name}: shape {got.shape}, want {want_shape}")
        elif _chars(got) != spec["chars"]:
            errors.append(f"{where}: {name}: chars {_chars(got)!r}, want {spec['chars']!r}")
        return
    got = np.asarray(got, dtype=np.float64)
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


def _compare_attrs(errors: list[str], where: str, name: str, attrs, spec: dict) -> None:
    for key, want in spec.get("attrs", {}).items():
        if key not in attrs:
            errors.append(f"{where}: {name}: missing attribute {key}")
        elif _as_str(attrs[key]) != want:
            errors.append(f"{where}: {name}: attribute {key} = {_as_str(attrs[key])!r}, want {want!r}")


def _check_text_attrs(errors: list[str], where: str, f: h5py.File) -> None:
    """Require every string attribute to be a scalar fixed-length string.

    netCDF-C reads those as NC_CHAR text; a variable-length string or a
    one-element array becomes NC_STRING (`ncdump -h` prints `string :Title`),
    which the SOFA Toolbox under Octave cannot load.
    """
    prefix = f"{where}: " if where else ""
    objects = [("/", f)] + [(name, f[name]) for name in f]
    for name, obj in objects:
        for key in obj.attrs:
            attr = h5py.h5a.open(obj.id, key.encode())
            tid = attr.get_type()
            if not isinstance(tid, h5py.h5t.TypeStringID):
                continue
            if tid.is_variable_str() or attr.shape != ():
                kind = "variable-length" if tid.is_variable_str() else "fixed-length"
                errors.append(f"{prefix}{name}: attribute {key} is a {kind} string of shape {attr.shape}, want scalar fixed-length (NC_CHAR)")


def check_h5py(path: str, exp: dict, errors: list[str]) -> None:
    where = f"h5py    {os.path.basename(path)}"
    with h5py.File(path, "r") as f:
        _check_text_attrs(errors, where, f)
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
            _compare_attrs(errors, where, name, f[name].attrs, spec)


def check_netcdf(path: str, exp: dict, errors: list[str]) -> None:
    where = f"netCDF4 {os.path.basename(path)}"
    with netCDF4.Dataset(path, "r") as nc:
        nc.set_auto_mask(False)
        nc.set_auto_chartostring(False)
        attrs = nc.ncattrs()
        for key, want in exp["attributes"].items():
            if key not in attrs:
                errors.append(f"{where}: missing global attribute {key}")
            elif _as_str(nc.getncattr(key)) != want:
                errors.append(f"{where}: attribute {key} = {_as_str(nc.getncattr(key))!r}, want {want!r}")
        got_dims = {k: len(v) for k, v in nc.dimensions.items()}
        if got_dims != exp["dimensions"]:
            errors.append(f"{where}: dimensions {got_dims}, want {exp['dimensions']}")
        for name, var in nc.variables.items():
            phony = [d for d in var.dimensions if d.startswith("phony_dim")]
            if phony:
                errors.append(f"{where}: {name}: unnamed dimensions {var.dimensions}")
        for name, spec in exp["datasets"].items():
            if name not in nc.variables:
                errors.append(f"{where}: missing variable {name}")
                continue
            if list(nc.variables[name].dimensions) != spec["dims"]:
                errors.append(f"{where}: {name}: dimensions {nc.variables[name].dimensions}, want {tuple(spec['dims'])}")
            var = nc.variables[name]
            _compare(errors, where, name, var[:], spec)
            _compare_attrs(errors, where, name, {k: var.getncattr(k) for k in var.ncattrs()}, spec)


REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SOFAR_DIR = os.path.join(REPO, "testdata", "sofar")


def _read_all(path: str, label: str) -> tuple[dict, dict]:
    """Global attributes and variables of path, read with h5py or netCDF4."""
    attrs, variables = {}, {}
    if label == "h5py":
        with h5py.File(path, "r") as f:
            attrs = {k: _as_str(v) for k, v in f.attrs.items()}
            for name, ds in f.items():
                if "NAME" in ds.attrs and b"not a netCDF variable" in bytes(ds.attrs["NAME"]):
                    continue  # placeholder dimension scale
                variables[name] = ds[()]
    else:
        with netCDF4.Dataset(path, "r") as nc:
            nc.set_auto_mask(False)
            nc.set_auto_chartostring(False)
            attrs = {k: _as_str(nc.getncattr(k)) for k in nc.ncattrs()}
            variables = {name: var[:] for name, var in nc.variables.items()}
    attrs.pop("_NCProperties", None)
    return attrs, variables


def _same_values(orig, got) -> bool:
    orig, got = np.asarray(orig), np.asarray(got)
    if orig.dtype.kind in "SU" or got.dtype.kind in "SU":
        return _chars(orig).rstrip("\0") == _chars(got).rstrip("\0")
    orig, got = np.squeeze(orig.astype(np.float64)), np.squeeze(got.astype(np.float64))
    if orig.shape != got.shape:
        try:
            orig = np.broadcast_to(orig, got.shape)
        except ValueError:
            return False
    return bool(np.array_equal(orig, got))


def check_resaved(directory: str) -> bool:
    """Compare DIR/resaved/*.sofa with the originals in testdata/sofar/."""
    with open(os.path.join(directory, "resaved", "resaved.json"), encoding="utf-8") as fh:
        resaved = json.load(fh)
    failed = False
    for fname in sorted(resaved):
        for label in ("h5py", "netCDF4"):
            errors: list[str] = []
            try:
                want_attrs, want_vars = _read_all(os.path.join(SOFAR_DIR, fname), label)
                got_path = os.path.join(directory, "resaved", fname)
                got_attrs, got_vars = _read_all(got_path, label)
                if label == "h5py":
                    with h5py.File(got_path, "r") as f:
                        _check_text_attrs(errors, "", f)
                for key, want in want_attrs.items():
                    # Save does not write empty optional attributes.
                    if got_attrs.get(key, "") != want:
                        errors.append(f"attribute {key} = {got_attrs.get(key)!r}, want {want!r}")
                for name, want in want_vars.items():
                    if name not in got_vars:
                        errors.append(f"missing variable {name}")
                    elif not _same_values(want, got_vars[name]):
                        errors.append(f"{name}: values differ from the original")
            except Exception as exc:  # noqa: BLE001 - report every reader failure
                errors.append("cannot read: " + traceback.format_exception_only(type(exc), exc)[-1].strip())
            if errors:
                failed = True
                print(f"FAIL {label:7} resaved/{fname}")
                for e in errors:
                    print(f"     {e}")
            else:
                print(f"ok   {label:7} resaved/{fname} (same as the sofar original)")
    return failed


def write_reference(directory: str) -> None:
    """Write each expected file with h5py, in the layout the generator uses."""
    with open(os.path.join(directory, "expected.json"), encoding="utf-8") as fh:
        expected = json.load(fh)
    for fname, exp in expected.items():
        path = os.path.join(directory, fname)
        with h5py.File(path, "w", track_order=True) as f:
            for key, val in exp["attributes"].items():
                f.attrs[key] = np.bytes_(val)
            datasets = {}
            for name, spec in exp["datasets"].items():
                if "chars" in spec:
                    data = np.array([c.encode("latin-1") for c in spec["chars"]], dtype="S1").reshape(spec["shape"])
                else:
                    data = np.asarray(spec["values"], dtype=np.float64).reshape(spec["shape"])
                datasets[name] = f.create_dataset(name, data=data)
                for key, val in spec.get("attrs", {}).items():
                    datasets[name].attrs[key] = np.bytes_(val)
            # Dimension scales as netCDF-C writes them: a coordinate variable
            # when a dataset has the dimension's name, else a placeholder.
            scales = {}
            for dim, size in exp["dimensions"].items():
                if dim in datasets:
                    datasets[dim].make_scale(dim)
                    scales[dim] = datasets[dim]
                else:
                    scales[dim] = f.create_dataset(dim, (size,), dtype="f4")
                    scales[dim].make_scale(f"This is a netCDF dimension but not a netCDF variable.{size:10d}")
            for name, spec in exp["datasets"].items():
                if name in scales:
                    continue
                for i, dim in enumerate(spec["dims"]):
                    datasets[name].dims[i].attach_scale(scales[dim])
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
                print(f"ok   {label:7} {fname} ({n} datasets, {len(expected[fname]['attributes'])} attributes)")

    if os.path.isdir(os.path.join(args.dir, "resaved")):
        failed = check_resaved(args.dir) or failed

    if failed:
        print("interop check FAILED", file=sys.stderr)
        return 1
    print("interop check passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
