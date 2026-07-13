package inmem_test

import (
	"context"
	"testing"

	"github.com/sam33339999/wikibuild/internal/model"
	"github.com/sam33339999/wikibuild/internal/store"
	"github.com/sam33339999/wikibuild/internal/store/inmem"
	"github.com/stretchr/testify/require"
)

func TestCreateUser_AndGetByUsername(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()

	u, err := repo.CreateUser(ctx, model.User{
		Username:     "admin",
		PasswordHash: "$2a$10$abc",
	})
	require.NoError(t, err)
	require.NotZero(t, u.ID)
	require.Equal(t, "admin", u.Username)

	got, err := repo.GetUserByUsername(ctx, "admin")
	require.NoError(t, err)
	require.Equal(t, u.ID, got.ID)
	require.Equal(t, "admin", got.Username)
	require.Equal(t, "$2a$10$abc", got.PasswordHash)
}

func TestGetUserByUsername_NotFound(t *testing.T) {
	repo := inmem.New()
	_, err := repo.GetUserByUsername(context.Background(), "ghost")
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestCreateUser_DuplicateUsername(t *testing.T) {
	repo := inmem.New()
	ctx := context.Background()

	_, err := repo.CreateUser(ctx, model.User{Username: "admin", PasswordHash: "h1"})
	require.NoError(t, err)

	_, err = repo.CreateUser(ctx, model.User{Username: "admin", PasswordHash: "h2"})
	require.ErrorIs(t, err, store.ErrDuplicateUsername)
}
