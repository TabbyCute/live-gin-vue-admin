package wallet_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	req "tb_live_module/model/wallet/request"
)

// 显式启用，在受限本机MySQL测试库运行。记录真实写入和最终全量对账，非生产容量承诺。
func TestMySQLWalletSealLoad(t *testing.T) {
	if os.Getenv("WALLET_RUN_LOAD") != "1" {
		t.Skip("set WALLET_RUN_LOAD=1 for bounded write load")
	}
	s := mysqlFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	const accounts = 16
	for i := 0; i < accounts; i++ {
		id := 10 + i
		account := strconv.Itoa(10000000 + id)
		if i > 1 {
			require.NoError(t, s.DB.Exec("INSERT INTO live_account(id,account_no,status) VALUES (?,?,1)", id, account).Error)
		}
		r := grant(fmt.Sprintf("load_fund_%d", id))
		r.AccountNo = account
		r.Amount = "10000"
		o, err := s.Create(ctx, creator, r)
		require.NoError(t, err)
		_, err = s.Approve(ctx, reviewer, review(o.OrderNo))
		require.NoError(t, err)
	}
	pool, err := s.DB.DB()
	require.NoError(t, err)
	type profile struct {
		Name        string  `json:"name"`
		Requests    int     `json:"requests"`
		Concurrency int     `json:"concurrency"`
		Errors      int     `json:"errors"`
		Seconds     float64 `json:"seconds"`
		QPS         float64 `json:"qps"`
		P50         float64 `json:"p50Ms"`
		P95         float64 `json:"p95Ms"`
		P99         float64 `json:"p99Ms"`
		PoolWaits   int64   `json:"poolWaits"`
	}
	profiles := []profile{}
	run := func(name string, n, concurrency int, fn func(int) error) {
		durations := make([]time.Duration, n)
		errs := make([]error, n)
		var next atomic.Int64
		var wg sync.WaitGroup
		before := pool.Stats()
		start := time.Now()
		for k := 0; k < concurrency; k++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					i := int(next.Add(1) - 1)
					if i >= n {
						return
					}
					begin := time.Now()
					errs[i] = fn(i)
					durations[i] = time.Since(begin)
				}
			}()
		}
		wg.Wait()
		elapsed := time.Since(start)
		errors := 0
		var first error
		for _, e := range errs {
			if e != nil {
				errors++
				if first == nil {
					first = e
				}
			}
		}
		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
		ms := func(p int) float64 { return float64(durations[(n-1)*p/100]) / float64(time.Millisecond) }
		profiles = append(profiles, profile{Name: name, Requests: n, Concurrency: concurrency, Errors: errors, Seconds: elapsed.Seconds(), QPS: float64(n) / elapsed.Seconds(), P50: ms(50), P95: ms(95), P99: ms(99), PoolWaits: pool.Stats().WaitCount - before.WaitCount})
		t.Logf("%s: %.1f ops/s, p95 %.2f ms, p99 %.2f ms, errors %d", name, float64(n)/elapsed.Seconds(), ms(95), ms(99), errors)
		require.NoError(t, first)
	}
	run("single_wallet_debit", 512, 32, func(i int) error { _, err := s.Debit(ctx, movement(fmt.Sprintf("single_%d", i), "1")); return err })
	run("sixteen_wallet_debit", 512, 32, func(i int) error {
		r := movement(fmt.Sprintf("distributed_%d", i), "1")
		r.AccountID = uint64(10 + i%accounts)
		r.ActorID = strconv.FormatUint(r.AccountID, 10)
		_, err := s.Debit(ctx, r)
		return err
	})
	d, err := s.Debit(ctx, movement("load_refund_parent", "256"))
	require.NoError(t, err)
	run("same_original_partial_refund", 128, 32, func(i int) error {
		r := movement(fmt.Sprintf("load_refund_%d", i), "1")
		r.OriginalNo = d.TransactionNo
		_, err := s.Refund(ctx, r)
		return err
	})
	run("single_wallet_idempotent_replay", 256, 32, func(i int) error { _, err := s.Debit(ctx, movement("load_refund_parent", "256")); return err })
	run("balance_and_ledger", 512, 32, func(i int) error {
		id := uint64(10 + i%accounts)
		if _, err := s.Balance(ctx, id, ""); err != nil {
			return err
		}
		_, err := s.Ledger(ctx, id, req.LedgerSearch{Limit: 20})
		return err
	})
	// 独立算术预期+三游标完整扫描；每批64，验证跨页，不能只看尾余额。
	checked := 0
	for i := 0; i < accounts; i++ {
		want := int64(10000 - 32)
		if i == 0 {
			want -= 512 + 256 - 128
		}
		b, err := s.Balance(ctx, uint64(10+i), "")
		require.NoError(t, err)
		require.Equal(t, strconv.FormatInt(want, 10), b.AvailableBalance)
		q := req.CheckSearch{AccountQuery: req.AccountQuery{AccountNo: strconv.Itoa(10000010 + i)}, Limit: 64}
		for page := 0; page < 100; page++ {
			c, err := s.Check(ctx, creator, q)
			require.NoError(t, err)
			require.True(t, c.Consistent, c.Issues)
			checked += c.CheckedTransactions
			if !c.HasMore {
				break
			}
			require.Less(t, page, 99, "cursor did not terminate")
			q.AfterVersion = c.NextVersion
			q.AfterOrderID = c.NextOrderID
			q.AfterTransactionID = c.NextTransactionID
		}
	}
	require.Equal(t, 1153, checked)
	var mysqlVersion string
	require.NoError(t, s.DB.Raw("SELECT VERSION()").Scan(&mysqlVersion).Error)
	result := map[string]any{"go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "cpus": runtime.NumCPU(), "mysql": mysqlVersion, "maxOpenConnections": 24, "checkedTransactions": checked, "profiles": profiles, "scope": "isolated local service/MySQL sample; not HTTP or production capacity; sync_binlog=1 innodb_flush_log_at_trx_commit=1"}
	data, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	if path := os.Getenv("WALLET_LOAD_RESULT"); path != "" {
		require.NoError(t, os.WriteFile(path, append(data, '\n'), 0600))
	}
}
