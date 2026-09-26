#!/usr/bin/env python3
"""Rewrite the string attributes of a go-sofa-written SOFA file as netCDF text.

go-hdf5 stores every string attribute with a one-element dataspace, which
netCDF-C reads as an NC_STRING array rather than NC_CHAR text (``ncdump -h``
prints ``string :Title = ...``). The SOFA Toolbox under GNU Octave cannot
load such files (PLAN.md E8). Until go-hdf5 writes scalar string
attributes, this copies INFILE to OUTFILE and rewrites those attributes with
a scalar dataspace, as netCDF-C does. Data and positions are not touched.

    python3 scripts/matlab/char_attributes.py INFILE OUTFILE
"""

from __future__ import annotations

import shutil
import sys

import h5py
import numpy as np

# HDF5 dimension-scale and netCDF bookkeeping attributes stay as they are.
KEEP = {
    "CLASS",
    "NAME",
    "REFERENCE_LIST",
    "DIMENSION_LIST",
    "_Netcdf4Dimid",
    "_Netcdf4Coordinates",
    "_NCProperties",
}


def _fix(obj) -> int:
    n = 0
    for key in list(obj.attrs):
        if key in KEEP:
            continue
        attr = obj.attrs.get_id(key)
        if attr.get_type().get_class() == h5py.h5t.STRING and attr.shape == (1,):
            value = obj.attrs[key][0]
            del obj.attrs[key]
            obj.attrs[key] = np.bytes_(value)
            n += 1
    return n


def main() -> int:
    if len(sys.argv) != 3:
        print(__doc__, file=sys.stderr)
        return 2
    src, dst = sys.argv[1:]
    shutil.copyfile(src, dst)
    with h5py.File(dst, "r+") as f:
        n = _fix(f) + sum(_fix(f[name]) for name in f)
    print(f"rewrote {n} string attributes as text in {dst}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
