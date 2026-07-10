package agent

import (
	"context"
	"encoding/json"
	"math"
	"net"
	"testing"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

func TestClampInt(t *testing.T) {
	tests := []struct {
		v, min, max, want int
	}{
		{5, 1, 10, 5},
		{0, 1, 10, 1},
		{15, 1, 10, 10},
		{-3, 0, 100, 0},
		{50, 50, 50, 50},
	}
	for _, tt := range tests {
		if got := clampInt(tt.v, tt.min, tt.max); got != tt.want {
			t.Errorf("clampInt(%d, %d, %d) = %d, want %d", tt.v, tt.min, tt.max, got, tt.want)
		}
	}
}

func TestProbeConfigEnabledPointerNil(t *testing.T) {
	raw := `{"intervalSeconds":10,"targets":{"hub":"1.2.3.4:22"}}`
	var cfg VPSProbeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled != nil {
		t.Errorf("Enabled should be nil when omitted, got %v", *cfg.Enabled)
	}
}

func TestProbeConfigEnabledExplicitFalse(t *testing.T) {
	raw := `{"enabled":false,"targets":{"hub":"1.2.3.4:22"}}`
	var cfg VPSProbeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled == nil {
		t.Fatal("Enabled should not be nil when explicitly set")
	}
	if *cfg.Enabled {
		t.Error("Enabled should be false")
	}
}

func TestProbeConfigEnabledExplicitTrue(t *testing.T) {
	raw := `{"enabled":true}`
	var cfg VPSProbeConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled == nil || !*cfg.Enabled {
		t.Error("Enabled should be true when explicitly set")
	}
}

func TestNewVPSProbeCollectorDefaults(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", "")
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil with default config")
	}
	if len(c.config.Targets) != len(defaultProbeTargets) {
		t.Errorf("expected %d default targets, got %d", len(defaultProbeTargets), len(c.config.Targets))
	}
}

