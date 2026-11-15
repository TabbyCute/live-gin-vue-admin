package anchorhome

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"github.com/google/uuid"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"gorm.io/gorm"
	"image"
	"image/color"
	stddraw "image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"os"
	"regexp"
	"strings"
	model "tb_live_module/model/anchorhome"
	"time"
)

var storageKeyPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validPhotoNo(no string) bool {
	return strings.HasPrefix(no, "ph_") && storageKeyPattern.MatchString(strings.TrimPrefix(no, "ph_"))
}

type preparedPhoto struct {
	key           string
	width, height int
	bytes         int64
}

func (s *AlbumService) prepare(ctx context.Context, data []byte) (preparedPhoto, error) {
	var out preparedPhoto
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return out, ErrInvalid
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return out, ErrInvalid
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 12000 || cfg.Height > 12000 || int64(cfg.Width)*int64(cfg.Height) > 20_000_000 {
		return out, ErrInvalid
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	im, decodedFormat, err := image.Decode(bytes.NewReader(data))
	if err != nil || decodedFormat != format || im.Bounds().Dx() != cfg.Width || im.Bounds().Dy() != cfg.Height {
		return out, ErrInvalid
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	full := resized(im, 2048)
	if format == "jpeg" {
		full = orient(full, jpegOrientation(data))
	}
	thumb := resized(full, 480)
	fullBytes, err := encodeJPEG(full)
	if err != nil {
		return out, err
	}
	thumbBytes, err := encodeJPEG(thumb)
	if err != nil {
		return out, err
	}
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	out = preparedPhoto{key: strings.ReplaceAll(uuid.NewString(), "-", ""), width: full.Bounds().Dx(), height: full.Bounds().Dy(), bytes: int64(len(fullBytes) + len(thumbBytes))}
	if err := s.writeImage(out.key+".full.jpg", fullBytes); err != nil {
		return preparedPhoto{}, err
	}
	if err := s.writeImage(out.key+".thumb.jpg", thumbBytes); err != nil {
		_ = s.root.Remove(out.key + ".full.jpg")
		return preparedPhoto{}, err
	}
	// fsync the directory entry as well as bytes before committing metadata.
	d, err := s.root.Open(".")
	if err != nil {
		s.removePrepared(out.key)
		return preparedPhoto{}, err
	}
	err = d.Sync()
	_ = d.Close()
	if err != nil {
		s.removePrepared(out.key)
	}
	return out, err
}
func (s *AlbumService) removePrepared(key string) {
	if !storageKeyPattern.MatchString(key) {
		return
	}
	_ = s.root.Remove(key + ".full.jpg")
	_ = s.root.Remove(key + ".thumb.jpg")
}
func resized(src image.Image, longSide int) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > longSide || h > longSide {
		if w >= h {
			h = max(1, h*longSide/w)
			w = longSide
		} else {
			w = max(1, w*longSide/h)
			h = longSide
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	stddraw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.White}, image.Point{}, stddraw.Src)
	if w == b.Dx() && h == b.Dy() {
		stddraw.Draw(dst, dst.Bounds(), src, b.Min, stddraw.Over)
		return dst
	}
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, b, stddraw.Over, nil)
	return dst
}
func encodeJPEG(im image.Image) ([]byte, error) {
	var b bytes.Buffer
	err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 88})
	return b.Bytes(), err
}
func (s *AlbumService) writeImage(name string, data []byte) error {
	f, err := s.root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		_ = s.root.Remove(name)
		return err
	}
	if closeErr != nil {
		_ = s.root.Remove(name)
	}
	return closeErr
}

// jpegOrientation reads only bounded EXIF IFD0 orientation, not arbitrary metadata.
func jpegOrientation(b []byte) int {
	if len(b) < 4 || b[0] != 0xff || b[1] != 0xd8 {
		return 1
	}
	for offset := 2; offset+4 <= len(b); {
		if b[offset] != 0xff {
			return 1
		}
		marker := b[offset+1]
		if marker == 0xda || marker == 0xd9 {
			return 1
		}
		if marker == 0xff {
			offset++
			continue
		}
		size := int(binary.BigEndian.Uint16(b[offset+2 : offset+4]))
		if size < 2 || size > len(b)-offset-2 {
			return 1
		}
		segment := b[offset+4 : offset+2+size]
		offset += size + 2
		if marker != 0xe1 || len(segment) < 14 || !bytes.Equal(segment[:6], []byte("Exif\x00\x00")) {
			continue
		}
		t := segment[6:]
		var order binary.ByteOrder
		if string(t[:2]) == "II" {
			order = binary.LittleEndian
		} else if string(t[:2]) == "MM" {
			order = binary.BigEndian
		} else {
			return 1
		}
		if order.Uint16(t[2:4]) != 42 {
			return 1
		}
		pos := uint64(order.Uint32(t[4:8]))
		if pos+2 > uint64(len(t)) {
			return 1
		}
		count := int(order.Uint16(t[pos : pos+2]))
		pos += 2
		for i := 0; i < count && pos+12 <= uint64(len(t)); i++ {
			entry := t[pos : pos+12]
			pos += 12
			if order.Uint16(entry[:2]) == 0x112 && order.Uint16(entry[2:4]) == 3 && order.Uint32(entry[4:8]) == 1 {
				v := int(order.Uint16(entry[8:10]))
				if v >= 1 && v <= 8 {
					return v
				}
				return 1
			}
		}
	}
	return 1
}
func orient(src *image.RGBA, orientation int) *image.RGBA {
	if orientation <= 1 || orientation > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	ow, oh := w, h
	if orientation >= 5 {
		ow, oh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, ow, oh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := x, y
			switch orientation {
			case 2:
				dx = w - 1 - x
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dy = h - 1 - y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.SetRGBA(dx, dy, src.RGBAAt(x, y))
		}
	}
	return dst
}

