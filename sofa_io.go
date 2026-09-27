package sofa

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	hdf5 "github.com/cwbudde/go-hdf5"
)

// OpenReader reads a SOFA file of size bytes from r, such as a
// bytes.Reader over a file held in memory or an embedded asset, like Open
// reads one from disk: it reads all data and metadata into the returned
// File, and r is not used after OpenReader returns. Reads stay within the
// first size bytes of r. OpenReader never closes r.
func OpenReader(r io.ReaderAt, size int64) (*File, error) {
	return openReader(r, size, false)
}

// openReader reads a SOFA file of size bytes from r; see read for lazy.
func openReader(r io.ReaderAt, size int64, lazy bool) (*File, error) {
	h, err := hdf5.OpenReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("open HDF5: %w", err)
	}
	return read(h, size, lazy)
}

// open reads the SOFA file at path; see read for lazy. The read budget
// comes from the size of the file it opened, not of whatever path names by
// the time of a second lookup.
func open(path string, lazy bool) (*File, error) {
	osf, err := os.Open(path) //nolint:gosec // G304: opening the caller's path is the point
	if err != nil {
		return nil, fmt.Errorf("open HDF5: %w", err)
	}
	fi, err := osf.Stat()
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open HDF5: %w", err), osf.Close())
	}
	f, err := openReader(osf, fi.Size(), lazy)
	if err != nil || !lazy {
		if cerr := osf.Close(); cerr != nil {
			return nil, errors.Join(err, fmt.Errorf("close HDF5: %w", cerr))
		}
		return f, err
	}
	f.lazy.file = osf
	return f, nil
}

// Save writes the SOFA file to the specified path.
// It validates the File struct before writing and creates a fully compliant
// SOFA file with netCDF-4/HDF5 dimension scales.
//
// Save is atomic: the file is written to a temporary file in the same
// directory, flushed and fsynced, and then renamed over path. A failed
// Save leaves any existing file at path untouched and removes the
// temporary file. If path already exists its permission bits are kept;
// otherwise the new file gets mode 0644.
//
// Like the SOFA Toolbox's SOFAsave, Save records its own provenance in the
// written file, without changing the File: APIName "go-sofa" and
// go-sofa's module version as APIVersion, the save time as DateModified,
// and as DateCreated when that is empty. When the File names another API
// (or another go-sofa version), History gains the line "resaved by
// go-sofa <version> from <APIName> <APIVersion>". Set ApplicationName and
// ApplicationVersion to record your program.
//
// The save time is the current time, or the SOURCE_DATE_EPOCH environment
// variable (seconds since the Unix epoch) when set; a malformed value,
// the empty string included, is an error. Output is otherwise deterministic: with SOURCE_DATE_EPOCH set,
// saving the same File twice produces byte-identical files.
//
// All required SOFA attributes and datasets are written, along with optional
// fields if present in the File struct. ReceiverPositions and
// SourcePositions are required; an empty ListenerPositions or
// EmitterPositions is written as the conventions' default, [0 0 0]
// cartesian in metres, again without changing the File. So are global
// attributes the file's SOFAConventions makes mandatory beyond the generic
// ones (DatabaseName and ListenerShortName for SimpleFreeFieldHRIR, …):
// Save writes the empty default the SOFA convention tables give them when
// Attributes lacks one. Likewise, the SourceView and SourceUp variables
// that SingleRoomSRIR, SingleRoomDRIR and FreeFieldDirectivityTF make
// mandatory are written with the tables' defaults (and the attributes
// VariableAttributes holds for them) when Variables lacks them, and the
// Reference attribute FreeFieldDirectivityTF makes mandatory on
// SourcePosition, SourceView and SourceUp as "" where none is set. An
// empty RoomType is written as the convention's default: reverberant for
// SingleRoomDRIR, shoebox for SingleRoomSRIR when Variables holds
// RoomCornerA and RoomCornerB, free field otherwise.
//
// Returns an error if:
//   - Validation fails (missing required fields, invalid dimensions, etc.)
//   - HDF5 file creation fails
//   - Any write, flush, close, sync or rename operation fails
//   - f came from OpenLazy and its audio fields are still empty
//     (ErrNotLoaded)
func (f *File) Save(path string) (err error) {
	if err := f.checkWritable("save " + path); err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	if fi, statErr := os.Stat(path); statErr == nil {
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("save %s: not a regular file", path)
		}
		mode = fi.Mode().Perm()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("create temporary file: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	create := func(opts []interface{}) (*hdf5.FileWriter, error) {
		return hdf5.CreateForWrite(tmpName, hdf5.CreateTruncate, opts...)
	}
	if err := f.writeHDF5(create, nil); err != nil {
		return err
	}
	if err := syncFile(tmpName); err != nil {
		return fmt.Errorf("sync %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s to %s: %w", tmpName, path, err)
	}
	// Persist the directory entry. Platforms and filesystems that cannot
	// fsync a directory are tolerated; real I/O errors are returned.
	if err := syncFile(dir); err != nil && !syncUnsupported(err) {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}

// WriteTo writes the SOFA file to w, as Save writes it to a file: it
// validates f first and produces the same bytes. It returns the number of
// bytes written, and implements io.WriterTo.
//
// The file is assembled in memory (HDF5 needs random access while writing)
// and handed to w in one Write only once it is complete, so a validation
// or encoding error writes nothing to w. An error from w is returned
// together with the bytes w accepted.
func (f *File) WriteTo(w io.Writer) (n int64, err error) {
	if w == nil {
		return 0, errors.New("write: nil io.Writer")
	}
	if err := f.checkWritable("write"); err != nil {
		return 0, err
	}
	sink := &commitWriter{w: w}
	create := func(opts []interface{}) (*hdf5.FileWriter, error) {
		return hdf5.CreateForWriteTo(sink, opts...)
	}
	err = f.writeHDF5(create, func() { sink.committed = true })
	return sink.n, err
}

// checkWritable returns an error unless f can be written: ErrNotLoaded
// (prefixed with op) for a File from OpenLazy whose audio fields are
// empty, or the validation error.
func (f *File) checkWritable(op string) error {
	if f.lazy != nil && !f.hasAudio() {
		return fmt.Errorf("%s: %w", op, ErrNotLoaded)
	}
	if err := f.validate(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	return nil
}

// commitWriter passes writes to w and counts the bytes w accepted, once
// committed; before that it discards them, so an HDF5 writer closed after
// a failed write leaves w untouched.
type commitWriter struct {
	w         io.Writer
	n         int64
	committed bool
}

func (c *commitWriter) Write(p []byte) (int, error) {
	if !c.committed {
		return len(p), nil
	}
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// saveTestHook, when non-nil, is called by writeHDF5 after all datasets
// have been written and before the writer is closed. Tests use it to
// inject a failure late in Save.
var saveTestHook func() error

// syncFile opens name and fsyncs it.
func syncFile(name string) error {
	fd, err := os.Open(name) //nolint:gosec // path chosen by Save
	if err != nil {
		return err
	}
	return errors.Join(fd.Sync(), fd.Close())
}
