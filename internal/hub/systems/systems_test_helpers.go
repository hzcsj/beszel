//go:build testing

package systems

import (
	"context"
	"fmt"

	entities "github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// The hub integration tests create/replace systems and cleanup the test apps quickly.
// Background SMART fetching can outlive teardown and crash in PocketBase internals (nil DB).
//
// We keep the explicit SMART refresh endpoint / method available, but disable
// the automatic background fetch during tests.
func backgroundSmartFetchEnabled() bool { return false }

// TESTING ONLY: GetSystemCount returns the number of systems in the store
func (sm *SystemManager) GetSystemCount() int {
	return sm.systems.Length()
}

// TESTING ONLY: HasSystem checks if a system with the given ID exists in the store
func (sm *SystemManager) HasSystem(systemID string) bool {
	return sm.systems.Has(systemID)
}

// TESTING ONLY: GetSystemStatusFromStore returns the status of a system with the given ID
// Returns an empty string if the system doesn't exist
func (sm *SystemManager) GetSystemStatusFromStore(systemID string) string {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return ""
	}
	return sys.Status
}

// TESTING ONLY: GetSystemContextFromStore returns the context and cancel function for a system
func (sm *SystemManager) GetSystemContextFromStore(systemID string) (context.Context, context.CancelFunc, error) {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return nil, nil, fmt.Errorf("no system")
	}
	return sys.ctx, sys.cancel, nil
}

// TESTING ONLY: GetSystemFromStore returns a store from the system
func (sm *SystemManager) GetSystemFromStore(systemID string) (*System, error) {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return nil, fmt.Errorf("no system")
	}
	return sys, nil
}

// TESTING ONLY: GetAllSystemIDs returns a slice of all system IDs in the store
func (sm *SystemManager) GetAllSystemIDs() []string {
	data := sm.systems.GetAll()
	ids := make([]string, 0, len(data))
	for id := range data {
		ids = append(ids, id)
	}
	return ids
}

// TESTING ONLY: GetSystemData returns the combined data for a system with the given ID
// Returns nil if the system doesn't exist
// This method is intended for testing
func (sm *SystemManager) GetSystemData(systemID string) *entities.CombinedData {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return nil
	}
	return sys.data
}

// TESTING ONLY: GetSystemHostPort returns the host and port for a system with the given ID
// Returns empty strings if the system doesn't exist
func (sm *SystemManager) GetSystemHostPort(systemID string) (string, string) {
	sys, ok := sm.systems.GetOk(systemID)
	if !ok {
		return "", ""
	}
	return sys.Host, sys.Port
}

// TESTING ONLY: SetSystemStatusInDB sets the status of a system directly and updates the database record
// This is intended for testing
// Returns false if the system doesn't exist
func (sm *SystemManager) SetSystemStatusInDB(systemID string, status string) bool {
	if !sm.HasSystem(systemID) {
		return false
	}

	// Update the database record
	record, err := sm.hub.FindRecordById("systems", systemID)
	if err != nil {
		return false
	}

	record.Set("status", status)
	err = sm.hub.Save(record)
	if err != nil {
		return false
	}

	return true
}

// TESTING ONLY: RemoveAllSystems removes all systems from the store
func (sm *SystemManager) RemoveAllSystems() {
	for _, system := range sm.systems.GetAll() {
		sm.RemoveSystem(system.Id)
	}
	sm.smartFetchMap.StopCleaner()
}

func (s *System) StopUpdater() {
	s.cancel()
}

func (s *System) CreateRecords(data *entities.CombinedData) (*core.Record, error) {
	s.data = data
	return s.createRecords(data)
}

// TestRealtimeAuth exercises the production authorizeCustomSubscription function.
// subscription should use the real PocketBase SDK format, e.g.:
//
//	rt_systems?options={"query":{"system":"abc123"}}
//
// For non-custom topics (no rt_ prefix), returns nil without checking auth.
func (sm *SystemManager) TestRealtimeAuth(app core.App, auth *core.Record, subscription string) error {
	return sm.authorizeCustomSubscription(app, auth, subscription)
}

// TestParseRealtimeSystemID exposes the production parser for tests.
func TestParseRealtimeSystemID(subscription string) string {
	return parseRealtimeSystemID(subscription)
}

// TestRegistryEntryCount returns the total number of subscription entries
// across all systems in the global realtime registry.
func TestRegistryEntryCount() int {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	total := 0
	for _, entries := range registry.entries {
		total += len(entries)
	}
	return total
}

// TestRegistryWorkerRunning returns whether the realtime worker is running.
func TestRegistryWorkerRunning() bool {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.worker != nil && registry.worker.running
}

// TestResetRegistry clears the global registry for test isolation.
// It waits for the worker goroutine to fully exit before returning,
// preventing races with hub cleanup.
func TestResetRegistry() {
	registry.mu.Lock()
	var workerDone chan struct{}
	if registry.worker != nil && registry.worker.running {
		close(registry.worker.stopChan)
		registry.worker.running = false
		workerDone = registry.worker.done
	}
	registry.worker = nil
	registry.entries = make(map[string][]*subscriptionEntry)
	registry.mu.Unlock()
	if workerDone != nil {
		<-workerDone
	}
}

// TestHookResult captures the full result of a hook-level realtime subscribe test.
type TestHookResult struct {
	Err            error
	ClientSubCount int
	RegistryCount  int
	WorkerRunning  bool
}

// TestOnRealtimeSubscribeRequest exercises the full onRealtimeSubscribeRequest
// handler through a proper hook chain. A terminal handler simulates PocketBase's
// subscription registration (subscribing the client), so that the post-Next()
// registry logic in onRealtimeSubscribeRequest actually observes new subscriptions.
//
// Returns a TestHookResult with error, client subscription count, registry count
// and worker state — allowing callers to assert all side effects.
func (sm *SystemManager) TestOnRealtimeSubscribeRequest(app core.App, auth *core.Record, subs []string) *TestHookResult {
	client := subscriptions.NewDefaultClient()
	reqEvent := &core.RequestEvent{
		App:  app,
		Auth: auth,
	}
	e := &core.RealtimeSubscribeRequestEvent{
		RequestEvent:  reqEvent,
		Client:        client,
		Subscriptions: subs,
	}

	h := &hook.Hook[*core.RealtimeSubscribeRequestEvent]{}
	h.BindFunc(sm.onRealtimeSubscribeRequest)
	h.BindFunc(func(e *core.RealtimeSubscribeRequestEvent) error {
		for _, sub := range e.Subscriptions {
			e.Client.Subscribe(sub)
		}
		return e.Next()
	})

	err := h.Trigger(e)

	return &TestHookResult{
		Err:            err,
		ClientSubCount: len(client.Subscriptions()),
		RegistryCount:  TestRegistryEntryCount(),
		WorkerRunning:  TestRegistryWorkerRunning(),
	}
}
