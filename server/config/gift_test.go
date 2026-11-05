package config

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGiftGuardFailsClosed(t *testing.T) {
	require.NoError(t, (Gift{}).Validate())
	require.Error(t, (Gift{GuardEnabled: true}).Validate())
}
