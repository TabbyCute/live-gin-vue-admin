#!/usr/bin/env bash
set -euo pipefail
gift_server_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
gift_compose_file="$gift_server_dir/tests/gift/compose.yaml"
cd "$gift_server_dir"
cleanup() {
  local status=$?
  trap - EXIT
  if ! docker compose -f "$gift_compose_file" down --volumes; then
    echo '礼物测试容器清理失败' >&2
    if [[ "$status" -eq 0 ]]; then status=1; fi
  fi
  exit "$status"
}
trap cleanup EXIT
docker compose -f "$gift_compose_file" up -d --wait
export GIFT_TEST_MYSQL_DSN='root:gift-local-test-only@tcp(127.0.0.1:13327)/gift_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
go test ./... -count=1
go test -race ./service/gift ./service/earning ./api/v1/gift ./router/gift -count=1
GIFT_TEST_MYSQL_DSN="${GIFT_TEST_MYSQL_DSN}&clientFoundRows=true" go test ./service/gift -run MySQL -count=1
go test ./api/v1/gift -run '^$' -fuzz FuzzGiftJSON -fuzztime=10s -parallel=2
go vet ./...
go build ./...
cd "$gift_server_dir/../web"
node --experimental-vm-modules --test tests/gift*.test.mjs
npm run build
