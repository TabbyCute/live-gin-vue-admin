#!/usr/bin/env bash
set -euo pipefail
payout_server_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cleanup() {
  local status=$?
  trap - EXIT
  for module in payout earning gift; do
    if ! docker compose -f "$payout_server_dir/tests/$module/compose.yaml" down --volumes; then status=1; fi
  done
  exit "$status"
}
trap cleanup EXIT
cd "$payout_server_dir"
for module in payout earning gift; do docker compose -f "tests/$module/compose.yaml" up -d --wait; done
export PAYOUT_TEST_MYSQL_DSN='root:payout-local-test-only@tcp(127.0.0.1:13329)/payout_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export EARNING_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/earning_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export GIFT_TEST_MYSQL_DSN='root:gift-local-test-only@tcp(127.0.0.1:13327)/gift_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
docker compose -f tests/earning/compose.yaml exec -T mysql mysql -uroot -pearning-local-test-only -e 'CREATE DATABASE IF NOT EXISTS payout_cli_test CHARACTER SET utf8mb4 COLLATE utf8mb4_bin'
export PAYOUT_CLI_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/payout_cli_test?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
EARNING_RUN_LOAD=0 go test ./... -count=1
EARNING_RUN_LOAD=0 go test -race ./service/payout ./service/earning ./api/v1/payout ./api/v1/earning ./router/payout ./router/earning ./middleware -count=1
PAYOUT_TEST_MYSQL_DSN="${PAYOUT_TEST_MYSQL_DSN}&clientFoundRows=true" go test ./service/payout -run MySQLPayeeConcurrent -count=1
go test ./api/v1/payout -run '^$' -fuzz FuzzPayeeJSON -fuzztime=10s -parallel=2
go test ./api/v1/payout -run '^$' -fuzz FuzzOrderJSON -fuzztime=10s -parallel=2
go test ./api/v1/payout -run '^$' -fuzz FuzzManualPayoutJSON -fuzztime=10s -parallel=2
go test ./service/payout -run '^$' -fuzz FuzzManualEvidenceAndReference -fuzztime=10s -parallel=2
go test ./service/earning -run '^$' -fuzz FuzzManualPaidConservation -fuzztime=10s -parallel=2
go test ./service/payout -run '^$' -fuzz FuzzPayeeValidation -fuzztime=10s -parallel=2
go test ./api/v1/earning -run '^$' -fuzz FuzzEarningJSON -fuzztime=10s -parallel=2
go vet ./...
go build ./...
EARNING_RUN_LOAD=1 go test ./service/earning -run '^TestMySQL(FinanceLoad|ManualPayoutLoad)$' -count=1 -v
cd "$payout_server_dir/../web"
# The local Node24 VM harness crashes with JIT; HTTP tests still need WebAssembly.
node --jitless --experimental-vm-modules --test tests/earning.test.mjs tests/settlement-create.test.mjs tests/payout-order.test.mjs tests/payout-manual.test.mjs
node --experimental-vm-modules --test tests/gift*.test.mjs tests/wallet*.test.mjs tests/root-admin.test.mjs
pnpm run build
if [[ -n "${PAYOUT_H5_DIR:-}" ]]; then
  cd "$PAYOUT_H5_DIR"
  pnpm test
  pnpm run build
fi
