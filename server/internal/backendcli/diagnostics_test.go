package backendcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestCleanupFailurePreservesPrimaryStageAndCause(t *testing.T) {
	secret := &mysqlDriver.MySQLError{Number: 1213, Message: "UPDATE secret_table SET credential='private'"}
	primary := error(secret)
	report := Report{Stage: "backup"}
	recordCleanupFailure(&report, &primary, context.DeadlineExceeded)
	require.Equal(t, "backup", report.Stage)
	require.Same(t, secret, primary)
	require.True(t, report.CleanupFailed)
	require.Equal(t, "timeout", report.CleanupFailureCode)
	require.Equal(t, "mysql_1213", safeFailureCode(primary))
	safe := &stageError{stage: report.Stage, cause: primary}
	require.Equal(t, "backup", safe.Error())
	require.ErrorIs(t, safe, secret)

	primary = nil
	recordCleanupFailure(&report, &primary, context.Canceled)
	require.Equal(t, "migration_lock_release", report.Stage)
	require.Equal(t, "migration_lock_release", primary.Error())
	require.ErrorIs(t, primary, context.Canceled)
}

func TestSafeFailureCodesNeverContainUnderlyingDetails(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "timeout"},
		{fmt.Errorf("private: %w", context.Canceled), "canceled"},
		{&net.OpError{Op: "read", Net: "tcp", Err: os.ErrDeadlineExceeded}, "timeout"},
		{&mysqlDriver.MySQLError{Number: 1205, Message: "SQL contains credential"}, "mysql_1205"},
		{errors.New("private address and SQL"), ""},
	}
	for _, tc := range cases {
		require.Equal(t, tc.want, safeFailureCode(&stageError{stage: "backup", cause: tc.err}))
	}
}

func TestCLIRejectsExcessiveDeadlinesBeforeConfiguration(t *testing.T) {
	for _, args := range [][]string{
		{"-read-timeout", "121s"}, {"-read-timeout", "0s"}, {"-timeout", "16m"}, {"-timeout", "59s"},
		{"-backup-scope", "private-any-table"},
	} {
		var out, progress bytes.Buffer
		require.Equal(t, 1, RunWithProgress(append([]string{"-config", "private-missing.yaml", "-progress"}, args...), &out, &progress))
		var report Report
		require.NoError(t, json.Unmarshal(out.Bytes(), &report))
		require.Equal(t, "arguments", report.Stage)
		require.Empty(t, progress.String())
		require.NotContains(t, out.String(), "private")
	}
}
