//go:build unix

package internal

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"syscall"
	"testing"
)

func TestCutterRejectsFIFOWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, "info.log"), 0600))
	c := NewCutter(dir, "info", 7)
	_, e := c.Write([]byte("must not open pipe"))
	require.Error(t, e)
}
