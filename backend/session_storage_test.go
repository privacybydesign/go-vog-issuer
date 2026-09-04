package main

import (
	"testing"
	"time"

	"go-vog-issuer/vog"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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

func newTestRedisStorage(t *testing.T) (*RedisSessionStorage, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return NewRedisSessionStorage(client, "test"), mr
}

func TestRedisSessionStorageRoundTrip(t *testing.T) {
	storage, _ := newTestRedisStorage(t)

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
	require.Equal(t, StageValidated, got.Stage)
	require.Equal(t, "9999012026032500922", got.Document.ReferenceNumber)

	require.NoError(t, storage.Remove("abc"))
	_, err = storage.Retrieve("abc")
	require.Error(t, err)
	require.Error(t, storage.Remove("abc"))
}

func TestRedisSessionStorageTTLTracksCreatedAt(t *testing.T) {
	storage, mr := newTestRedisStorage(t)

	createdAt := time.Now().Add(-45 * time.Minute)
	session := &Session{Id: "abc", CreatedAt: createdAt, Stage: StageValidated}
	require.NoError(t, storage.Store(session))

	ttl := mr.TTL(storage.key("abc"))
	require.InDelta(t, 15*time.Minute, ttl, float64(2*time.Second),
		"TTL should reflect what's left of the one hour session lifetime")

	// A retry (e.g. resetting a spent disclosure) must not push the TTL back
	// out to a full hour.
	session.Stage = StageDisclosing
	require.NoError(t, storage.Store(session))

	ttl = mr.TTL(storage.key("abc"))
	require.InDelta(t, 15*time.Minute, ttl, float64(2*time.Second),
		"retrying must not reset the TTL to a full session lifetime")
}

func TestRedisSessionStorageRejectsAlreadyExpiredSession(t *testing.T) {
	storage, _ := newTestRedisStorage(t)

	session := &Session{Id: "abc", CreatedAt: time.Now().Add(-2 * time.Hour)}
	require.Error(t, storage.Store(session))
}
