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

func TestValidateTargetID(t *testing.T) {
	valid := []string{"cn", "hk", "my-probe", "A_1", "abcdefghijklmnop"}
	for _, id := range valid {
		if err := validateTargetID(id); err != nil {
			t.Errorf("expected valid id %q, got err: %v", id, err)
		}
	}
	invalid := []string{"", "has space", "abcdefghijklmnopq", "日本語", "a/b"}
	for _, id := range invalid {
		if err := validateTargetID(id); err == nil {
			t.Errorf("expected invalid id %q to fail", id)
		}
	}
}

func TestValidateTargetLabel(t *testing.T) {
	valid := []string{"CN", "HK", "电信联通移动探测移动端可", "Ab"}
	for _, l := range valid {
		if err := validateTargetLabel(l); err != nil {
			t.Errorf("expected valid label %q, got err: %v", l, err)
		}
	}
	invalid := []string{"", "电信联通移动探测移动端可达", "has\x00null"}
	for _, l := range invalid {
		if err := validateTargetLabel(l); err == nil {
			t.Errorf("expected invalid label %q to fail", l)
		}
	}
}

func TestValidateTargetAddress(t *testing.T) {
	valid := []string{"1.2.3.4:80", "example.com:443", "[::1]:8080", "host.internal:35601"}
	for _, a := range valid {
		if err := validateTargetAddress(a); err != nil {
			t.Errorf("expected valid address %q, got err: %v", a, err)
		}
	}
	invalid := []string{"", "noport", ":80", "host:0", "host:99999", "host:http", "host:-1"}
	for _, a := range invalid {
		if err := validateTargetAddress(a); err == nil {
			t.Errorf("expected invalid address %q to fail", a)
		}
	}

	long := string(make([]byte, maxAddressLen+1))
	if err := validateTargetAddress(long + ":80"); err == nil {
		t.Error("expected overlong address to fail")
	}
}

func TestParseArrayTargets0Targets(t *testing.T) {
	raw := json.RawMessage(`[]`)
	result := parseArrayTargets(raw)
	if len(result) != 0 {
		t.Errorf("expected 0 targets, got %d", len(result))
	}
}

func TestParseArrayTargets1Target(t *testing.T) {
	raw := json.RawMessage(`[{"id":"cn","label":"CN","address":"1.2.3.4:80"}]`)
	result := parseArrayTargets(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 target, got %d", len(result))
	}
	if result[0].id != "cn" || result[0].label != "CN" || result[0].pos != 1 {
		t.Errorf("unexpected target: %+v", result[0])
	}
}

func TestParseArrayTargets3Targets(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"ct","label":"CT","address":"ct.example.com:80"},
		{"id":"cu","label":"CU","address":"cu.example.com:80"},
		{"id":"cm","label":"CM","address":"cm.example.com:80"}
	]`)
	result := parseArrayTargets(raw)
	if len(result) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(result))
	}
	for i, want := range []string{"ct", "cu", "cm"} {
		if result[i].id != want {
			t.Errorf("target %d: want id %q, got %q", i, want, result[i].id)
		}
		if result[i].pos != uint8(i+1) {
			t.Errorf("target %d: want pos %d, got %d", i, i+1, result[i].pos)
		}
	}
}

func TestParseArrayTargetsFourAccepted(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"a","label":"A","address":"1.2.3.4:80"},
		{"id":"b","label":"B","address":"1.2.3.4:80"},
		{"id":"c","label":"C","address":"1.2.3.4:80"},
		{"id":"d","label":"D","address":"1.2.3.4:80"}
	]`)
	result := parseArrayTargets(raw)
	if len(result) != 4 {
		t.Fatalf("expected 4 targets, got %d", len(result))
	}
	for i, target := range result {
		if target.pos != uint8(i+1) {
			t.Errorf("target %d: got pos %d, want %d", i, target.pos, i+1)
		}
	}
}

func TestParseArrayTargetsOver4Rejected(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"a","label":"A","address":"1.2.3.4:80"},
		{"id":"b","label":"B","address":"1.2.3.4:80"},
		{"id":"c","label":"C","address":"1.2.3.4:80"},
		{"id":"d","label":"D","address":"1.2.3.4:80"},
		{"id":"e","label":"E","address":"1.2.3.4:80"}
	]`)
	result := parseArrayTargets(raw)
	if result != nil {
		t.Errorf("expected nil for >4 targets, got %d", len(result))
	}
}

func TestParseArrayTargetsDuplicateID(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"cn","label":"CN1","address":"1.2.3.4:80"},
		{"id":"cn","label":"CN2","address":"5.6.7.8:80"}
	]`)
	result := parseArrayTargets(raw)
	if len(result) != 1 {
		t.Errorf("expected 1 target after dedup, got %d", len(result))
	}
}

