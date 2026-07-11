package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

// VPSProbeTargetConfig is the per-target V7 array element.
type VPSProbeTargetConfig struct {
	ID      string `json:"id"`
	Label   string `json:"label"`
	Address string `json:"address,omitempty"`
	Local   bool   `json:"local,omitempty"`
}

// rawVPSProbeConfig handles both the V7 array and legacy map formats for targets.
type rawVPSProbeConfig struct {
	Enabled         *bool           `json:"enabled,omitempty"`
	HubLocal        bool            `json:"hubLocal"`
	IntervalSeconds int             `json:"intervalSeconds"`
	TimeoutMs       int             `json:"timeoutMs"`
	WindowSize      int             `json:"windowSize"`
	Targets         json.RawMessage `json:"targets"`
}

type resolvedTarget struct {
	id      string
	label   string
	address string
	local   bool
	pos     uint8
}

type probeSample struct {
	latencyMs float64
	success   bool
}

type probeWindow struct {
	samples []probeSample
	pos     int
	count   int
}

type VPSProbeCollector struct {
	mu         sync.RWMutex
	targets    []resolvedTarget
	windows    map[string]*probeWindow
	latest     system.VPSProbeStats
	cancel     context.CancelFunc
	done       chan struct{}
	interval   time.Duration
	timeout    time.Duration
	windowSize int
	intervalS  int
}

var canonicalProbeKeys = [4]string{"hub", "ct", "cu", "cm"}
var canonicalProbeLabels = map[string]string{"hub": "HUB", "ct": "CT", "cu": "CU", "cm": "CM"}

var defaultProbeTargets = map[string]string{
	"hub": "hzcsj.ikfly.com:20022",
	"ct":  "ct.tz.cloudcpp.com:80",
	"cu":  "cu.tz.cloudcpp.com:80",
	"cm":  "cm.tz.cloudcpp.com:80",
}

var idPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// Four targets are supported so proxy nodes can keep the three primary
// carrier probes and add one cross-region probe. The compact systems list
// still renders only the first three; detail views and tooltips use all four.
const maxTargets = 4

func newVPSProbeCollector() *VPSProbeCollector {
	raw, exists := utils.GetEnv("BESZEL_AGENT_VPS_PROBE_CONFIG")
	if !exists {
		slog.Info("VPS probe collector not configured (BESZEL_AGENT_VPS_PROBE_CONFIG absent)")
		return nil
	}

	var cfg rawVPSProbeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		slog.Warn("BESZEL_AGENT_VPS_PROBE_CONFIG parse error, probes disabled", "err", err)
		return nil
	}

	if cfg.Enabled != nil && !*cfg.Enabled {
		slog.Info("VPS probe collector explicitly disabled")
		return nil
	}

	intervalS := 5
	if cfg.IntervalSeconds > 0 {
		intervalS = clampInt(cfg.IntervalSeconds, 1, 300)
	}
	timeoutMs := 1000
	if cfg.TimeoutMs > 0 {
		timeoutMs = clampInt(cfg.TimeoutMs, 100, 5000)
	}
	windowSize := 60
	if cfg.WindowSize > 0 {
		windowSize = clampInt(cfg.WindowSize, 10, 600)
	}

	targets := parseTargets(cfg.Targets, cfg.HubLocal)
	if len(targets) == 0 {
		slog.Info("VPS probe collector: no valid targets configured, probes disabled")
		return nil
	}

	c := &VPSProbeCollector{
		targets:    targets,
		windows:    make(map[string]*probeWindow, len(targets)),
		latest:     make(system.VPSProbeStats, len(targets)),
		interval:   time.Duration(intervalS) * time.Second,
		timeout:    time.Duration(timeoutMs) * time.Millisecond,
		windowSize: windowSize,
		intervalS:  intervalS,
	}
	for _, t := range targets {
		c.windows[t.id] = &probeWindow{
			samples: make([]probeSample, windowSize),
		}
	}
	return c
}

func parseTargets(raw json.RawMessage, hubLocal bool) []resolvedTarget {
	if len(raw) == 0 {
		return legacyCanonicalTargets(hubLocal)
	}

	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) == 0 {
		return legacyCanonicalTargets(hubLocal)
	}

	if trimmed[0] == '[' {
		return parseArrayTargets(raw)
	}
	if trimmed[0] == '{' {
		return parseLegacyMapTargets(raw, hubLocal)
	}
	return legacyCanonicalTargets(hubLocal)
}

