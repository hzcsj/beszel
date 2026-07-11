package systems

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/common"
	"github.com/henrygd/beszel/internal/entities/system"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

type subscriptionKind uint8

const (
	kindDetail subscriptionKind = iota
	kindList
)

type subscriptionEntry struct {
	subscription     string
	kind             subscriptionKind
	connectedClients uint8
}

type realtimeRegistry struct {
	mu      sync.Mutex
	entries map[string][]*subscriptionEntry // systemID → list of subscription entries
	worker  *realtimeWorker
}

type realtimeWorker struct {
	ticker   *time.Ticker
	stopChan chan struct{}
	done     chan struct{} // closed when goroutine exits
	running  bool
}

var registry = &realtimeRegistry{
	entries: make(map[string][]*subscriptionEntry),
}

func (sm *SystemManager) onRealtimeConnectRequest(e *core.RealtimeConnectRequestEvent) error {
	e.Next()
	subs := e.Client.Subscriptions()
	for k := range subs {
		sm.removeRealtimeSubscription(k, subs[k])
	}
	return nil
}

// parseRealtimeSystemID extracts the system ID from a PocketBase realtime
// subscription topic. The SDK encodes options as JSON:
//
//	rt_systems?options={"query":{"system":"abc123"}}
func parseRealtimeSystemID(subscription string) string {
	idx := strings.IndexByte(subscription, '?')
	if idx < 0 {
		return ""
	}
	q, err := url.ParseQuery(subscription[idx+1:])
	if err != nil {
		return ""
	}
	if optJSON := q.Get("options"); optJSON != "" {
		var opts struct {
			Query map[string]string `json:"query"`
		}
		if json.Unmarshal([]byte(optJSON), &opts) == nil {
			if id := opts.Query["system"]; id != "" {
				return id
			}
		}
	}
	return q.Get("system")
}

// authorizeCustomSubscription validates auth and system access for a single
// custom rt_systems/rt_metrics subscription. Returns nil if authorized.
func (sm *SystemManager) authorizeCustomSubscription(app core.App, auth *core.Record, subscription string) error {
	if auth == nil {
		return fmt.Errorf("unauthorized")
	}
	systemId := parseRealtimeSystemID(subscription)
	if systemId == "" {
		return fmt.Errorf("missing or malformed system parameter")
	}
	sys, ok := sm.systems.GetOk(systemId)
	if !ok {
		return fmt.Errorf("system not found: %s", systemId)
	}
	if !sys.HasUser(app, auth) {
		return fmt.Errorf("forbidden: no access to system %s", systemId)
	}
	return nil
}

// authorizeRealtimeSubscriptions validates auth and system access for custom
// rt_systems/rt_metrics topics before they are registered. It delegates to
// authorizeCustomSubscription and translates errors into PocketBase API errors.
func (sm *SystemManager) authorizeRealtimeSubscriptions(e *core.RealtimeSubscribeRequestEvent) error {
	for _, sub := range e.Subscriptions {
		if !strings.HasPrefix(sub, "rt_systems") && !strings.HasPrefix(sub, "rt_metrics") {
			continue
		}
		if err := sm.authorizeCustomSubscription(e.App, e.Auth, sub); err != nil {
			msg := err.Error()
			switch {
			case strings.HasPrefix(msg, "unauthorized"):
				return e.UnauthorizedError("The request requires valid record authorization token.", nil)
			case strings.HasPrefix(msg, "missing"):
				return e.BadRequestError("Missing or malformed system parameter.", nil)
			case strings.HasPrefix(msg, "system not found"):
				return e.NotFoundError("The requested resource wasn't found.", nil)
			default:
				return e.ForbiddenError("The authorized record is not allowed to perform this action.", nil)
			}
		}
	}
	return nil
}

