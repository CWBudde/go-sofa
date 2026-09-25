package sofa

import (
	"errors"
	"io/fs"
	"runtime"
	"syscall"
	"testing"
)

func TestSyncUnsupported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("every directory-sync error is tolerated on Windows")
	}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"EINVAL", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.EINVAL}, true},
		{"ENOTSUP", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.ENOTSUP}, true},
		{"ErrUnsupported", errors.ErrUnsupported, true},
		{"EIO", &fs.PathError{Op: "sync", Path: "/d", Err: syscall.EIO}, false},
		{"permission", fs.ErrPermission, false},
	}
	for _, tt := range tests {
		if got := syncUnsupported(tt.err); got != tt.want {
			t.Errorf("syncUnsupported(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