func legacyCanonicalTargets(hubLocal bool) []resolvedTarget {
	slog.Warn("config has no targets field, using canonical defaults; migrate to the array format")
	return buildCanonicalTargets(nil, hubLocal)
}

func parseArrayTargets(raw json.RawMessage) []resolvedTarget {
	var configs []VPSProbeTargetConfig
	if err := json.Unmarshal(raw, &configs); err != nil {
		slog.Warn("targets array parse error", "err", err)
		return nil
	}

	if len(configs) > maxTargets {
		slog.Warn("targets array exceeds maximum, rejecting all entries", "count", len(configs), "max", maxTargets)
		return nil
	}

	seenIDs := make(map[string]bool, len(configs))
	result := make([]resolvedTarget, 0, len(configs))

	for i, tc := range configs {
		id := strings.TrimSpace(tc.ID)
		label := strings.TrimSpace(tc.Label)
		address := strings.TrimSpace(tc.Address)

		if err := validateTargetID(id); err != nil {
			slog.Warn("skipping target with invalid id", "index", i, "id", tc.ID, "err", err)
			continue
		}
		if err := validateTargetLabel(label); err != nil {
			slog.Warn("skipping target with invalid label", "index", i, "id", id, "err", err)
			continue
		}
		if seenIDs[id] {
			slog.Warn("skipping target with duplicate id", "index", i, "id", id)
			continue
		}
		if !tc.Local {
			if err := validateTargetAddress(address); err != nil {
				slog.Warn("skipping target with invalid address", "index", i, "id", id, "err", err)
				continue
			}
		}
		seenIDs[id] = true
		result = append(result, resolvedTarget{
			id:      id,
			label:   label,
			address: address,
			local:   tc.Local,
			pos:     uint8(len(result) + 1),
		})
	}

	if len(result) > maxTargets {
		result = result[:maxTargets]
	}
	return result
}

func parseLegacyMapTargets(raw json.RawMessage, hubLocal bool) []resolvedTarget {
	slog.Warn("using deprecated targets object format; migrate to the array format")
	var mapTargets map[string]string
	if err := json.Unmarshal(raw, &mapTargets); err != nil {
		slog.Warn("legacy targets map parse error", "err", err)
		return nil
	}
	return buildCanonicalTargets(mapTargets, hubLocal)
}

func buildCanonicalTargets(mapTargets map[string]string, hubLocal bool) []resolvedTarget {
	result := make([]resolvedTarget, 0, len(canonicalProbeKeys))
	for i, key := range canonicalProbeKeys {
		addr := ""
		if mapTargets != nil {
			addr = mapTargets[key]
		}
		if addr == "" {
			addr = defaultProbeTargets[key]
		}
		isLocal := hubLocal && key == "hub"
		label := canonicalProbeLabels[key]
		result = append(result, resolvedTarget{
			id:      key,
			label:   label,
			address: addr,
			local:   isLocal,
			pos:     uint8(i + 1),
		})
	}
	return result
}

func validateTargetID(id string) error {
	if id == "" {
		return fmt.Errorf("empty id")
	}
	if len(id) > 16 {
		return fmt.Errorf("id exceeds 16 characters")
	}
	if !idPattern.MatchString(id) {
		return fmt.Errorf("id contains invalid characters")
	}
	return nil
}

func validateTargetLabel(label string) error {
	if label == "" {
		return fmt.Errorf("empty label")
	}
	count := utf8.RuneCountInString(label)
	if count > 12 {
		return fmt.Errorf("label exceeds 12 code points")
	}
	for _, r := range label {
		if unicode.IsControl(r) {
			return fmt.Errorf("label contains control character")
		}
	}
	return nil
}

const maxAddressLen = 253 + 1 + 5 // max DNS label + colon + max port digits

func validateTargetAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("empty address")
	}
	if len(addr) > maxAddressLen {
		return fmt.Errorf("address exceeds %d bytes", maxAddressLen)
	}
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid host:port: %w", err)
	}
	if host == "" {
		return fmt.Errorf("empty host")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("port must be numeric 1..65535")
	}
	return nil
}

func (c *VPSProbeCollector) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	c.done = make(chan struct{})
	go func() {
		defer close(c.done)
		c.run(ctx)
	}()
}

func (c *VPSProbeCollector) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
	if c.done != nil {
		<-c.done
	}
}

