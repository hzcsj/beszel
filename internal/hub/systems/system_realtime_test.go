package systems

import (
	"encoding/json"
	"testing"
	"time"

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
				"hub": {Local: true, Updated: 1720000000, Target: "hub.example.com:22"},
				"ct":  {LatencyMs: 25.678, LossPct: 1.2, Success: true, Samples: 60, Updated: 1720000000, Target: "ct.tz.example.com:80", LatencyAvg1mMs: 26.789, LatencyAvgWindowMs: 27.890, LossPct1m: 1.5, Samples1m: 12},
				"cu":  {LatencyMs: 18.901, LossPct: 0, Success: true, Samples: 60, Updated: 1720000000, Target: "cu.tz.example.com:80", LatencyAvg1mMs: 19.012, LatencyAvgWindowMs: 20.123, LossPct1m: 0, Samples1m: 12},
				"cm":  {LatencyMs: 35.234, LossPct: 3.5, Success: true, Samples: 60, Updated: 1720000000, Target: "cm.tz.example.com:80", LatencyAvg1mMs: 36.345, LatencyAvgWindowMs: 37.456, LossPct1m: 4.2, Samples1m: 12},
			},
		},
	}

	summary := buildListSummary("sys-abc123def456", data)
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
