package systems

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/henrygd/beszel/internal/entities/system"
)

func setupRegistry(entries map[string][]*subscriptionEntry) func() {
	registry.mu.Lock()
	old := registry.entries
	registry.entries = entries
	registry.mu.Unlock()
	return func() {
		registry.mu.Lock()
		registry.entries = old
		registry.mu.Unlock()
	}
}

func TestComputeIntervalDetailAlways1s(t *testing.T) {
	sm := &SystemManager{}
	cleanup := setupRegistry(map[string][]*subscriptionEntry{
		"sys1": {{kind: kindDetail, connectedClients: 1}},
		"sys2": {{kind: kindList, connectedClients: 1}},
	})
	defer cleanup()

	got := sm.computeInterval()
	if got != 1*time.Second {
		t.Errorf("with detail subscription, interval should be 1s, got %v", got)
	}
}

func TestComputeIntervalListOnly(t *testing.T) {
	sm := &SystemManager{}
	entries := make(map[string][]*subscriptionEntry, 25)
	for i := 0; i < 25; i++ {
		id := string(rune('a' + i))
		entries[id] = []*subscriptionEntry{{kind: kindList, connectedClients: 1}}
	}
	cleanup := setupRegistry(entries)
	defer cleanup()

	got := sm.computeInterval()
	if got != 2*time.Second {
		t.Errorf("25 list-only nodes should give 2s, got %v", got)
	}
}

func TestComputeIntervalFewNodes(t *testing.T) {
	sm := &SystemManager{}
	cleanup := setupRegistry(map[string][]*subscriptionEntry{
		"sys1": {{kind: kindList, connectedClients: 1}},
		"sys2": {{kind: kindList, connectedClients: 1}},
	})
	defer cleanup()

	got := sm.computeInterval()
	if got != 1*time.Second {
		t.Errorf("2 list-only nodes should give 1s, got %v", got)
	}
}

func TestComputeIntervalManyNodes(t *testing.T) {
	sm := &SystemManager{}
	entries := make(map[string][]*subscriptionEntry, 150)
	for i := 0; i < 150; i++ {
		entries[string(rune(i+1000))] = []*subscriptionEntry{{kind: kindList, connectedClients: 1}}
	}
	cleanup := setupRegistry(entries)
	defer cleanup()

	got := sm.computeInterval()
	if got != 5*time.Second {
		t.Errorf("150 list-only nodes should give 5s, got %v", got)
	}
}

func TestComputeIntervalDetailAndListSameNode(t *testing.T) {
	sm := &SystemManager{}
	cleanup := setupRegistry(map[string][]*subscriptionEntry{
		"sys1": {
			{kind: kindList, connectedClients: 1},
			{kind: kindDetail, connectedClients: 1},
		},
	})
	defer cleanup()

	got := sm.computeInterval()
	if got != 1*time.Second {
		t.Errorf("node with both list and detail should give 1s, got %v", got)
	}
}

func TestComputeIntervalDetailWithManyListNodes(t *testing.T) {
	sm := &SystemManager{}
	entries := make(map[string][]*subscriptionEntry, 101)
	for i := 0; i < 100; i++ {
		entries[string(rune(i+1000))] = []*subscriptionEntry{{kind: kindList, connectedClients: 1}}
	}
	entries["detail-node"] = []*subscriptionEntry{{kind: kindDetail, connectedClients: 1}}
	cleanup := setupRegistry(entries)
	defer cleanup()

	got := sm.computeInterval()
	if got != 1*time.Second {
		t.Errorf("any detail subscription should force 1s, got %v", got)
	}
}

func TestBuildListSummaryClearsDeprecated(t *testing.T) {
	data := &system.CombinedData{
		Info: system.Info{
			Hostname:             "test-host",
			KernelVersion:        "5.15.0",
			CpuModel:             "Intel Xeon",
			Cores:                4,
			Cpu:                  25.5,
			MemPct:               60.0,
			DiskPct:              30.0,
			AgentVersion:         "0.9.0",
			Services:             []uint16{5, 1},
			BandwidthByDirection: [2]uint64{1000, 2000},
		},
	}

	summary := buildListSummary("sys1", data)
	if summary.SystemID != "sys1" {
		t.Errorf("expected SystemID sys1, got %s", summary.SystemID)
	}
	if summary.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}
	if summary.Info.Hostname != "" {
		t.Error("Hostname should be cleared in summary")
	}
	if summary.Info.KernelVersion != "" {
		t.Error("KernelVersion should be cleared")
	}
	if summary.Info.CpuModel != "" {
		t.Error("CpuModel should be cleared")
	}
	if summary.Info.Cores != 0 {
		t.Error("Cores should be cleared")
	}
	if summary.Info.Cpu != 25.5 {
		t.Errorf("Cpu should be preserved: got %f", summary.Info.Cpu)
	}
	if summary.Info.BandwidthByDirection != [2]uint64{1000, 2000} {
		t.Error("BandwidthByDirection should be preserved")
	}
	if len(summary.Info.Services) != 2 {
		t.Errorf("Services should be preserved in summary, got len=%d", len(summary.Info.Services))
	}
}

