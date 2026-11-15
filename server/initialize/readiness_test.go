package initialize

import (
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http/httptest"
	"tb_live_module/global"
	"tb_live_module/internal/telemetry"
	"testing"
)

func TestReadinessFailsClosedAndNeverMigrates(t *testing.T) {
	oldDB, oldRedis, oldRequired := global.GVA_DB, global.GVA_REDIS, global.GVA_CONFIG.System.UseRedis
	t.Cleanup(func() {
		global.GVA_DB, global.GVA_REDIS, global.GVA_CONFIG.System.UseRedis = oldDB, oldRedis, oldRequired
	})
	r := gin.New()
	r.GET("/ready", readiness)
	probe := func(status int) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", "/ready", nil))
		require.Equal(t, status, w.Code)
		require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
		require.NotContains(t, w.Body.String(), "password")
	}
	global.GVA_DB = nil
	probe(503)
	db, e := gorm.Open(sqlite.Open("file:"+t.TempDir()+"/ready.db"), &gorm.Config{})
	require.NoError(t, e)
	pool, e := db.DB()
	require.NoError(t, e)
	t.Cleanup(func() { pool.Close() })
	global.GVA_DB = db
	global.GVA_CONFIG.System.UseRedis = false
	probe(200)
	telemetry.Draining.Store(true)
	t.Cleanup(func() { telemetry.Draining.Store(false) })
	probe(503)
	telemetry.Draining.Store(false)
	probe(200)
	global.GVA_CONFIG.System.UseRedis = true
	global.GVA_REDIS = nil
	probe(503)
	global.GVA_CONFIG.System.UseRedis = false
	require.NoError(t, pool.Close())
	probe(503)
}
