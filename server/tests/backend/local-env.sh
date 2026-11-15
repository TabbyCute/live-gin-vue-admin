#!/usr/bin/env bash
# Fixed disposable localhost endpoints only. Never load application YAML here.
export WALLET_TEST_MYSQL_DSN='root:wallet-local-test-only@tcp(127.0.0.1:13326)/wallet_test_seal?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export GIFT_TEST_MYSQL_DSN='root:gift-local-test-only@tcp(127.0.0.1:13327)/gift_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export EARNING_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/earning_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export PAYOUT_TEST_MYSQL_DSN='root:payout-local-test-only@tcp(127.0.0.1:13329)/payout_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export EARNING_SCALE_MYSQL_DSN='root:payout-local-test-only@tcp(127.0.0.1:13329)/earning_test_scale?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export BACKEND_MIGRATION_TEST_DSN="$EARNING_SCALE_MYSQL_DSN"
export EVENT_DELIVERY_MYSQL_DSN='root:payout-local-test-only@tcp(127.0.0.1:13329)/event_test_delivery?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
export SOCIAL_TEST_MYSQL_DSN='root:social_test_only@tcp(127.0.0.1:13316)/social_test_relationship?parseTime=true&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s'
export SOCIAL_TEST_REDIS_ADDR='127.0.0.1:16389'
export PRESENCE_TEST_MYSQL_DSN='root:presence-test-only@tcp(127.0.0.1:15436)/presence_test?parseTime=true&loc=UTC&timeout=5s&readTimeout=10s&writeTimeout=10s'
export PRESENCE_TEST_REDIS='127.0.0.1:18479'
export PAYOUT_CLI_TEST_MYSQL_DSN='root:earning-local-test-only@tcp(127.0.0.1:13328)/payout_cli_test?parseTime=true&timeout=5s&readTimeout=10s&writeTimeout=10s'
unset MCP_TEST_URL EVENT_DELIVERY_BACKUP_MYSQL_DSN SRS_LOCAL_SMOKE BACKEND_SCOPE_SOURCE_TEST_DSN

