package initialize

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"os"
	"path/filepath"
	"strings"
	"tb_live_module/global"
	"tb_live_module/internal/telemetry"
	svc "tb_live_module/service/anchorhome"
	"time"
)

func MigrateAnchorHomeAlbum(db *gorm.DB) error        { return svc.MigrateAlbumSchema(db) }
func ValidateAnchorHomeAlbumSchema(db *gorm.DB) error { return svc.VerifyAlbumSchema(db) }

// InitializeAnchorHomeAlbum performs no DDL. Storage may never overlap a public
// static/upload root; the process must be the only writer of this 0700 directory.
func InitializeAnchorHomeAlbum(db *gorm.DB) error {
	cfg := global.GVA_CONFIG.AnchorHome
	if cfg.Disabled {
		svc.DefaultAlbum = &svc.AlbumService{}
		return nil
	}
	if err := ValidateAnchorHomeAlbumSchema(db); err != nil {
		return err
	}
	storage := cfg.StorageDir
	if storage == "" {
		storage = "private/anchor-home"
	}
	abs, err := filepath.Abs(storage)
	if err != nil {
		return err
	}
	lexicalStorage := abs
	// Check both the configured path and its real target. A symlink placed under
	// public uploads still exposes its target even when that target lies outside.
	if err = os.MkdirAll(abs, 0700); err != nil {
		return err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return err
	}
	for _, public := range []string{global.GVA_CONFIG.Local.StorePath, global.GVA_CONFIG.Local.Path, "uploads", "resource", "assets", publicAdminAssetsDir, publicH5AssetsDir} {
		if public == "" {
			continue
		}
		p, err := filepath.Abs(public)
		if err != nil {
			return err
		}
		roots := []string{p}
		if resolved, e := filepath.EvalSymlinks(p); e == nil {
			roots = append(roots, resolved)
		}
		for _, root := range roots {
			for _, target := range []string{abs, lexicalStorage} {
				rel, err := filepath.Rel(root, target)
				if err != nil {
					return err
				}
				if rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
					return fmt.Errorf("anchor-home storage must be outside public roots")
				}
			}
		}
	}
	prefix := "/" + strings.Trim(global.GVA_CONFIG.System.RouterPrefix, "/")
	prefix = strings.TrimSuffix(prefix, "/")
	album, err := svc.NewAlbumService(db, svc.AlbumOptions{StorageDir: abs, MaxPhotos: cfg.MaxPhotos, MaxUploadsPerDay: cfg.MaxUploadsPerDay, Concurrency: cfg.ImageConcurrency, PublicBase: prefix + "/v1/app/anchor-home", AdminBase: prefix + "/v1/admin/anchor-home"})
	if err != nil {
		return err
	}
	svc.DefaultAlbum = album
	return nil
}

// StartAnchorHomeMediaCleanup reclaims crash orphans and owner-deleted media;
// DB metadata and immutable audits are retained. It never deletes arbitrary paths.
func StartAnchorHomeMediaCleanup(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	if global.GVA_CONFIG.AnchorHome.Disabled {
		close(done)
		return done
	}
	album := svc.DefaultAlbum
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				finish := telemetry.Default.StartWorker("anchor_home_media_cleanup", 15*time.Minute)
				work, cancel := context.WithTimeout(ctx, time.Minute)
				count, err := album.CleanupOrphans(work, time.Now().Add(-24*time.Hour))
				cancel()
				finish(err)
				if global.GVA_LOG != nil {
					if err != nil {
						global.GVA_LOG.Warn("anchor_home_media_cleanup_failed", zap.String("errorCode", svc.ErrorCode(err)))
					} else {
						global.GVA_LOG.Info("anchor_home_media_cleanup_completed", zap.Int("removed", count))
					}
				}
			}
		}
	}()
	return done
}
