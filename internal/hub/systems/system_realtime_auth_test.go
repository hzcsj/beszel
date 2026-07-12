//go:build testing

package systems_test

import (
	"fmt"
	"testing"

	"github.com/henrygd/beszel/internal/hub/systems"
	"github.com/henrygd/beszel/internal/tests"
	"github.com/pocketbase/pocketbase/tools/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sdkTopic builds a subscription string matching PocketBase SDK encoding:
//
//	rt_systems?options={"query":{"system":"abc123"}}
func sdkTopic(prefix, systemId string) string {
	return fmt.Sprintf(`%s?options={"query":{"system":"%s"}}`, prefix, systemId)
}

func TestParseRealtimeSystemID(t *testing.T) {
	t.Run("SDK format", func(t *testing.T) {
		got := systems.TestParseRealtimeSystemID(`rt_systems?options={"query":{"system":"abc123"}}`)
		assert.Equal(t, "abc123", got)
	})
	t.Run("bare query fallback", func(t *testing.T) {
		got := systems.TestParseRealtimeSystemID("rt_metrics?system=xyz")
		assert.Equal(t, "xyz", got)
	})
	t.Run("no query", func(t *testing.T) {
		got := systems.TestParseRealtimeSystemID("rt_systems")
		assert.Equal(t, "", got)
	})
	t.Run("empty options", func(t *testing.T) {
		got := systems.TestParseRealtimeSystemID(`rt_systems?options={}`)
		assert.Equal(t, "", got)
	})
}

func TestRealtimeAuthGuestRejected(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	user, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)

	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"port":  "33914",
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, nil, sdkTopic("rt_systems", sys.Id))
	assert.Error(t, err, "guest should be rejected")
}

func TestRealtimeAuthUnauthorizedUser(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	owner, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)

	outsider, err := tests.CreateUser(hub, "outsider@test.com", "password123")
	require.NoError(t, err)

	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"port":  "33914",
		"users": []string{owner.Id},
	})
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, outsider, sdkTopic("rt_metrics", sys.Id))
	assert.Error(t, err, "unauthorized user should be rejected")
}

func TestRealtimeAuthMemberAllowed(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	owner, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)

	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"port":  "33914",
		"users": []string{owner.Id},
	})
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, owner, sdkTopic("rt_systems", sys.Id))
	assert.NoError(t, err, "member should be allowed")
}

func TestRealtimeAuthReadonlyWithShareAll(t *testing.T) {
	t.Setenv("SHARE_ALL_SYSTEMS", "true")

	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	owner, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)

	roUser, err := tests.CreateUserWithRole(hub, "viewer@test.com", "password123", "readonly")
	require.NoError(t, err)

	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name":  "test-system",
		"host":  "127.0.0.1",
		"port":  "33914",
		"users": []string{owner.Id},
	})
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, roUser, sdkTopic("rt_systems", sys.Id))
	assert.NoError(t, err, "readonly with SHARE_ALL_SYSTEMS should be allowed")

	err = sm.TestRealtimeAuth(hub, roUser, sdkTopic("rt_metrics", sys.Id))
	assert.Error(t, err, "readonly detailed realtime metrics should be rejected")
}

func TestRealtimeAuthMissingSystemParam(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	user, err := tests.CreateUser(hub, "user@test.com", "password123")
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, user, "rt_systems")
	assert.Error(t, err, "missing system param should be rejected")
}

func TestRealtimeAuthNonexistentSystem(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()

	user, err := tests.CreateUser(hub, "user@test.com", "password123")
	require.NoError(t, err)

	err = sm.TestRealtimeAuth(hub, user, sdkTopic("rt_metrics", "nonexistent"))
	assert.Error(t, err, "nonexistent system should be rejected")
}

// --- Hook-level tests: full hook chain with registry/worker/status assertions ---

func apiErrorStatus(err error) int {
	if err == nil {
		return 0
	}
	if ae, ok := err.(*router.ApiError); ok {
		return ae.Status
	}
	return -1
}

