package internal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Cutter reuses its open file. Routine writes never scan the log directory.
type Cutter struct {
	level        string
	layout       string
	formats      []string
	director     string
	retentionDay int
	file         *os.File
	mutex        *sync.RWMutex
	filename     string
	size         int64
	maxBytes     int64
	backups      int
	lastCleanup  time.Time
}
type CutterOption func(*Cutter)

func CutterWithLayout(layout string) CutterOption { return func(c *Cutter) { c.layout = layout } }
func CutterWithFormats(formats ...string) CutterOption {
	return func(c *Cutter) {
		for _, f := range formats {
			if f != "" && f != "." && f != ".." && !strings.ContainsAny(f, "/\\\x00") {
				c.formats = append(c.formats, f)
			}
		}
	}
}
func NewCutter(director, level string, retentionDay int, options ...CutterOption) *Cutter {
	c := &Cutter{level: level, director: director, retentionDay: retentionDay, mutex: new(sync.RWMutex), maxBytes: 64 << 20, backups: 3}
	for _, option := range options {
		option(c)
	}
	return c
}
func (c *Cutter) Write(b []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if int64(len(b)) > c.maxBytes {
		return 0, errors.New("log entry exceeds bounded file size")
	}
	parts := []string{c.director}
	if c.layout != "" {
		parts = append(parts, time.Now().Format(c.layout))
	}
	parts = append(parts, c.formats...)
	parts = append(parts, c.level+".log")
	filename := filepath.Join(parts...)
	if c.file != nil && filename != c.filename {
		if e := c.close(); e != nil {
			return 0, e
		}
	}
	if c.file == nil {
		if c.director == "" {
			return 0, errors.New("private log directory required")
		}
		if e := privateLogDirectory(c.director); e != nil {
			return 0, e
		}
		if e := privateLogDirectory(filepath.Dir(filename)); e != nil {
			return 0, e
		}
		if e := c.open(filename); e != nil {
			return 0, e
		}
	}
	if c.size+int64(len(b)) > c.maxBytes {
		if e := c.close(); e != nil {
			return 0, e
		}
		if e := os.Remove(filename + fmt.Sprintf(".%d", c.backups)); e != nil && !os.IsNotExist(e) {
			return 0, e
		}
		for i := c.backups - 1; i >= 1; i-- {
			if e := os.Rename(filename+fmt.Sprintf(".%d", i), filename+fmt.Sprintf(".%d", i+1)); e != nil && !os.IsNotExist(e) {
				return 0, e
			}
		}
		if e := os.Rename(filename, filename+".1"); e != nil && !os.IsNotExist(e) {
			return 0, e
		}
		if e := c.open(filename); e != nil {
			return 0, e
		}
	}
	n, e := c.file.Write(b)
	c.size += int64(n)
	if time.Since(c.lastCleanup) > time.Hour {
		c.lastCleanup = time.Now()
		if cleanupErr := removeNDaysFolders(c.director, c.retentionDay); cleanupErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, "log retention cleanup failed")
		}
	}
	return n, e
}
func (c *Cutter) open(filename string) error {
	// Refuse a pre-existing symlink before opening; log directories are private
	// to the service account. Untrusted users must not have directory write access.
	if st, e := os.Lstat(filename); e == nil && !st.Mode().IsRegular() {
		return errors.New("non-regular log target rejected")
	}
	f, e := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	s, e := f.Stat()
	if e != nil {
		_ = f.Close()
		return e
	}
	if !s.Mode().IsRegular() {
		_ = f.Close()
		return errors.New("log target must be regular file")
	}
	// Upgrade an existing 0644 file as well as creating new private files.
	if e := f.Chmod(0600); e != nil {
		_ = f.Close()
		return e
	}
	c.file, c.filename, c.size = f, filename, s.Size()
	return nil
}

func privateLogDirectory(path string) error {
	if e := os.MkdirAll(path, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(path)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return errors.New("private log directory must not be a symlink")
	}
	// Restrict the root too: historical rotated files from the old writer can
	// remain readable by mode, but cannot be traversed through this private root.
	return os.Chmod(path, 0700)
}
func (c *Cutter) close() error {
	if c.file == nil {
		return nil
	}
	e := c.file.Close()
	c.file = nil
	return e
}
func (c *Cutter) Close() error { c.mutex.Lock(); defer c.mutex.Unlock(); return c.close() }
func (c *Cutter) Sync() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.file != nil {
		return c.file.Sync()
	}
	return nil
}

// Only direct, date-named directories may be removed. Modification time is not
// a retention timestamp; arbitrary sibling directories and symlinks survive.
func removeNDaysFolders(dir string, days int) error {
	if days <= 0 {
		return nil
	}
	entries, e := os.ReadDir(dir)
	if e != nil {
		return e
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format(time.DateOnly)
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if _, e := time.Parse(time.DateOnly, name); e != nil || name >= cutoff {
			continue
		}
		if e := os.RemoveAll(filepath.Join(dir, name)); e != nil {
			return e
		}
	}
	return nil
}
