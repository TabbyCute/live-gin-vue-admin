// Package testlocal contains fail-closed guards for destructive local test fixtures.
// It is imported only by tests, never by production migration commands.
package testlocal

import (
	"errors"
	driver "github.com/go-sql-driver/mysql"
)

// EventDeliveryMySQL must run BEFORE gorm.Open or any possible connection/DDL.
func EventDeliveryMySQL(dsn string) (string, error) {
	cfg, err := driver.ParseDSN(dsn)
	if err != nil || cfg.Net != "tcp" || cfg.Addr != "127.0.0.1:13329" || cfg.DBName != "event_test_delivery" {
		return "", errors.New("event tests require exact isolated local database")
	}
	return cfg.FormatDSN(), nil
}