func TestParseArrayTargetsInvalidIDSkipped(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"日本","label":"JP","address":"1.2.3.4:80"},
		{"id":"cn","label":"CN","address":"5.6.7.8:80"}
	]`)
	result := parseArrayTargets(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 valid target, got %d", len(result))
	}
	if result[0].id != "cn" {
		t.Errorf("expected cn, got %s", result[0].id)
	}
}

func TestParseArrayTargetsLocalTarget(t *testing.T) {
	raw := json.RawMessage(`[{"id":"local","label":"Self","local":true}]`)
	result := parseArrayTargets(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 target, got %d", len(result))
	}
	if !result[0].local {
		t.Error("expected local=true")
	}
}

func TestParseArrayTargetsOrderPreserved(t *testing.T) {
	raw := json.RawMessage(`[
		{"id":"cm","label":"CM","address":"cm.example.com:80"},
		{"id":"ct","label":"CT","address":"ct.example.com:80"},
		{"id":"cu","label":"CU","address":"cu.example.com:80"},
		{"id":"hk","label":"HK","address":"hk.example.com:80"}
	]`)
	result := parseArrayTargets(raw)
	expected := []string{"cm", "ct", "cu", "hk"}
	for i, want := range expected {
		if result[i].id != want {
			t.Errorf("position %d: want %q, got %q", i, want, result[i].id)
		}
	}
}

func TestParseArrayTargetsIPv6(t *testing.T) {
	raw := json.RawMessage(`[{"id":"v6","label":"IPv6","address":"[::1]:8080"}]`)
	result := parseArrayTargets(raw)
	if len(result) != 1 {
		t.Fatalf("expected 1 target, got %d", len(result))
	}
	if result[0].address != "[::1]:8080" {
		t.Errorf("unexpected address: %s", result[0].address)
	}
}

func TestParseLegacyMapTargets(t *testing.T) {
	raw := json.RawMessage(`{"hub":"10.0.0.1:22","ct":"10.0.0.2:80"}`)
	result := parseLegacyMapTargets(raw, false)
	if len(result) != 4 {
		t.Fatalf("legacy map should produce 4 canonical targets, got %d", len(result))
	}
	if result[0].id != "hub" || result[0].address != "10.0.0.1:22" {
		t.Errorf("hub: %+v", result[0])
	}
	if result[0].label != "HUB" {
		t.Errorf("hub label should be HUB, got %s", result[0].label)
	}
}

func TestParseLegacyMapTargetsHubLocal(t *testing.T) {
	raw := json.RawMessage(`{"hub":"10.0.0.1:22"}`)
	result := parseLegacyMapTargets(raw, true)
	hub := result[0]
	if !hub.local {
		t.Error("hub should be local when hubLocal=true")
	}
}

func TestNewVPSProbeCollectorNoEnvVar(t *testing.T) {
	for _, k := range []string{"BESZEL_AGENT_VPS_PROBE_CONFIG", "VPS_PROBE_CONFIG"} {
		t.Setenv(k, "")
	}
	// Unset to simulate absence
	c := newVPSProbeCollector()
	if c != nil {
		t.Error("collector should be nil when env var is absent")
	}
}

func TestNewVPSProbeCollectorDisabledExplicitly(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"enabled":false,"targets":[{"id":"cn","label":"CN","address":"1.2.3.4:80"}]}`)
	c := newVPSProbeCollector()
	if c != nil {
		t.Error("collector should be nil when explicitly disabled")
	}
}

func TestNewVPSProbeCollectorInvalidJSON(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `not valid json`)
	c := newVPSProbeCollector()
	if c != nil {
		t.Error("collector should be nil on invalid JSON")
	}
}

func TestNewVPSProbeCollectorNewArrayFormat(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{
		"intervalSeconds":10,"timeoutMs":500,"windowSize":30,
		"targets":[
			{"id":"ct","label":"CT","address":"ct.example.com:80"},
			{"id":"cu","label":"CU","address":"cu.example.com:80"}
		]
	}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil")
	}
	if len(c.targets) != 2 {
		t.Errorf("expected 2 targets, got %d", len(c.targets))
	}
	if c.interval.Seconds() != 10 {
		t.Errorf("expected 10s interval, got %v", c.interval)
	}
	if c.timeout.Milliseconds() != 500 {
		t.Errorf("expected 500ms timeout, got %v", c.timeout)
	}
	if c.windowSize != 30 {
		t.Errorf("expected windowSize 30, got %d", c.windowSize)
	}
}

func TestNewVPSProbeCollectorLegacyFormat(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"targets":{"hub":"10.0.0.1:22"}}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil with legacy format")
	}
	if len(c.targets) != 4 {
		t.Errorf("legacy format should produce 4 targets, got %d", len(c.targets))
	}
}

