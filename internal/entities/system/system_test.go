package system

import (
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

func TestVPSProbeTargetStatsJSONRoundTrip(t *testing.T) {
	original := VPSProbeTargetStats{
		LatencyMs:          42.5,
		LossPct:            3.2,
		Success:            true,
		Samples:            60,
		Updated:            1720000000,
		Target:             "example.com:443",
		LatencyAvg1mMs:     43.0,
		LatencyAvgWindowMs: 44.0,
		LossPct1m:          2.0,
		Samples1m:          12,
		Local:              false,
		Label:              "电信联通",
		Position:           2,
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded VPSProbeTargetStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if decoded.Label != original.Label {
		t.Errorf("Label: got %q, want %q", decoded.Label, original.Label)
	}
	if decoded.Position != original.Position {
		t.Errorf("Position: got %d, want %d", decoded.Position, original.Position)
	}
	if decoded.LatencyMs != original.LatencyMs {
		t.Errorf("LatencyMs: got %f, want %f", decoded.LatencyMs, original.LatencyMs)
	}
	if decoded.Target != original.Target {
		t.Errorf("Target: got %q, want %q", decoded.Target, original.Target)
	}
}

func TestVPSProbeTargetStatsJSONOmitEmpty(t *testing.T) {
	empty := VPSProbeTargetStats{Success: true}
	data, err := json.Marshal(empty)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	s := string(data)
	if contains(s, `"label"`) {
		t.Error("empty Label should be omitted from JSON")
	}
	if contains(s, `"pos"`) {
		t.Error("zero Position should be omitted from JSON")
	}
}

func TestVPSProbeStatsMapJSONRoundTrip(t *testing.T) {
	original := VPSProbeStats{
		"cn": {Label: "CN", Position: 1, LatencyMs: 10},
		"hk": {Label: "HK", Position: 2, Local: true},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	var decoded VPSProbeStats
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if len(decoded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(decoded))
	}
	cn := decoded["cn"]
	if cn.Label != "CN" || cn.Position != 1 {
		t.Errorf("cn: got label=%q pos=%d, want CN/1", cn.Label, cn.Position)
	}
	hk := decoded["hk"]
	if hk.Label != "HK" || hk.Position != 2 || !hk.Local {
		t.Errorf("hk: got label=%q pos=%d local=%v, want HK/2/true", hk.Label, hk.Position, hk.Local)
	}
}

func TestVPSProbeTargetStatsCBORRoundTrip(t *testing.T) {
	original := VPSProbeTargetStats{
		LatencyMs:          42.5,
		LossPct:            3.2,
		Success:            true,
		Samples:            60,
		Updated:            1720000000,
		Target:             "example.com:443",
		LatencyAvg1mMs:     43.0,
		LatencyAvgWindowMs: 44.0,
		LossPct1m:          2.0,
		Samples1m:          12,
		Local:              false,
		Label:              "电信联通",
		Position:           2,
	}
	data, err := cbor.Marshal(original)
	if err != nil {
		t.Fatalf("cbor marshal error: %v", err)
	}
	var decoded VPSProbeTargetStats
	if err := cbor.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("cbor unmarshal error: %v", err)
	}
	if decoded.Label != original.Label {
		t.Errorf("Label: got %q, want %q", decoded.Label, original.Label)
	}
	if decoded.Position != original.Position {
		t.Errorf("Position: got %d, want %d", decoded.Position, original.Position)
	}
	if decoded.LatencyMs != original.LatencyMs {
		t.Errorf("LatencyMs: got %f, want %f", decoded.LatencyMs, original.LatencyMs)
	}
	if decoded.Target != original.Target {
		t.Errorf("Target: got %q, want %q", decoded.Target, original.Target)
	}
	if decoded.LatencyAvgWindowMs != original.LatencyAvgWindowMs {
		t.Errorf("LatencyAvgWindowMs: got %f, want %f", decoded.LatencyAvgWindowMs, original.LatencyAvgWindowMs)
	}
	if decoded.LossPct != original.LossPct {
		t.Errorf("LossPct: got %f, want %f", decoded.LossPct, original.LossPct)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && jsonContains(s, substr)
}

func jsonContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
