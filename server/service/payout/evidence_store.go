package payout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const MaxEvidenceBytes = 2 << 20

var objectRE = regexp.MustCompile(`^peo_[a-f0-9]{32}$`)

type EvidenceStore struct{ root *os.Root }

// Explicit private storage, outside the application tree. There is no public
// URL or static-file mount. Only authenticated service reads can access bytes.
func OpenEvidenceStore(path string, publicRoots ...string) (*EvidenceStore, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, ErrEvidence
	}
	if e := os.MkdirAll(path, 0700); e != nil {
		return nil, ErrEvidence
	}
	canonical, e := filepath.EvalSymlinks(path)
	if e != nil {
		return nil, ErrEvidence
	}
	cwd, e := os.Getwd()
	if e != nil {
		return nil, ErrEvidence
	}
	for _, root := range publicRoots {
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, ErrEvidence
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			abs = resolved
		}
		rel, err := filepath.Rel(abs, canonical)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, ErrEvidence
		}
	}
	if rel, e := filepath.Rel(cwd, canonical); e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, ErrEvidence
	}
	info, e := os.Stat(canonical)
	if e != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, ErrEvidence
	}
	r, e := os.OpenRoot(canonical)
	if e != nil {
		return nil, ErrEvidence
	}
	return &EvidenceStore{root: r}, nil
}
func (s *EvidenceStore) Close() error {
	if s == nil || s.root == nil {
		return nil
	}
	return s.root.Close()
}
func (s *EvidenceStore) Put(ctx context.Context, name string, b []byte) error {
	if s == nil || s.root == nil || !objectRE.MatchString(name) || len(b) > MaxEvidenceBytes+64 || ctx.Err() != nil {
		return ErrEvidence
	}
	f, e := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return ErrEvidence
	}
	_, e = f.Write(b)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil && ce == nil {
		// Persist the directory entry before committing evidence metadata. A
		// successful file fsync alone does not guarantee crash-safe creation.
		dir, err := s.root.Open(".")
		if err == nil {
			err = dir.Sync()
			closeErr := dir.Close()
			if err == nil {
				err = closeErr
			}
		}
		e = err
	}
	if e != nil || ce != nil {
		_ = s.root.Remove(name)
		return ErrEvidence
	}
	return nil
}
func (s *EvidenceStore) Read(ctx context.Context, name string) ([]byte, error) {
	if s == nil || s.root == nil || !objectRE.MatchString(name) || ctx.Err() != nil {
		return nil, ErrEvidence
	}
	f, e := s.root.Open(name)
	if e != nil {
		return nil, ErrEvidence
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > MaxEvidenceBytes+64 {
		return nil, ErrEvidence
	}
	b, e := io.ReadAll(io.LimitReader(f, MaxEvidenceBytes+65))
	if e != nil || len(b) > MaxEvidenceBytes+64 {
		return nil, ErrEvidence
	}
	return b, nil
}
func (s *EvidenceStore) Remove(name string) error {
	if s == nil || s.root == nil || !objectRE.MatchString(name) {
		return ErrEvidence
	}
	return s.root.Remove(name)
}
func validateEvidence(mime string, b []byte) (string, error) {
	if len(b) == 0 || len(b) > MaxEvidenceBytes || (mime != "image/png" && mime != "image/jpeg") {
		return "", ErrEvidence
	}
	config, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 4096 || config.Height > 4096 || int64(config.Width)*int64(config.Height) > 4_000_000 {
		return "", ErrEvidence
	}
	if (mime == "image/png" && format != "png") || (mime == "image/jpeg" && format != "jpeg") {
		return "", ErrEvidence
	}
	if mime == "image/png" && !bytes.HasSuffix(b, []byte{0, 0, 0, 0, 'I', 'E', 'N', 'D', 0xae, 0x42, 0x60, 0x82}) {
		return "", ErrEvidence
	}
	if mime == "image/jpeg" && !bytes.HasSuffix(b, []byte{0xff, 0xd9}) {
		return "", ErrEvidence
	}
	if _, _, e = image.Decode(bytes.NewReader(b)); e != nil {
		return "", ErrEvidence
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