func (c *VPSProbeCollector) GetResults() system.VPSProbeStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(system.VPSProbeStats, len(c.latest))
	for k, v := range c.latest {
		result[k] = v
	}
	return result
}

func (c *VPSProbeCollector) run(ctx context.Context) {
	c.probeAll(ctx)
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.probeAll(ctx)
		}
	}
}

func (c *VPSProbeCollector) probeAll(ctx context.Context) {
	var wg sync.WaitGroup
	type result struct {
		id      string
		latency float64
		ok      bool
	}
	results := make(chan result, len(c.targets))

	for _, t := range c.targets {
		if t.local {
			continue
		}
		wg.Add(1)
		go func(id, addr string) {
			defer wg.Done()
			lat, ok := c.probeTarget(ctx, addr)
			results <- result{id: id, latency: lat, ok: ok}
		}(t.id, t.address)
	}
	wg.Wait()
	close(results)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()

	for _, t := range c.targets {
		if t.local {
			c.latest[t.id] = system.VPSProbeTargetStats{
				Local:    true,
				Target:   t.address,
				Updated:  now,
				Label:    t.label,
				Position: t.pos,
			}
		}
	}

	for r := range results {
		w := c.windows[r.id]
		w.samples[w.pos] = probeSample{latencyMs: r.latency, success: r.ok}
		w.pos = (w.pos + 1) % len(w.samples)
		if w.count < len(w.samples) {
			w.count++
		}

		var t *resolvedTarget
		for i := range c.targets {
			if c.targets[i].id == r.id {
				t = &c.targets[i]
				break
			}
		}
		addr := ""
		label := ""
		var pos uint8
		if t != nil {
			addr = t.address
			label = t.label
			pos = t.pos
		}

		stats := computeWindowStats(w, c.intervalS, addr)
		stats.Updated = now
		stats.Label = label
		stats.Position = pos
		c.latest[r.id] = stats
	}
}

func (c *VPSProbeCollector) probeTarget(ctx context.Context, addr string) (latencyMs float64, success bool) {
	dialCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	start := time.Now()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(dialCtx, "tcp", addr)
	elapsed := time.Since(start)
	latencyMs = float64(elapsed.Microseconds()) / 1000.0

	if err == nil {
		conn.Close()
		return latencyMs, true
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "dial" {
		if opErr.Err != nil {
			errStr := opErr.Err.Error()
			if len(errStr) > 0 && (errStr == "connection refused" ||
				(len(errStr) > 18 && errStr[len(errStr)-18:] == "connection refused")) {
				return latencyMs, true
			}
		}
	}

	return 0, false
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// computeWindowStats computes probe statistics from a ring buffer deterministically.
func computeWindowStats(w *probeWindow, intervalSec int, target string) system.VPSProbeTargetStats {
	if w.count == 0 {
		return system.VPSProbeTargetStats{Target: target}
	}

	var windowFailed int
	var windowLatSum float64
	var windowLatCount int
	for i := 0; i < w.count; i++ {
		s := w.samples[i]
		if !s.success {
			windowFailed++
		} else {
			windowLatSum += s.latencyMs
			windowLatCount++
		}
	}

	recentCount := clampInt(int(math.Ceil(60.0/float64(intervalSec))), 1, w.count)
	var recentLatSum float64
	var recentLatCount int
	var recentFailed int
	for j := 0; j < recentCount; j++ {
		idx := (w.pos - 1 - j + len(w.samples)) % len(w.samples)
		s := w.samples[idx]
		if s.success {
			recentLatSum += s.latencyMs
			recentLatCount++
		} else {
			recentFailed++
		}
	}

	lastSample := w.samples[(w.pos-1+len(w.samples))%len(w.samples)]
	var lastLat float64
	if lastSample.success {
		lastLat = lastSample.latencyMs
	}

	stats := system.VPSProbeTargetStats{
		LatencyMs: lastLat,
		LossPct:   float64(windowFailed) / float64(w.count) * 100,
		Success:   lastSample.success,
		Samples:   uint16(w.count),
		Target:    target,
		Samples1m: uint16(recentCount),
		LossPct1m: float64(recentFailed) / float64(recentCount) * 100,
	}
	if recentLatCount > 0 {
		stats.LatencyAvg1mMs = recentLatSum / float64(recentLatCount)
	}
	if windowLatCount > 0 {
		stats.LatencyAvgWindowMs = windowLatSum / float64(windowLatCount)
	}
	return stats
}
