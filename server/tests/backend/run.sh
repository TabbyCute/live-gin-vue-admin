#!/usr/bin/env bash
set -euo pipefail
backend_server="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$backend_server"
source tests/backend/local-env.sh
# These dedicated projects are kept running for inspection, never down user services.
for module in wallet gift earning payout social presence; do
  docker compose -f "tests/$module/compose.yaml" up -d --wait
done
docker compose -f tests/earning/compose.yaml exec -T mysql mysql -uroot -pearning-local-test-only -e 'CREATE DATABASE IF NOT EXISTS payout_cli_test CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;'
docker compose -f tests/payout/compose.yaml exec -T mysql mysql -uroot -ppayout-local-test-only -e 'CREATE DATABASE IF NOT EXISTS earning_test_scale CHARACTER SET utf8mb4 COLLATE utf8mb4_bin; CREATE DATABASE IF NOT EXISTS event_test_delivery CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;'
go test -p 2 ./... -count=1
go test -race -p 2 ./service/wallet ./service/gift ./service/guard ./service/ranking ./service/earning ./service/payout ./service/bankreconcile ./service/eventdelivery ./service/social ./service/live ./service/system ./api/v1/... ./router/... ./middleware ./initialize ./core/... ./internal/... -count=1
# The source is a completed earning fixture. Run serially after package tests
# so no earning test can reset it during the read-only snapshot/restore proof.
BACKEND_SCOPE_SOURCE_TEST_DSN="$EARNING_TEST_MYSQL_DSN" go test -race ./internal/backendcli -run '^TestMySQLScopedBackupRealFinancialTriggers$' -count=1 -v
WALLET_RUN_LOAD=1 go test ./service/wallet -run '^TestMySQL(WalletSealLoad|WalletPlansAndLoad|MovementQueryPlans)$' -count=1 -v
GIFT_RUN_LOAD=1 GUARD_RUN_LOAD=1 go test ./service/gift -run '^TestMySQL.*Load$' -count=1 -v
EARNING_RUN_LOAD=1 go test ./service/earning -run '^TestMySQL(FinanceLoad|ManualPayoutLoad)$' -count=1 -v
SOCIAL_RUN_LOAD=1 go test ./service/social ./service/live -run '^Test(MySQLPublicProfilePlansAndBatchLoad|MySQLLoadProfile|RedisHeartbeatLoadProfile)$' -count=1 -v
PRESENCE_LOAD=1 go test ./service/live -run '^TestPresenceLoad' -count=1 -v
for target in 'api/v1/bankreconcile:FuzzBankStrictJSON' 'api/v1/eventdelivery:FuzzRetryBody' 'api/v1/payout:FuzzManualPayoutJSON' 'api/v1/earning:FuzzEarningJSON' 'api/v1/gift:FuzzGiftJSON' 'api/v1/wallet:FuzzWalletJSON' 'api/v1/live:FuzzPresenceJSON' 'service/earning:FuzzManualPaidConservation' 'service/earning:FuzzRoundingConservation' 'service/live:FuzzPresenceCredentials'; do
  go test "./${target%:*}" -run '^$' -fuzz "^${target#*:}$" -fuzztime=5s -parallel=2
done
go vet ./...
go build ./...
bash tests/srs/run-local.sh
cd ../web
node --experimental-vm-modules --test tests/*.test.mjs
pnpm build
