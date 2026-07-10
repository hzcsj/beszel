package agent

import (
	"context"
	"encoding/json"
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