func TestNewVPSProbeCollectorLegacyNoTargetsField(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"hubLocal":true}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil when targets field omitted (legacy compat)")
	}
	if len(c.targets) != 4 {
		t.Errorf("legacy no-targets should produce 4 canonical targets, got %d", len(c.targets))
	}
	if !c.targets[0].local {
		t.Error("hub should be local when hubLocal=true")
	}
}

func TestNewVPSProbeCollectorLegacyIntervalOnly(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":15}`)
	c := newVPSProbeCollector()
	if c == nil {
		t.Fatal("collector should be non-nil when only interval configured")
	}
	if len(c.targets) != 4 {
		t.Errorf("expected 4 canonical targets, got %d", len(c.targets))
	}
}

func TestNewVPSProbeCollectorExplicitEmptyArray(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"targets":[]}`)
	c := newVPSProbeCollector()
	if c != nil {
		t.Error("explicit empty targets array should disable probes (nil collector)")
	}
}

func TestNewVPSProbeCollectorClamping(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{
		"intervalSeconds":999,"timeoutMs":1,"windowSize":5,
		"targets":[{"id":"cn","label":"CN","address":"1.2.3.4:80"}]
	}`)
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
	if c.windowSize != 10 {
		t.Errorf("windowSize should be clamped to 10, got %d", c.windowSize)
	}
}

func TestProbeCollectorStartStop(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":[{"id":"hub","label":"HUB","address":"192.0.2.1:1"}]}`)
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
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":[{"id":"hub","label":"HUB","address":"192.0.2.1:1"}]}`)
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
}

func TestProbeCollectorStopBeforeStart(t *testing.T) {
	t.Setenv("BESZEL_AGENT_VPS_PROBE_CONFIG", `{"intervalSeconds":300,"targets":[{"id":"hub","label":"HUB","address":"192.0.2.1:1"}]}`)
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

func newTestCollector(t *testing.T, addr string, windowSize, intervalSec int) *VPSProbeCollector {
	t.Helper()
	return &VPSProbeCollector{
		targets: []resolvedTarget{
			{id: "hub", label: "HUB", address: addr, pos: 1},
		},
		windows: map[string]*probeWindow{
			"hub": {samples: make([]probeSample, windowSize)},
		},
		latest:    make(system.VPSProbeStats),
		timeout:   2 * time.Second,
		intervalS: intervalSec,
	}
}

func TestProbeAllConcurrentDynamicTargets(t *testing.T) {
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
		targets: []resolvedTarget{
			{id: "ct", label: "CT", address: ln.Addr().String(), pos: 1},
			{id: "cu", label: "CU", address: refusedAddr, pos: 2},
			{id: "cm", label: "CM", address: "192.0.2.1:1", pos: 3},
		},
		windows: map[string]*probeWindow{
			"ct": {samples: make([]probeSample, 10)},
			"cu": {samples: make([]probeSample, 10)},
			"cm": {samples: make([]probeSample, 10)},
		},
		latest:    make(system.VPSProbeStats, 3),
		timeout:   200 * time.Millisecond,
		intervalS: 5,
	}

	c.probeAll(context.Background())

	ct := c.latest["ct"]
	if !ct.Success {
		t.Error("ct (open listener) should succeed")
	}
	if ct.Label != "CT" {
		t.Errorf("ct label: want CT, got %s", ct.Label)
	}
	if ct.Position != 1 {
		t.Errorf("ct pos: want 1, got %d", ct.Position)
	}

	cu := c.latest["cu"]
	if !cu.Success {
		t.Error("cu (refused port) should succeed")
	}

	cm := c.latest["cm"]
	if cm.Success {
		t.Error("cm (unreachable) should fail")
	}
	if cm.LossPct != 100 {
		t.Errorf("cm loss should be 100%%, got %f%%", cm.LossPct)
	}
}

func TestProbeAllLocalTargetSkipsDial(t *testing.T) {
	c := &VPSProbeCollector{
		targets: []resolvedTarget{
			{id: "self", label: "Self", local: true, pos: 1},
			{id: "ct", label: "CT", address: "192.0.2.1:1", pos: 2},
		},
		windows: map[string]*probeWindow{
			"self": {samples: make([]probeSample, 10)},
			"ct":   {samples: make([]probeSample, 10)},
		},
		latest:    make(system.VPSProbeStats, 2),
		timeout:   100 * time.Millisecond,
		intervalS: 5,
	}

	c.probeAll(context.Background())

	self := c.latest["self"]
	if !self.Local {
		t.Error("self should be Local")
	}
	if self.Label != "Self" {
		t.Errorf("self label: want Self, got %s", self.Label)
	}
	if self.Position != 1 {
		t.Errorf("self pos: want 1, got %d", self.Position)
	}
	if c.windows["self"].count != 0 {
		t.Error("local target window should not have samples")
	}
}