// ReadPhoto opens an immutable file only after current database authorization.
// kind is a server-selected capability, never a client-controlled field.
func (s *AlbumService) ReadPhoto(ctx context.Context, actor uint64, kind, no, variant string) (*os.File, int64, error) {
	if err := s.ready(); err != nil {
		return nil, 0, err
	}
	if !validPhotoNo(no) || (variant != "full" && variant != "thumb") {
		return nil, 0, ErrInvalid
	}
	db := s.DB.WithContext(ctx)
	var p model.Photo
	if err := db.Where("photo_no=?", no).Take(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = ErrNotFound
		}
		return nil, 0, err
	}
	switch kind {
	case "public":
		if p.Status != model.PhotoApproved {
			return nil, 0, ErrNotFound
		}
		if _, err := VisibleAnchor(ctx, db, actor, p.AnchorNo); err != nil {
			return nil, 0, err
		}
	case "owner":
		if actor != p.OwnerID || p.Status == model.PhotoDeleted {
			return nil, 0, ErrNotFound
		}
		if _, err := ownerAnchor(db, actor); err != nil {
			return nil, 0, err
		}
	case "admin":
		if err := albumAdmin(db, actor); err != nil {
			return nil, 0, err
		}
		if p.Status == model.PhotoDeleted {
			return nil, 0, ErrNotFound
		}
	default:
		return nil, 0, ErrForbidden
	}
	if !storageKeyPattern.MatchString(p.StorageKey) {
		return nil, 0, ErrUnavailable
	}
	f, err := s.root.Open(p.StorageKey + "." + variant + ".jpg")
	if err != nil {
		return nil, 0, ErrUnavailable
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxUploadBytes*2 {
		_ = f.Close()
		return nil, 0, ErrUnavailable
	}
	return f, info.Size(), nil
}

// CleanupOrphans is explicit maintenance, never a request-side rollback handler.
// It removes only generated filename patterns: unreferenced files after 24 hours,
// or owner-deleted photos after 7 days. Metadata/audits remain. Each run scans at
// most 1000 entries; its directory cursor resumes next run to avoid starvation.
func (s *AlbumService) CleanupOrphans(ctx context.Context, olderThan time.Time) (int, error) {
	if err := s.ready(); err != nil {
		return 0, err
	}
	if olderThan.After(time.Now().Add(-24 * time.Hour)) {
		return 0, ErrInvalid
	}
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()
	if s.cleanupDir == nil {
		var err error
		s.cleanupDir, err = s.root.Open(".")
		if err != nil {
			return 0, err
		}
	}
	removed := 0
	for batch := 0; batch < 10; batch++ {
		entries, err := s.cleanupDir.ReadDir(100)
		if err != nil && err != io.EOF {
			_ = s.cleanupDir.Close()
			s.cleanupDir = nil
			return removed, err
		}
		if len(entries) == 0 {
			_ = s.cleanupDir.Close()
			s.cleanupDir = nil
			return removed, nil
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return removed, ctx.Err()
			}
			name := entry.Name()
			key, ok := strings.CutSuffix(name, ".full.jpg")
			if !ok {
				key, ok = strings.CutSuffix(name, ".thumb.jpg")
			}
			if !ok || !storageKeyPattern.MatchString(key) {
				continue
			}
			info, e := entry.Info()
			if e != nil {
				return removed, e
			}
			if !info.Mode().IsRegular() || !info.ModTime().Before(olderThan) {
				continue
			}
			var photo model.Photo
			e = s.DB.WithContext(ctx).Where("storage_key=?", key).Take(&photo).Error
			if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
				return removed, e
			}
			if e == nil && (photo.Status != model.PhotoDeleted || photo.UpdatedAt > time.Now().Add(-7*24*time.Hour).UnixMilli()) {
				continue
			}
			if e := s.root.Remove(name); e != nil && !errors.Is(e, os.ErrNotExist) {
				return removed, e
			}
			removed++
		}
	}
	return removed, nil
}