func TestNewVPSProbeCollectorDisabledExplicitly(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"enabled":false}`)
	c := newVPSProbeCollector()
	if c != nil {
		t.Error("collector should be nil when explicitly disabled")
	}
}

func TestNewVPSProbeCollectorOmittedEnabled(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":10}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil when enabled is omitted")
	}
	if c.interval.Seconds() != 10 {
		t.Errorf("expected 10s interval, got %v", c.interval)
	}
}

func TestNewVPSProbeCollectorInvalidJSON(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `not valid json`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should fall back to defaults on invalid JSON")
	}
}

func TestNewVPSProbeCollectorCustomTargetsCanonicalOnly(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"targets":{"hub":"10.0.0.1:22","myhost":"10.0.0.2:80"}}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	if len(c.config.Targets) != 4 {
		t.Errorf("should always have 4 canonical targets, got %d", len(c.config.Targets))
	}
	if c.config.Targets["hub"] != "10.0.0.1:22" {
		t.Errorf("hub should be overridden: got %s", c.config.Targets["hub"])
	}
	if c.config.Targets["ct"] != defaultProbeTargets["ct"] {
		t.Errorf("ct should remain default: got %s", c.config.Targets["ct"])
	}
	if _, ok := c.config.Targets["myhost"]; ok {
		t.Error("non-canonical key 'myhost' should be ignored")
	}
}

func TestNewVPSProbeCollectorClamping(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":999,"timeoutMs":1,"windowSize":5}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	if c.interval.Seconds() != 300 {
		t.Errorf("interval should be clamped to 300s, got %v", c.interval)
	}
	if c.timeout.Milliseconds() != 100 {
		t.Errorf("timeout should be clamped to 100ms, got %v", c.timeout)
	}
	if c.config.WindowSize != 10 {
		t.Errorf("windowSize should be clamped to 10, got %d", c.config.WindowSize)
	}
}

func TestProbeCollectorStartStop(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":{"hub":"192.0.2.1:1"}}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	c.Start()
	results := c.GetResults()
	if results == nil {
		t.Error("GetResults should return non-nil map")
	}
	c.Stop()
}

func TestProbeCollectorStopWaitsForRunExit(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":{"hub":"192.0.2.1:1"}}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	c.Start()
	c.Stop()
	select {
	case <-c.done:
	default:
		t.Error("done channel should be closed after Stop")
	}
	results := c.GetResults()
	if results == nil {
		t.Error("GetResults after Stop should return non-nil map")
	}
}

func TestProbeCollectorStopBeforeStart(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":{"hub":"192.0.2.1:1"}}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	c.Stop()
}

func TestProbeTargetSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	c := &VPSProbeCollector{timeout: 2 * time.Second}
	lat, ok := c.probeTarget(context.Background(), ln.Addr().String())
	if !ok {
		t.Error("probing open local listener should succeed")
	}
	if lat <= 0 {
		t.Errorf("expected positive latency, got %f", lat)
	}
}

func TestProbeTargetRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	c := &VPSProbeCollector{timeout: 2 * time.Second}
	lat, ok := c.probeTarget(context.Background(), addr)
	if !ok {
		t.Error("ECONNREFUSED should count as success (host reachable)")
	}
	if lat <= 0 {
		t.Errorf("expected positive latency on refused, got %f", lat)
	}
}

func TestProbeTargetTimeout(t *testing.T) {
	c := &VPSProbeCollector{timeout: 50 * time.Millisecond}
	lat, ok := c.probeTarget(context.Background(), "192.0.2.1:1")
	if ok {
		t.Error("probing unreachable RFC 5737 address should fail")
	}
	if lat != 0 {
		t.Errorf("failed probe should return 0 latency, got %f", lat)
	}
}

func TestProbeAllConcurrentWithLocalTargets(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	lnRefused, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedAddr := lnRefused.Addr().String()
	lnRefused.Close()

	c := &VPSProbeCollector{
		config: VPSProbeConfig{
			WindowSize: 10,
			Targets: map[string]string{
				"hub": ln.Addr().String(),
				"ct":  ln.Addr().String(),
				"cu":  refusedAddr,
				"cm":  "192.0.2.1:1",
			},
		},
		windows: make(map[string]*probeWindow, 4),
		latest:  make(system.VPSProbeStats, 4),
		timeout: 200 * time.Millisecond,
	}
	for key := range c.config.Targets {
		c.windows[key] = &probeWindow{samples: make([]probeSample, 10)}
	}

	c.probeAll(context.Background())

	hub := c.latest["hub"]
	if !hub.Success {
		t.Error("hub (open listener) should succeed")
	}
	ct := c.latest["ct"]
	if !ct.Success {
		t.Error("ct (open listener) should succeed")
	}
	cu := c.latest["cu"]
	if !cu.Success {
		t.Error("cu (refused port) should succeed")
	}
	cm := c.latest["cm"]
	if cm.Success {
		t.Error("cm (unreachable) should fail")
	}
	if hub.LossPct != 0 {
		t.Errorf("hub loss should be 0%%, got %f%%", hub.LossPct)
	}
	if cm.LossPct != 100 {
		t.Errorf("cm loss should be 100%%, got %f%%", cm.LossPct)
	}
}

func TestProbeWindowRollingLossViaProbeAll(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	const windowSize = 4
	newCollector := func() *VPSProbeCollector {
		return &VPSProbeCollector{
			config: VPSProbeConfig{
				WindowSize: windowSize,
				Targets:    map[string]string{"hub": ln.Addr().String()},
			},
			windows: map[string]*probeWindow{
				"hub": {samples: make([]probeSample, windowSize)},
			},
			latest:  make(system.VPSProbeStats),
			timeout: 2 * time.Second,
		}
	}

	t.Run("all_success", func(t *testing.T) {
		c := newCollector()
		for i := 0; i < windowSize; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.Samples != windowSize {
			t.Errorf("expected %d samples, got %d", windowSize, hub.Samples)
		}
		if hub.LossPct != 0 {
			t.Errorf("all probes succeeded, loss should be 0%%, got %f%%", hub.LossPct)
		}
		if !hub.Success {
			t.Error("last probe should be success")
		}
	})

	t.Run("mixed_success_failure", func(t *testing.T) {
		c := newCollector()
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)
		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)

		hub := c.latest["hub"]
		if hub.Samples != windowSize {
			t.Errorf("expected %d samples, got %d", windowSize, hub.Samples)
		}
		if hub.LossPct != 50 {
			t.Errorf("expected 50%% loss, got %f%%", hub.LossPct)
		}
		if hub.Success {
			t.Error("last probe used cancelled context, should be failure")
		}
	})

	t.Run("eviction_recovery", func(t *testing.T) {
		c := newCollector()
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)
		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)

		hub := c.latest["hub"]
		if hub.LossPct != 50 {
			t.Errorf("pre-eviction: expected 50%% loss, got %f%%", hub.LossPct)
		}

		for i := 0; i < windowSize; i++ {
			c.probeAll(context.Background())
		}

		hub = c.latest["hub"]
		if hub.LossPct != 0 {
			t.Errorf("post-eviction: expected 0%% loss, got %f%%", hub.LossPct)
		}
		if !hub.Success {
			t.Error("last probe should be success after recovery")
		}
	})
}

func TestCanonicalProbeKeysLength(t *testing.T) {
	if len(canonicalProbeKeys) != 4 {
		t.Errorf("expected 4 canonical keys, got %d", len(canonicalProbeKeys))
	}
	expected := map[string]bool{"hub": true, "ct": true, "cu": true, "cm": true}
	for _, k := range canonicalProbeKeys {
		if !expected[k] {
			t.Errorf("unexpected canonical key: %s", k)
		}
	}
}

func newTestCollectorWithListener(t *testing.T, addr string, windowSize, intervalSec int) *VPSProbeCollector {
	t.Helper()
	return &VPSProbeCollector{
		config: VPSProbeConfig{
			IntervalSeconds: intervalSec,
			WindowSize:      windowSize,
			Targets:         map[string]string{"hub": addr},
		},
		windows: map[string]*probeWindow{
			"hub": {samples: make([]probeSample, windowSize)},
		},
		latest:  make(system.VPSProbeStats),
		timeout: 2 * time.Second,
	}
}

func TestLatencyAverages(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	t.Run("raw_lat_unchanged", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 60, 5)
		c.probeAll(context.Background())
		hub := c.latest["hub"]
		if hub.LatencyMs <= 0 {
			t.Error("raw lat should be positive on success")
		}
		if !hub.Success {
			t.Error("should succeed")
		}
	})

	t.Run("one_minute_mean_recent_samples", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 60, 5)
		for i := 0; i < 20; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.LatencyAvg1mMs <= 0 {
			t.Error("lat1 should be positive after 20 successful probes")
		}
		if hub.LatencyAvgWindowMs <= 0 {
			t.Error("latw should be positive after 20 successful probes")
		}
	})

	t.Run("window_mean_uses_all_success_only", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 4, 5)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)
		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)

		hub := c.latest["hub"]
		if hub.LossPct != 50 {
			t.Errorf("expected 50%% loss, got %f%%", hub.LossPct)
		}
		if hub.LatencyAvgWindowMs <= 0 {
			t.Error("latw should be positive (only counts successful samples)")
		}
	})

	t.Run("failed_samples_do_not_zero_latency_mean", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 4, 5)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		avgAfterOne := c.latest["hub"].LatencyAvgWindowMs

		c.probeAll(cancelledCtx)
		avgAfterTwo := c.latest["hub"].LatencyAvgWindowMs

		if avgAfterTwo != avgAfterOne {
			t.Errorf("window mean should not change when failed sample is added (was %f, now %f)", avgAfterOne, avgAfterTwo)
		}
	})

	t.Run("no_success_omits_both_means", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 4, 5)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		for i := 0; i < 4; i++ {
			c.probeAll(cancelledCtx)
		}
		hub := c.latest["hub"]
		if hub.LatencyAvg1mMs != 0 {
			t.Errorf("lat1 should be 0 when no success, got %f", hub.LatencyAvg1mMs)
		}
		if hub.LatencyAvgWindowMs != 0 {
			t.Errorf("latw should be 0 when no success, got %f", hub.LatencyAvgWindowMs)
		}
		if hub.LossPct != 100 {
			t.Errorf("expected 100%% loss, got %f%%", hub.LossPct)
		}
	})

	t.Run("wraparound_recent_samples", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 4, 5)

		for i := 0; i < 6; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.Samples != 4 {
			t.Errorf("expected 4 samples after wraparound, got %d", hub.Samples)
		}
		if hub.LatencyAvg1mMs <= 0 {
			t.Error("lat1 should still be positive after wraparound")
		}
		if hub.LatencyAvgWindowMs <= 0 {
			t.Error("latw should still be positive after wraparound")
		}
	})

	t.Run("eviction_restores_averages", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 4, 5)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)
		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)

		if c.latest["hub"].LossPct != 50 {
			t.Errorf("expected 50%% loss, got %f%%", c.latest["hub"].LossPct)
		}

		for i := 0; i < 4; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.LossPct != 0 {
			t.Errorf("expected 0%% loss after eviction, got %f%%", hub.LossPct)
		}
		if hub.LatencyAvgWindowMs <= 0 {
			t.Error("latw should be positive after eviction")
		}
	})

	t.Run("interval_above_one_minute", func(t *testing.T) {
		c := newTestCollectorWithListener(t, ln.Addr().String(), 10, 120)
		for i := 0; i < 5; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.LatencyAvg1mMs <= 0 {
			t.Error("lat1 should be positive even with large interval (clamped to 1 recent sample)")
		}
	})
}

func makeWindow(samples []probeSample) *probeWindow {
	w := &probeWindow{
		samples: make([]probeSample, len(samples)),
		count:   len(samples),
		pos:     len(samples) % len(samples),
	}
	copy(w.samples, samples)
	return w
}

func TestComputeWindowStatsDeterministic(t *testing.T) {
	t.Run("12_samples_mixed", func(t *testing.T) {
		samples := make([]probeSample, 60)
		for i := range samples {
			samples[i] = probeSample{latencyMs: float64(10 + i), success: true}
		}
		samples[58] = probeSample{success: false}
		samples[59] = probeSample{success: false}

		w := &probeWindow{samples: samples, count: 60, pos: 0}
		s := computeWindowStats(w, 5, "hub")

		if s.Samples1m != 12 {
			t.Errorf("n1: want 12, got %d", s.Samples1m)
		}
		wantLoss1 := 2.0 / 12.0 * 100
		if math.Abs(s.LossPct1m-wantLoss1) > 1e-9 {
			t.Errorf("loss1: want %f, got %f", wantLoss1, s.LossPct1m)
		}
		wantLat1 := float64(58+59+60+61+62+63+64+65+66+67) / 10.0
		if s.LatencyAvg1mMs != wantLat1 {
			t.Errorf("lat1: want %f, got %f", wantLat1, s.LatencyAvg1mMs)
		}
	})

	t.Run("all_success_0pct_loss", func(t *testing.T) {
		samples := []probeSample{
			{latencyMs: 10, success: true},
			{latencyMs: 20, success: true},
			{latencyMs: 30, success: true},
			{latencyMs: 40, success: true},
		}
		w := makeWindow(samples)
		s := computeWindowStats(w, 5, "hub")
		if s.LossPct1m != 0 {
			t.Errorf("loss1 should be 0, got %f", s.LossPct1m)
		}
		if s.Samples1m != 4 {
			t.Errorf("n1 should be 4, got %d", s.Samples1m)
		}
		wantLat := (10.0 + 20 + 30 + 40) / 4.0
		if s.LatencyAvgWindowMs != wantLat {
			t.Errorf("latw: want %f, got %f", wantLat, s.LatencyAvgWindowMs)
		}
	})

	t.Run("all_failed_100pct_loss", func(t *testing.T) {
		samples := []probeSample{
			{success: false},
			{success: false},
			{success: false},
			{success: false},
		}
		w := makeWindow(samples)
		s := computeWindowStats(w, 5, "hub")
		if s.LossPct1m != 100 {
			t.Errorf("loss1 should be 100, got %f", s.LossPct1m)
		}
		if s.Samples1m == 0 {
			t.Error("n1 must be positive even when all failed")
		}
		if s.LatencyAvg1mMs != 0 {
			t.Errorf("lat1 should be 0 (omitted) when all failed, got %f", s.LatencyAvg1mMs)
		}
	})

	t.Run("wraparound_selects_newest", func(t *testing.T) {
		samples := make([]probeSample, 4)
		samples[0] = probeSample{latencyMs: 100, success: true}
		samples[1] = probeSample{latencyMs: 200, success: true}
		samples[2] = probeSample{latencyMs: 300, success: true}
		samples[3] = probeSample{latencyMs: 400, success: true}
		w := &probeWindow{samples: samples, count: 4, pos: 2}

		s := computeWindowStats(w, 5, "hub")
		wantLat1 := (100.0 + 200 + 300 + 400) / 4.0
		if s.LatencyAvg1mMs != wantLat1 {
			t.Errorf("lat1: want %f, got %f", wantLat1, s.LatencyAvg1mMs)
		}
		if s.LatencyMs != 200 {
			t.Errorf("raw lat should be 200 (pos=2, newest at idx 1), got %f", s.LatencyMs)
		}
	})

	t.Run("failed_not_zero_latency", func(t *testing.T) {
		samples := []probeSample{
			{latencyMs: 10, success: true},
			{success: false},
		}
		w := makeWindow(samples)
		s := computeWindowStats(w, 5, "hub")
		if s.LatencyAvg1mMs != 10 {
			t.Errorf("lat1 should be 10 (only success counted), got %f", s.LatencyAvg1mMs)
		}
		if s.LatencyAvgWindowMs != 10 {
			t.Errorf("latw should be 10, got %f", s.LatencyAvgWindowMs)
		}
	})

	t.Run("large_interval_clamp", func(t *testing.T) {
		samples := []probeSample{
			{latencyMs: 50, success: true},
			{latencyMs: 100, success: true},
			{latencyMs: 150, success: true},
		}
		w := makeWindow(samples)
		s := computeWindowStats(w, 120, "hub")
		if s.Samples1m != 1 {
			t.Errorf("n1 should be 1 for interval>60s, got %d", s.Samples1m)
		}
		if s.LatencyAvg1mMs != 150 {
			t.Errorf("lat1 should use only newest sample (150), got %f", s.LatencyAvg1mMs)
		}
	})

	t.Run("eviction_precise", func(t *testing.T) {
		samples := make([]probeSample, 4)
		samples[0] = probeSample{latencyMs: 10, success: true}
		samples[1] = probeSample{success: false}
		w := &probeWindow{samples: samples, count: 2, pos: 2}

		s1 := computeWindowStats(w, 5, "hub")
		if s1.LossPct != 50 {
			t.Errorf("pre-eviction loss: want 50, got %f", s1.LossPct)
		}

		w.samples[2] = probeSample{latencyMs: 20, success: true}
		w.pos = 3
		w.count = 3
		w.samples[3] = probeSample{latencyMs: 30, success: true}
		w.pos = 0
		w.count = 4

		s2 := computeWindowStats(w, 5, "hub")
		wantLoss := 1.0 / 4.0 * 100
		if s2.LossPct != wantLoss {
			t.Errorf("post-eviction loss: want %f, got %f", wantLoss, s2.LossPct)
		}
	})
}
