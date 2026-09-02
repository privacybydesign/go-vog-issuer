package main

import (
	"testing"
	"time"

	"go-vog-issuer/vog"

	"github.com/stretchr/testify/require"
)

func TestInMemorySessionStorageRoundTrip(t *testing.T) {
	storage := NewInMemorySessionStorage()

	session := &Session{
		Id:             "abc",
		CreatedAt:      time.Now(),
		Stage:          StageValidated,
		Document:       testVogDocument(),
		ValidationCode: vog.CodeAuthentic,
	}
	require.NoError(t, storage.Store(session))

	got, err := storage.Retrieve("abc")
	require.NoError(t, err)
	require.Equal(t, session.Id, got.Id)
	require.Equal(t, StageValidated, got.Stage)
	require.Equal(t, "9999012026032500922", got.Document.ReferenceNumber)

	// The stored copy is independent from the caller's struct.
	session.Stage = StageDisclosing
	got, err = storage.Retrieve("abc")
	require.NoError(t, err)
	require.Equal(t, StageValidated, got.Stage)

	// Updating replaces.
	session.IrmaToken = "token"
	require.NoError(t, storage.Store(session))
	got, err = storage.Retrieve("abc")
	require.NoError(t, err)
	require.Equal(t, StageDisclosing, got.Stage)
	require.Equal(t, "token", got.IrmaToken)

	require.NoError(t, storage.Remove("abc"))
	_, err = storage.Retrieve("abc")
	require.Error(t, err)
	require.Error(t, storage.Remove("abc"))
}

func TestInMemorySessionStorageExpiry(t *testing.T) {
	storage := NewInMemorySessionStorage()
	now := time.Now()
	storage.now = func() time.Time { return now }

	require.NoError(t, storage.Store(&Session{Id: "old", CreatedAt: now}))

	now = now.Add(SessionLifetime + time.Second)
	_, err := storage.Retrieve("old")
	require.Error(t, err, "expired sessions are not returned")
}
