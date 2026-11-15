#!/usr/bin/env bash
set -euo pipefail
presence_test_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
presence_server_dir="$(cd -- "$presence_test_dir/../.." && pwd)"
presence_project="presence-verify-$$"
presence_compose=(docker compose -p "$presence_project" -f "$presence_test_dir/compose.yaml")
cleanup() { "${presence_compose[@]}" down --volumes --remove-orphans >/dev/null 2>&1 || true; }
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
"${presence_compose[@]}" up -d --wait --wait-timeout 60
export PRESENCE_TEST_REDIS="127.0.0.1:${PRESENCE_REDIS_PORT:-18479}"
export PRESENCE_TEST_MYSQL_DSN="root:presence-test-only@tcp(127.0.0.1:${PRESENCE_MYSQL_PORT:-15436})/presence_test?parseTime=true&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s"
cd -- "$presence_server_dir"
go test ./... -count=1
go test -race ./service/live ./api/v1/live ./router/live ./middleware -count=1
go test ./service/live -run '^$' -fuzz '^FuzzPresenceCredentials$' -fuzztime=10s -parallel=2
go test ./api/v1/live -run '^$' -fuzz '^FuzzPresenceJSON$' -fuzztime=10s -parallel=2
PRESENCE_LOAD=1 go test ./service/live -run '^TestPresenceLoad' -count=1 -v
go vet ./...
go build ./...
