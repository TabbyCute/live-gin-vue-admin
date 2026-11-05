#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
# 独立compose项目与本机端口；fixture另核验地址和wallet_test_库名前缀。
trap 'docker compose -f tests/wallet/compose.yaml down --volumes' EXIT
docker compose -f tests/wallet/compose.yaml up -d --wait
export WALLET_TEST_MYSQL_DSN='root:wallet-local-test-only@tcp(127.0.0.1:13326)/wallet_test_seal?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
go test ./... -count=1
go test -race ./service/wallet ./api/v1/wallet ./router/wallet ./initialize ./config ./middleware -count=1
WALLET_TEST_MYSQL_DSN="${WALLET_TEST_MYSQL_DSN}&clientFoundRows=true" go test -race ./service/wallet -run MySQL -count=1
WALLET_RUN_LOAD=1 go test ./service/wallet -run 'TestMySQLWalletSealLoad|TestMySQLWalletPlansAndLoad|TestMySQLMovementQueryPlans' -count=1 -v
go test ./service/wallet -run '^$' -fuzz FuzzAmount -fuzztime=10s -parallel=2
go test ./api/v1/wallet -run '^$' -fuzz FuzzWalletJSON -fuzztime=10s -parallel=2
go vet ./...
go build ./...
if [[ "${WALLET_RUN_VULN:-0}" == "1" ]]; then
  GOTOOLCHAIN=go1.26.10 go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
fi
