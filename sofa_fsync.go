package sofa

import (
	"errors"
	"runtime"
	"syscall"
)

// syncUnsupported reports whether err from syncing a directory only means
// that the platform or filesystem cannot fsync directories (Windows cannot
// open them; some filesystems answer EINVAL or ENOTSUP). Such errors are
// ignored; any other error is a real I/O failure and is returned by Save.
func syncUnsupported(err error) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return errors.Is(err, errors.ErrUnsupported) ||
		errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.ENOTSUP)
}