func (sm *SystemManager) onRealtimeSubscribeRequest(e *core.RealtimeSubscribeRequestEvent) error {
	if err := sm.authorizeRealtimeSubscriptions(e); err != nil {
		return err
	}

	oldSubs := e.Client.Subscriptions()
	err := e.Next()
	newSubs := e.Client.Subscriptions()

	for k, options := range newSubs {
		if _, ok := oldSubs[k]; ok {
			continue
		}
		var kind subscriptionKind
		if strings.HasPrefix(k, "rt_systems") {
			kind = kindList
		} else if strings.HasPrefix(k, "rt_metrics") {
			kind = kindDetail
		} else {
			continue
		}
		systemId := options.Query["system"]
		if systemId == "" {
			continue
		}
		registry.mu.Lock()
		entries := registry.entries[systemId]
		found := false
		for _, e := range entries {
			if e.subscription == k && e.kind == kind {
				e.connectedClients++
				found = true
				break
			}
		}
		if !found {
			registry.entries[systemId] = append(entries, &subscriptionEntry{
				subscription:     k,
				kind:             kind,
				connectedClients: 1,
			})
		}
		registry.mu.Unlock()
		sm.ensureWorkerRunning()
	}

	for k := range oldSubs {
		if _, ok := newSubs[k]; !ok {
			sm.removeRealtimeSubscription(k, oldSubs[k])
		}
	}
	return err
}

func (sm *SystemManager) removeRealtimeSubscription(subscription string, options subscriptions.SubscriptionOptions) {
	if !strings.HasPrefix(subscription, "rt_metrics") && !strings.HasPrefix(subscription, "rt_systems") {
		return
	}
	systemId := options.Query["system"]
	if systemId == "" {
		return
	}
	registry.mu.Lock()
	entries := registry.entries[systemId]
	for i, e := range entries {
		if e.subscription == subscription {
			e.connectedClients--
			if e.connectedClients <= 0 {
				registry.entries[systemId] = append(entries[:i], entries[i+1:]...)
				if len(registry.entries[systemId]) == 0 {
					delete(registry.entries, systemId)
				}
			}
			break
		}
	}
	registry.mu.Unlock()
	sm.checkWorkerStop()
}

func (sm *SystemManager) ensureWorkerRunning() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.worker != nil && registry.worker.running {
		return
	}
	w := &realtimeWorker{
		stopChan: make(chan struct{}),
		done:     make(chan struct{}),
		running:  true,
	}
	registry.worker = w
	go sm.runRealtimeWorker(w)
}

func (sm *SystemManager) checkWorkerStop() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if len(registry.entries) == 0 && registry.worker != nil && registry.worker.running {
		close(registry.worker.stopChan)
		registry.worker.running = false
	}
}

func (sm *SystemManager) runRealtimeWorker(w *realtimeWorker) {
	defer close(w.done)
	sm.fetchAndBroadcast()
	interval := sm.computeInterval()
	w.ticker = time.NewTicker(interval)
	defer w.ticker.Stop()
	for {
		select {
		case <-w.stopChan:
			return
		case <-w.ticker.C:
			registry.mu.Lock()
			count := len(registry.entries)
			registry.mu.Unlock()
			if count == 0 {
				return
			}
			newInterval := sm.computeInterval()
			if newInterval != interval {
				interval = newInterval
				w.ticker.Reset(interval)
			}
			sm.fetchAndBroadcast()
		}
	}
}

func (sm *SystemManager) computeInterval() time.Duration {
	registry.mu.Lock()
	listCount := 0
	hasDetail := false
	for _, entries := range registry.entries {
		hasListForSystem := false
		for _, e := range entries {
			if e.kind == kindList {
				hasListForSystem = true
			}
			if e.kind == kindDetail {
				hasDetail = true
			}
		}
		if hasListForSystem {
			listCount++
		}
	}
	registry.mu.Unlock()

	if hasDetail {
		return 1 * time.Second
	}
	switch {
	case listCount <= 20:
		return 1 * time.Second
	case listCount <= 100:
		return 2 * time.Second
	default:
		return 5 * time.Second
	}
}

// fetchAndBroadcast fetches data for all subscribed systems and broadcasts to subscribers.
type pendingFetch struct {
	systemID string
	entries  []*subscriptionEntry
}