func TestHookRejectGuest(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	user, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "s1", "host": "127.0.0.1", "port": "33914",
		"users": []string{user.Id},
	})
	require.NoError(t, err)

	r := sm.TestOnRealtimeSubscribeRequest(hub, nil, []string{sdkTopic("rt_systems", sys.Id)})
	assert.Error(t, r.Err, "guest should be rejected")
	assert.Equal(t, 401, apiErrorStatus(r.Err), "guest → 401")
	assert.Equal(t, 0, r.ClientSubCount, "no client subscriptions")
	assert.Equal(t, 0, r.RegistryCount, "registry unchanged")
	assert.False(t, r.WorkerRunning, "worker not started")
}

func TestHookRejectUnauthorized(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	owner, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)
	outsider, err := tests.CreateUser(hub, "outsider@test.com", "password123")
	require.NoError(t, err)
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "s1", "host": "127.0.0.1", "port": "33914",
		"users": []string{owner.Id},
	})
	require.NoError(t, err)

	r := sm.TestOnRealtimeSubscribeRequest(hub, outsider, []string{sdkTopic("rt_metrics", sys.Id)})
	assert.Error(t, r.Err, "outsider should be rejected")
	assert.Equal(t, 403, apiErrorStatus(r.Err), "outsider → 403")
	assert.Equal(t, 0, r.ClientSubCount, "no client subscriptions")
	assert.Equal(t, 0, r.RegistryCount, "registry unchanged")
	assert.False(t, r.WorkerRunning, "worker not started")
}

func TestHookRejectMissingParam(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	user, err := tests.CreateUser(hub, "user@test.com", "password123")
	require.NoError(t, err)

	r := sm.TestOnRealtimeSubscribeRequest(hub, user, []string{"rt_systems"})
	assert.Error(t, r.Err, "missing param should be rejected")
	assert.Equal(t, 400, apiErrorStatus(r.Err), "missing param → 400")
	assert.Equal(t, 0, r.ClientSubCount, "no client subscriptions")
	assert.Equal(t, 0, r.RegistryCount, "registry unchanged")
	assert.False(t, r.WorkerRunning, "worker not started")
}

func TestHookRejectNonexistentSystem(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	user, err := tests.CreateUser(hub, "user@test.com", "password123")
	require.NoError(t, err)

	r := sm.TestOnRealtimeSubscribeRequest(hub, user, []string{sdkTopic("rt_systems", "nonexistent")})
	assert.Error(t, r.Err, "nonexistent system should be rejected")
	assert.Equal(t, 404, apiErrorStatus(r.Err), "nonexistent → 404")
	assert.Equal(t, 0, r.ClientSubCount, "no client subscriptions")
	assert.Equal(t, 0, r.RegistryCount, "registry unchanged")
	assert.False(t, r.WorkerRunning, "worker not started")
}

func TestHookAcceptMemberRegistryGrows(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	owner, err := tests.CreateUser(hub, "owner@test.com", "password123")
	require.NoError(t, err)
	sys, err := tests.CreateRecord(hub, "systems", map[string]any{
		"name": "s1", "host": "127.0.0.1", "port": "33914",
		"users": []string{owner.Id},
	})
	require.NoError(t, err)

	r := sm.TestOnRealtimeSubscribeRequest(hub, owner, []string{sdkTopic("rt_systems", sys.Id)})
	assert.NoError(t, r.Err, "member should be accepted")
	assert.Greater(t, r.ClientSubCount, 0, "client should have subscriptions after accept")
	assert.Greater(t, r.RegistryCount, 0, "registry should grow after accept")
	assert.True(t, r.WorkerRunning, "worker should start after accept")

	systems.TestResetRegistry()
}

func TestHookAcceptNonCustomTopicNoRegistryEffect(t *testing.T) {
	hub, err := tests.NewTestHub(t.TempDir())
	require.NoError(t, err)
	defer hub.Cleanup()

	sm := hub.GetSystemManager()
	sm.Initialize()
	systems.TestResetRegistry()

	r := sm.TestOnRealtimeSubscribeRequest(hub, nil, []string{"systems"})
	assert.NoError(t, r.Err, "non-custom topic should pass without auth check")
	assert.Greater(t, r.ClientSubCount, 0, "client subscriptions should be registered by terminal handler")
	assert.Equal(t, 0, r.RegistryCount, "non-custom topic should not add registry entries")
	assert.False(t, r.WorkerRunning, "worker should not start for non-custom topic")
}
