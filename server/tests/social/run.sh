#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
trap 'docker compose -f tests/social/compose.yaml down --volumes' EXIT
docker compose -f tests/social/compose.yaml up -d --wait
export SOCIAL_TEST_MYSQL_DSN='root:social_test_only@tcp(127.0.0.1:13316)/social_test_relationship?parseTime=true&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s'
export SOCIAL_TEST_REDIS_ADDR='127.0.0.1:16389'
go test ./service/social ./service/live ./api/v1/social ./api/v1/live ./router/social ./initialize ./model/social -count=1
SOCIAL_RUN_LOAD=1 go test ./service/live -run TestMySQLPublicProfilePlansAndBatchLoad -count=1 -v
SOCIAL_RUN_LOAD=1 go test ./service/social -run 'TestMySQLLoadProfile|TestRedisHeartbeatLoadProfile' -count=1 -v
go test -race ./service/social ./service/live ./router/social ./model/social
go test ./service/social -run '^$' -bench . -benchmem -benchtime=1s