func TestProbeWindowRollingLoss(t *testing.T) {
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
	newC := func() *VPSProbeCollector {
		return &VPSProbeCollector{
			targets: []resolvedTarget{
				{id: "hub", label: "HUB", address: ln.Addr().String(), pos: 1},
			},
			windows: map[string]*probeWindow{
				"hub": {samples: make([]probeSample, windowSize)},
			},
			latest:    make(system.VPSProbeStats),
			timeout:   2 * time.Second,
			intervalS: 5,
		}
	}

	t.Run("all_success", func(t *testing.T) {
		c := newC()
		for i := 0; i < windowSize; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.Samples != windowSize {
			t.Errorf("expected %d samples, got %d", windowSize, hub.Samples)
		}
		if hub.LossPct != 0 {
			t.Errorf("expected 0%% loss, got %f%%", hub.LossPct)
		}
	})

	t.Run("mixed_success_failure", func(t *testing.T) {
		c := newC()
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
	})

	t.Run("eviction_recovery", func(t *testing.T) {
		c := newC()
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)
		c.probeAll(context.Background())
		c.probeAll(cancelledCtx)

		for i := 0; i < windowSize; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.LossPct != 0 {
			t.Errorf("expected 0%% loss after eviction, got %f%%", hub.LossPct)
		}
	})
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
		c := newTestCollector(t, ln.Addr().String(), 60, 5)
		c.probeAll(context.Background())
		hub := c.latest["hub"]
		if hub.LatencyMs <= 0 {
			t.Error("raw lat should be positive on success")
		}
	})

	t.Run("one_minute_mean_recent_samples", func(t *testing.T) {
		c := newTestCollector(t, ln.Addr().String(), 60, 5)
		for i := 0; i < 20; i++ {
			c.probeAll(context.Background())
		}
		hub := c.latest["hub"]
		if hub.LatencyAvg1mMs <= 0 {
			t.Error("lat1 should be positive after 20 probes")
		}
		if hub.LatencyAvgWindowMs <= 0 {
			t.Error("latw should be positive after 20 probes")
		}
	})

	t.Run("no_success_omits_both_means", func(t *testing.T) {
		c := newTestCollector(t, ln.Addr().String(), 4, 5)
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		for i := 0; i < 4; i++ {
			c.probeAll(cancelledCtx)
		}
		hub := c.latest["hub"]
		if hub.LatencyAvg1mMs != 0 {
			t.Errorf("lat1 should be 0, got %f", hub.LatencyAvg1mMs)
		}
		if hub.LatencyAvgWindowMs != 0 {
			t.Errorf("latw should be 0, got %f", hub.LatencyAvgWindowMs)
		}
		if hub.LossPct != 100 {
			t.Errorf("expected 100%% loss, got %f%%", hub.LossPct)
		}
	})
}

func TestLabelAndPositionInResults(t *testing.T) {
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

	c := &VPSProbeCollector{
		targets: []resolvedTarget{
			{id: "cn", label: "中国", address: ln.Addr().String(), pos: 1},
			{id: "hk", label: "HK", address: ln.Addr().String(), pos: 2},
		},
		windows: map[string]*probeWindow{
			"cn": {samples: make([]probeSample, 10)},
			"hk": {samples: make([]probeSample, 10)},
		},
		latest:    make(system.VPSProbeStats),
		timeout:   2 * time.Second,
		intervalS: 5,
	}
	c.probeAll(context.Background())

	cn := c.latest["cn"]
	if cn.Label != "中国" {
		t.Errorf("cn label: want '中国', got %q", cn.Label)
	}
	if cn.Position != 1 {
		t.Errorf("cn pos: want 1, got %d", cn.Position)
	}

	hk := c.latest["hk"]
	if hk.Label != "HK" {
		t.Errorf("hk label: want 'HK', got %q", hk.Label)
	}
	if hk.Position != 2 {
		t.Errorf("hk pos: want 2, got %d", hk.Position)
	}
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
			t.Errorf("lat1 should be 0 when all failed, got %f", s.LatencyAvg1mMs)
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
			t.Errorf("lat1 should be 10, got %f", s.LatencyAvg1mMs)
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
			t.Errorf("lat1 should use only newest sample, got %f", s.LatencyAvg1mMs)
		}
	})
}

func TestCanonicalProbeKeysLength(t *testing.T) {
	if len(canonicalProbeKeys) != 4 {
		t.Errorf("expected 4 canonical keys, got %d", len(canonicalProbeKeys))
	}
}
