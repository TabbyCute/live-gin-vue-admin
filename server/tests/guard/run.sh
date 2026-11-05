#!/usr/bin/env bash
set -euo pipefail
guard_server_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cleanup() {
  local status=$?
  trap - EXIT
  for module in gift wallet earning payout; do
    if ! docker compose -f "$guard_server_dir/tests/$module/compose.yaml" down --volumes; then status=1; fi
  done
  exit "$status"
}
trap cleanup EXIT
cd "$guard_server_dir"
for module in gift wallet earning payout; do docker compose -f "tests/$module/compose.yaml" up -d --wait; done
export GIFT_TEST_MYSQL_DSN='root:gift-local-test-only@tcp(127.0.0.1:13327)/gift_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export WALLET_TEST_MYSQL_DSN='root:wallet-local-test-only@tcp(127.0.0.1:13326)/wallet_test_seal?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export EARNING_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/earning_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export PAYOUT_TEST_MYSQL_DSN='root:payout-local-test-only@tcp(127.0.0.1:13329)/payout_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
go test ./... -count=1
GUARD_RUN_LOAD=1 go test ./service/gift -run '^TestMySQLGuardRankingLoad$' -count=1 -v
go test -race ./service/guard ./service/ranking ./service/gift ./service/wallet ./service/earning ./service/payout ./internal/httpkit ./router/guard ./router/ranking -count=1
go test ./service/guard -run '^$' -fuzz FuzzReplay -fuzztime=10s -parallel=2
go test ./api/v1/gift -run '^$' -fuzz FuzzGiftJSON -fuzztime=10s -parallel=2
go vet ./...
go build ./...
cd "$guard_server_dir/../web"
node --experimental-vm-modules --test tests/guard-ranking.test.mjs tests/gift*.test.mjs tests/wallet*.test.mjs tests/earning.test.mjs tests/settlement-create.test.mjs tests/payout*.test.mjs tests/root-admin.test.mjs
npm run build