func (sm *SystemManager) fetchAndBroadcast() {
	registry.mu.Lock()
	fetches := make([]pendingFetch, 0, len(registry.entries))
	for systemID, entries := range registry.entries {
		entriesCopy := make([]*subscriptionEntry, len(entries))
		copy(entriesCopy, entries)
		fetches = append(fetches, pendingFetch{systemID: systemID, entries: entriesCopy})
	}
	registry.mu.Unlock()

	var wg sync.WaitGroup
	for _, pf := range fetches {
		sys, err := sm.GetSystem(pf.systemID)
		if err != nil {
			continue
		}
		wg.Add(1)
		go func(s *System, entries []*subscriptionEntry) {
			defer wg.Done()
			if !s.fetchMu.TryLock() {
				return
			}
			defer s.fetchMu.Unlock()

			data, err := s.fetchDataFromAgent(common.DataRequestOptions{CacheTimeMs: 1000})
			if err != nil {
				return
			}

			if data.Info.BandwidthByDirection == [2]uint64{} && data.Stats.Bandwidth != [2]uint64{} {
				data.Info.BandwidthByDirection = data.Stats.Bandwidth
			}
			if len(data.Stats.VPSProbe) > 0 {
				data.Info.VPSProbe = data.Stats.VPSProbe
			}

			sm.deriveRealtimeTraffic(s, data)

			hasDetail := false
			hasList := false
			for _, e := range entries {
				if e.kind == kindDetail {
					hasDetail = true
				}
				if e.kind == kindList {
					hasList = true
				}
			}

			if hasDetail {
				fullBytes, err := json.Marshal(data)
				if err == nil {
					for _, e := range entries {
						if e.kind == kindDetail {
							notify(sm.hub, e.subscription, fullBytes)
						}
					}
				}
			}

			if hasList {
				summary := buildListSummary(s.Id, data)
				summaryBytes, err := json.Marshal(summary)
				if err == nil {
					for _, e := range entries {
						if e.kind == kindList {
							notify(sm.hub, e.subscription, summaryBytes)
						}
					}
				}
			}
		}(sys, pf.entries)
	}
	wg.Wait()
}

// deriveRealtimeTraffic updates VPS traffic from realtime samples without persistent fsync.
func (sm *SystemManager) deriveRealtimeTraffic(sys *System, data *system.CombinedData) {
	if sm.trafficManager == nil || data.Stats.NetworkInterfaces == nil {
		return
	}
	systemRecord, err := sys.getRecord(sm.hub)
	if err != nil {
		return
	}
	systemName := systemRecord.GetString("name")
	var dbTraffic *VPSTrafficNodeConfig
	if vpsJSON := systemRecord.GetString("vps"); vpsJSON != "" {
		var settings struct {
			Traffic *VPSTrafficNodeConfig `json:"traffic"`
		}
		if json.Unmarshal([]byte(vpsJSON), &settings) == nil && settings.Traffic != nil {
			dbTraffic = settings.Traffic
		}
	}
	data.Info.VPSTraffic = sm.trafficManager.DeriveTraffic(sys.Id, systemName, dbTraffic, data.Stats.NetworkInterfaces, false)
}

// SystemListSummary is a lightweight payload for All Systems row updates.
type SystemListSummary struct {
	SystemID  string      `json:"id"`
	Timestamp int64       `json:"ts"`
	Info      system.Info `json:"info"`
}

const (
	maxSummaryTargetLen   = 40
	minSummaryTargetLen   = len(summaryTargetEllipsis)
	maxListSummaryBytes   = 1024
	summaryTargetEllipsis = "…"
)

func roundSummaryFloat(value float64) float64 {
	return math.Round(value*100) / 100
}

func truncateSummaryTargetTo(target string, maxLen int) string {
	if len(target) <= maxLen {
		return target
	}

	end := maxLen - len(summaryTargetEllipsis)
	for end > 0 && !utf8.ValidString(target[:end]) {
		end--
	}
	return target[:end] + summaryTargetEllipsis
}

func truncateSummaryTarget(target string) string {
	return truncateSummaryTargetTo(target, maxSummaryTargetLen)
}