func TestBuildListSummaryDoesNotMutateOriginal(t *testing.T) {
	data := &system.CombinedData{
		Info: system.Info{
			Hostname:     "original-host",
			AgentVersion: "0.9.0",
			Cpu:          50.0,
		},
	}

	_ = buildListSummary("sys1", data)
	if data.Info.Hostname != "original-host" {
		t.Error("buildListSummary should not mutate the original data")
	}
}

func TestBuildListSummaryPayloadSizeUnder1KB(t *testing.T) {
	const port = ":65535"
	maxAddr := strings.Repeat("a", 37) + "😀" +
		strings.Repeat("b", 259-37-len("😀")-len(port)) + port
	if len(maxAddr) != 259 {
		t.Fatalf("max address length: got %d, want 259", len(maxAddr))
	}
	data := &system.CombinedData{
		Info: system.Info{
			Hostname:             "production-web-server-01.datacenter.example.com",
			KernelVersion:        "6.1.0-22-generic",
			CpuModel:             "AMD EPYC 9654 96-Core Processor",
			AgentVersion:         "0.9.0",
			Os:                   1,
			Cores:                96,
			Threads:              192,
			Cpu:                  78.123456,
			MemPct:               65.789012,
			DiskPct:              45.345678,
			Uptime:               8640000,
			Bandwidth:            123456789,
			BandwidthBytes:       123456789,
			BandwidthByDirection: [2]uint64{98765432, 24691357},
			DashboardTemp:        42.5,
			GpuPct:               75.5,
			Battery:              [2]uint8{95, 1},
			Services:             []uint16{15, 3},
			LoadAvg:              [3]float64{1.5, 2.3, 3.1},
			VPSTraffic: &system.VPSTrafficInfo{
				CycleRxBytes:   53687091200,
				CycleTxBytes:   10737418240,
				TotalRxBytes:   1099511627776,
				TotalTxBytes:   549755813888,
				BillableBytes:  53687091200,
				ProjectedBytes: 107374182400,
				QuotaBytes:     1099511627776,
				CycleStart:     "2025-07-01",
				ResetDay:       1,
				DaysLeft:       21,
				BillingMode:    "max_rx_tx",
			},
			VPSProbe: system.VPSProbeStats{
				"abcdefghijklmnop": {Local: true, Updated: 1720000000, Target: maxAddr, Label: "电信联通移动探测移动端可", Position: 1},
				"bcdefghijklmnopq": {LatencyMs: 25.678, LossPct: 1.2, Success: true, Samples: 60, Updated: 1720000000, Target: maxAddr, LatencyAvg1mMs: 26.789, LatencyAvgWindowMs: 27.890, LossPct1m: 1.5, Samples1m: 12, Label: "电信联通移动探测移动端可", Position: 2},
				"cdefghijklmnopqr": {LatencyMs: 18.901, LossPct: 19.99, Success: true, Samples: 60, Updated: 1720000000, Target: maxAddr, LatencyAvg1mMs: 19.012, LatencyAvgWindowMs: 20.123, LossPct1m: 19.99, Samples1m: 12, Label: "电信联通移动探测移动端可", Position: 3},
			},
		},
	}

	summary := buildListSummary("sys-abc123def456", data)

	for k, v := range summary.Info.VPSProbe {
		if v.Target == "" {
			if !v.Local {
				t.Errorf("probe %q: target must be preserved (truncated) in summary", k)
			}
		} else if len(v.Target) > maxSummaryTargetLen {
			t.Errorf("probe %q: target len %d exceeds maxSummaryTargetLen %d", k, len(v.Target), maxSummaryTargetLen)
		} else if !utf8.ValidString(v.Target) {
			t.Errorf("probe %q: target is not valid UTF-8: %q", k, v.Target)
		} else if !strings.HasSuffix(v.Target, summaryTargetEllipsis) {
			t.Errorf("probe %q: truncated target should end with %q, got %q", k, summaryTargetEllipsis, v.Target)
		}
	}
	if summary.Info.AgentVersion != data.Info.AgentVersion {
		t.Errorf("AgentVersion should be preserved: got %q, want %q", summary.Info.AgentVersion, data.Info.AgentVersion)
	}

	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal summary: %v", err)
	}

	const maxBytes = 1024
	if len(payload) > maxBytes {
		t.Errorf("summary payload is %d bytes, exceeds %d byte limit.\nPayload: %s", len(payload), maxBytes, string(payload))
	}
	t.Logf("summary payload size: %d bytes (limit %d)", len(payload), maxBytes)
}

