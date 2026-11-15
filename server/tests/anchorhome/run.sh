#!/usr/bin/env bash
set -euo pipefail
anchorhome_server="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$anchorhome_server"

# Fixed, disposable loopback project. Never read deployment YAML or inherit DSNs.
docker compose -f tests/anchorhome/compose.yaml up -d --wait
docker compose -f tests/anchorhome/compose.yaml exec -T -e MYSQL_PWD=anchor-home-local-only mysql mysql -uroot -e '
CREATE DATABASE IF NOT EXISTS anchor_home_test_album CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
CREATE DATABASE IF NOT EXISTS anchor_home_test_public CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
CREATE DATABASE IF NOT EXISTS anchor_home_test_migration CHARACTER SET utf8mb4 COLLATE utf8mb4_bin;
'
export ANCHOR_HOME_ALBUM_MYSQL_DSN='root:anchor-home-local-only@tcp(127.0.0.1:13339)/anchor_home_test_album?charset=utf8mb4&parseTime=true&loc=UTC'
export ANCHOR_HOME_PUBLIC_MYSQL_DSN='root:anchor-home-local-only@tcp(127.0.0.1:13339)/anchor_home_test_public?charset=utf8mb4&parseTime=true&loc=UTC'
export ANCHOR_HOME_MIGRATION_TEST_DSN='root:anchor-home-local-only@tcp(127.0.0.1:13339)/anchor_home_test_migration?charset=utf8mb4&parseTime=true&loc=UTC'
# Explicit opt-in browser fixtures must not turn the ordinary runner into a server.
unset H5_ANCHOR_HOME_BROWSER
go test -race -p 2 ./service/anchorhome ./api/v1/anchorhome ./router/anchorhome ./internal/anchorhomecli ./initialize ./middleware ./internal/telemetry -count=1
go test ./service/anchorhome -run '^$' -fuzz '^FuzzJPEGOrientation$' -fuzztime=5s -parallel=2

# Keep only this project available for inspection. Remove it when finished with:
# docker compose -f tests/anchorhome/compose.yaml down