func fitFourTargetSummary(summary *SystemListSummary, source system.VPSProbeStats) {
	payload, err := json.Marshal(summary)
	if err != nil || len(payload) <= maxListSummaryBytes {
		return
	}

	primaryCount := 0
	for _, v := range summary.Info.VPSProbe {
		if v.Position >= 1 && v.Position <= 3 {
			primaryCount++
		}
	}
	if primaryCount == 0 {
		return
	}

	excess := len(payload) - maxListSummaryBytes
	limit := maxSummaryTargetLen - (excess+primaryCount-1)/primaryCount
	if limit < minSummaryTargetLen {
		limit = minSummaryTargetLen
	}
	applyLimit := func(maxLen int) {
		for id, v := range summary.Info.VPSProbe {
			if v.Position < 1 || v.Position > 3 {
				continue
			}
			v.Target = truncateSummaryTargetTo(source[id].Target, maxLen)
			summary.Info.VPSProbe[id] = v
		}
	}
	applyLimit(limit)

	payload, err = json.Marshal(summary)
	if err == nil && len(payload) > maxListSummaryBytes && limit > minSummaryTargetLen {
		applyLimit(minSummaryTargetLen)
	}
}

func buildListSummary(systemID string, data *system.CombinedData) SystemListSummary {
	info := data.Info
	sourceProbe := info.VPSProbe
	info.Hostname = ""
	info.KernelVersion = ""
	info.CpuModel = ""
	info.Podman = false
	info.Os = 0
	info.Cores = 0
	info.Bandwidth = 0
	if info.VPSProbe != nil {
		stripped := make(system.VPSProbeStats, len(info.VPSProbe))
		for k, v := range info.VPSProbe {
			// Position four is hover/detail-only in the systems list. Keep the
			// fields needed by its tooltip here; rt_metrics and persisted history
			// still retain the complete target record.
			if len(info.VPSProbe) == 4 && v.Position == 4 {
				stripped[k] = system.VPSProbeTargetStats{
					LatencyMs:          roundSummaryFloat(v.LatencyMs),
					LossPct:            roundSummaryFloat(v.LossPct),
					Samples:            v.Samples,
					LatencyAvgWindowMs: roundSummaryFloat(v.LatencyAvgWindowMs),
					Local:              v.Local,
					Label:              v.Label,
					Position:           v.Position,
				}
				continue
			}
			target := truncateSummaryTarget(v.Target)
			stripped[k] = system.VPSProbeTargetStats{
				LatencyMs:          roundSummaryFloat(v.LatencyMs),
				LossPct:            roundSummaryFloat(v.LossPct),
				Success:            v.Success,
				Samples:            v.Samples,
				Updated:            v.Updated,
				Target:             target,
				LatencyAvgWindowMs: roundSummaryFloat(v.LatencyAvgWindowMs),
				Local:              v.Local,
				Label:              v.Label,
				Position:           v.Position,
			}
		}
		info.VPSProbe = stripped
	}
	if info.BandwidthByDirection != [2]uint64{} {
		info.BandwidthBytes = 0
	}
	summary := SystemListSummary{
		SystemID:  systemID,
		Timestamp: time.Now().UnixMilli(),
		Info:      info,
	}
	if len(sourceProbe) == 4 {
		fitFourTargetSummary(&summary, sourceProbe)
	}
	return summary
}

func notify(app core.App, subscription string, data []byte) error {
	message := subscriptions.Message{
		Name: subscription,
		Data: data,
	}
	clients := app.SubscriptionsBroker().Clients()
	for _, client := range clients {
		if !client.HasSubscription(subscription) {
			continue
		}
		client.Send(message)
	}
	return nil
}

func init() {
	if registry.entries == nil {
		registry.entries = make(map[string][]*subscriptionEntry)
	}
}

// LogActiveSubscriptions logs current subscription state for debugging.
func (sm *SystemManager) LogActiveSubscriptions() {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	for systemID, entries := range registry.entries {
		for _, e := range entries {
			kindStr := "detail"
			if e.kind == kindList {
				kindStr = "list"
			}
			slog.Debug("Active subscription", "system", systemID, "kind", kindStr, "clients", e.connectedClients)
		}
	}
}
