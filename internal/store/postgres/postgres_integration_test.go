//go:build integration

package postgres

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContainerStarts(t *testing.T) {
	pool, _ := StartPostgres(t)

	var got int
	err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&got)
	require.NoError(t, err)
	require.Equal(t, 1, got)
}
