package engagementcli

import (
	"context"
	"errors"
	"testing"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
)

func TestBackupErrorKeepsSafeMessageAndClassifiableCause(t *testing.T) {
	secret := &mysqlDriver.MySQLError{Number: 1205, Message: "SQL with private credential"}
	wrapped := &BackupError{cause: secret}
	require.Equal(t, "backup failed before mutation", wrapped.Error())
	var underlying *mysqlDriver.MySQLError
	require.True(t, errors.As(wrapped, &underlying))
	require.Same(t, secret, underlying)
	require.ErrorIs(t, &BackupError{cause: context.Canceled}, context.Canceled)
	_, _, err := BackupAll(nil, "relative")
	require.ErrorAs(t, err, &wrapped)
	require.Equal(t, "backup failed before mutation", err.Error())
}
