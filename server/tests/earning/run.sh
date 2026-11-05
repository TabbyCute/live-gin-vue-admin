#!/usr/bin/env bash
set -euo pipefail
earning_server_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
earning_compose="$earning_server_dir/tests/earning/compose.yaml"
gift_compose="$earning_server_dir/tests/gift/compose.yaml"
cleanup() {
  local status=$?
  trap - EXIT
  if ! docker compose -f "$earning_compose" down --volumes; then status=1; fi
  if ! docker compose -f "$gift_compose" down --volumes; then status=1; fi
  exit "$status"
}
trap cleanup EXIT
cd "$earning_server_dir"
docker compose -f "$earning_compose" up -d --wait
docker compose -f "$gift_compose" up -d --wait
export EARNING_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/earning_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export GIFT_TEST_MYSQL_DSN='root:gift-local-test-only@tcp(127.0.0.1:13327)/gift_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
EARNING_RUN_LOAD=0 go test ./... -count=1
EARNING_RUN_LOAD=0 go test -race ./service/earning ./service/gift ./api/v1/earning ./router/earning -count=1
EARNING_TEST_MYSQL_DSN="${EARNING_TEST_MYSQL_DSN}&clientFoundRows=true" go test ./service/earning -run MySQLFinanceConcurrent -count=1
go test ./api/v1/earning -run '^$' -fuzz FuzzEarningJSON -fuzztime=10s -parallel=2
go test ./service/earning -run '^$' -fuzz FuzzRoundingConservation -fuzztime=10s -parallel=2
go vet ./...
go build ./...
if [[ "${EARNING_RUN_LOAD:-0}" == 1 ]]; then go test ./service/earning -run '^TestMySQLFinanceLoad$' -v -count=1; fi
cd "$earning_server_dir/../web"
node --experimental-vm-modules --test tests/earning.test.mjs tests/settlement-create.test.mjs tests/gift*.test.mjs tests/wallet*.test.mjs
npm run build
