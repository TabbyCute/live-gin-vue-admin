#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
trap 'docker compose -f tests/social/compose.yaml down --volumes' EXIT
docker compose -f tests/social/compose.yaml up -d --wait
export SOCIAL_TEST_MYSQL_DSN='root:social_test_only@tcp(127.0.0.1:13316)/social_test_relationship?parseTime=true&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s'
export SOCIAL_TEST_REDIS_ADDR='127.0.0.1:16389'
go test ./service/social ./service/live ./api/v1/social ./api/v1/live ./router/social ./initialize ./initialize/internal ./model/social ./middleware -count=1
SOCIAL_RUN_LOAD=1 go test ./service/live -run TestMySQLPublicProfilePlansAndBatchLoad -count=1 -v
SOCIAL_RUN_LOAD=1 go test ./service/social -run 'TestMySQLLoadProfile|TestRedisHeartbeatLoadProfile' -count=1 -v
go test -race ./service/social ./service/live ./api/v1/social ./router/social ./model/social ./initialize ./initialize/internal ./middleware
go test ./service/social -run '^$' -bench . -benchmem -benchtime=1s
go vet ./service/social ./service/live ./api/v1/social ./api/v1/live ./router/social ./initialize ./initialize/internal ./model/social ./repository/social ./middleware
go build ./...
if [[ "${SOCIAL_RUN_VULN:-0}" == "1" ]]; then
  GOTOOLCHAIN=go1.26.10 go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
fi