func TestTruncateSummaryTargetUTF8(t *testing.T) {
	short := "探测.example.com:443"
	if got := truncateSummaryTarget(short); got != short {
		t.Errorf("short target changed: got %q, want %q", got, short)
	}

	long := strings.Repeat("a", 37) + "😀" + strings.Repeat("b", 20)
	got := truncateSummaryTarget(long)
	if len(got) > maxSummaryTargetLen {
		t.Errorf("truncated target len %d exceeds %d", len(got), maxSummaryTargetLen)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated target is not valid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, summaryTargetEllipsis) {
		t.Errorf("truncated target should end with %q, got %q", summaryTargetEllipsis, got)
	}
	if strings.ContainsRune(got, utf8.RuneError) {
		t.Errorf("truncated target contains a replacement rune: %q", got)
	}
}

func TestBuildListSummaryProbeFieldsPreserved(t *testing.T) {
	data := &system.CombinedData{
		Info: system.Info{
			VPSProbe: system.VPSProbeStats{
				"cn": {
					LatencyMs: 50.0, LossPct: 2.5, Success: true, Samples: 60,
					Updated: 1720000000, Target: "long.address.example.com:443",
					LatencyAvg1mMs: 48.0, LatencyAvgWindowMs: 52.0,
					LossPct1m: 3.0, Samples1m: 12,
					Local: false, Label: "中国电信", Position: 1,
				},
				"hk": {
					LatencyMs: 10.0, LossPct: 0, Success: true, Samples: 30,
					Updated: 1720000001, Target: "hk.example.com:80",
					LatencyAvgWindowMs: 11.0,
					Local:              true, Label: "HK Local", Position: 2,
				},
			},
		},
	}
	summary := buildListSummary("sys-probe-fields", data)
	vp := summary.Info.VPSProbe
	if vp == nil {
		t.Fatal("VPSProbe should not be nil")
	}
	cn := vp["cn"]
	if cn.Label != "中国电信" {
		t.Errorf("cn label: got %q, want 中国电信", cn.Label)
	}
	if cn.Position != 1 {
		t.Errorf("cn pos: got %d, want 1", cn.Position)
	}
	if cn.LatencyAvgWindowMs != 52.0 {
		t.Errorf("cn latw: got %f, want 52.0", cn.LatencyAvgWindowMs)
	}
	if cn.LossPct != 2.5 {
		t.Errorf("cn loss: got %f, want 2.5", cn.LossPct)
	}
	if cn.Target != "long.address.example.com:443" {
		t.Errorf("cn target should be preserved, got %q", cn.Target)
	}
	if cn.Success != true {
		t.Error("cn ok should be preserved")
	}
	if cn.Samples != 60 {
		t.Errorf("cn samples: got %d, want 60", cn.Samples)
	}
	if cn.Updated != 1720000000 {
		t.Errorf("cn ts: got %d, want 1720000000", cn.Updated)
	}
	if cn.LatencyMs != 50.0 {
		t.Errorf("cn lat: got %f, want 50.0", cn.LatencyMs)
	}
	if cn.LatencyAvg1mMs != 0 {
		t.Errorf("cn lat1m should be stripped, got %f", cn.LatencyAvg1mMs)
	}
	hk := vp["hk"]
	if hk.Label != "HK Local" {
		t.Errorf("hk label: got %q, want HK Local", hk.Label)
	}
	if !hk.Local {
		t.Error("hk local flag should be preserved")
	}
	if hk.Position != 2 {
		t.Errorf("hk pos: got %d, want 2", hk.Position)
	}
	if hk.Target != "hk.example.com:80" {
		t.Errorf("hk target should be preserved, got %q", hk.Target)
	}
}
