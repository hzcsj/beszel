package systems

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClampResetDay(t *testing.T) {
	tests := []struct{ in, want int }{
		{0, 1}, {-5, 1}, {1, 1}, {16, 16}, {28, 28}, {29, 28}, {31, 28}, {100, 28},
	}
	for _, tt := range tests {
		if got := clampResetDay(tt.in); got != tt.want {
			t.Errorf("clampResetDay(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestComputeBillable(t *testing.T) {
	tests := []struct {
		rx, tx uint64
		mode   string
		want   uint64
	}{
		{100, 200, BillingModeMaxRxTx, 200},
		{300, 200, BillingModeMaxRxTx, 300},
		{100, 200, BillingModeSumRxTx, 300},
		{100, 200, BillingModeTxOnly, 200},
		{100, 200, BillingModeRxOnly, 100},
		{100, 200, "", 200},
	}
	for _, tt := range tests {
		if got := computeBillable(tt.rx, tt.tx, tt.mode); got != tt.want {
			t.Errorf("computeBillable(%d, %d, %q) = %d, want %d", tt.rx, tt.tx, tt.mode, got, tt.want)
		}
	}
}

func TestComputeCycleStartAndKey(t *testing.T) {
	loc := time.Local
	tests := []struct {
		now       time.Time
		resetDay  int
		wantStart string
		wantKey   string
	}{
		{time.Date(2025, 7, 20, 10, 0, 0, 0, loc), 1, "2025-07-01", "2025-07-01"},
		{time.Date(2025, 7, 1, 0, 0, 0, 0, loc), 1, "2025-07-01", "2025-07-01"},
		{time.Date(2025, 7, 20, 10, 0, 0, 0, loc), 16, "2025-07-16", "2025-07-16"},
		{time.Date(2025, 7, 15, 23, 0, 0, 0, loc), 16, "2025-06-16", "2025-06-16"},
		{time.Date(2025, 1, 5, 0, 0, 0, 0, loc), 16, "2024-12-16", "2024-12-16"},
	}
	for _, tt := range tests {
		cs := computeCycleStart(tt.now, tt.resetDay)
		if got := cs.Format("2006-01-02"); got != tt.wantStart {
			t.Errorf("computeCycleStart(%v, %d) = %s, want %s", tt.now, tt.resetDay, got, tt.wantStart)
		}
		ck := computeCycleKey(tt.now, tt.resetDay)
		if ck != tt.wantKey {
			t.Errorf("computeCycleKey(%v, %d) = %s, want %s", tt.now, tt.resetDay, ck, tt.wantKey)
		}
	}
}

func TestNextCycleStart(t *testing.T) {
	loc := time.Local
	tests := []struct {
		now      time.Time
		resetDay int
		want     string
	}{
		{time.Date(2025, 7, 20, 10, 0, 0, 0, loc), 1, "2025-08-01"},
		{time.Date(2025, 7, 15, 23, 0, 0, 0, loc), 16, "2025-07-16"},
		{time.Date(2025, 12, 20, 0, 0, 0, 0, loc), 1, "2026-01-01"},
	}
	for _, tt := range tests {
		nc := nextCycleStart(tt.now, tt.resetDay)
		if got := nc.Format("2006-01-02"); got != tt.want {
			t.Errorf("nextCycleStart(%v, %d) = %s, want %s", tt.now, tt.resetDay, got, tt.want)
		}
	}
}

func TestFirstSampleInitialization(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"quotaBytes":2199023255552,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	ni := map[string][4]uint64{
		"eth0": {1000, 500, 1_000_000, 2_000_000},
	}

	info := m.DeriveTraffic("sys1", "hzcsj", nil, ni)
	if info == nil {
		t.Fatal("expected non-nil VPSTrafficInfo on first sample")
	}
	if info.CycleRxBytes != 0 || info.CycleTxBytes != 0 {
		t.Errorf("first sample should show zero cycle: got crx=%d, ctx=%d", info.CycleRxBytes, info.CycleTxBytes)
	}
	if info.QuotaBytes != 2199023255552 {
		t.Errorf("quota mismatch: got %d", info.QuotaBytes)
	}
	if info.BillingMode != BillingModeMaxRxTx {
		t.Errorf("billing mode mismatch: got %s", info.BillingMode)
	}
}

func TestNormalPositiveDeltas(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"sum_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	ni1 := map[string][4]uint64{"eth0": {0, 0, 100_000, 200_000}}
	m.DeriveTraffic("sys1", "node1", nil, ni1)

	ni2 := map[string][4]uint64{"eth0": {0, 0, 150_000, 300_000}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni2)
	if info == nil {
		t.Fatal("expected non-nil")
	}
	if info.CycleTxBytes != 50_000 {
		t.Errorf("expected CycleTx=50000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 100_000 {
		t.Errorf("expected CycleRx=100000, got %d", info.CycleRxBytes)
	}
	if info.BillableBytes != 150_000 {
		t.Errorf("expected BillableBytes=150000 (sum), got %d", info.BillableBytes)
	}
	if info.TotalTxBytes != 50_000 || info.TotalRxBytes != 100_000 {
		t.Errorf("totals mismatch: ttx=%d trx=%d", info.TotalTxBytes, info.TotalRxBytes)
	}
}

func TestCounterReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	ni1 := map[string][4]uint64{"eth0": {0, 0, 1_000_000, 2_000_000}}
	m.DeriveTraffic("sys1", "node1", nil, ni1)

	ni2 := map[string][4]uint64{"eth0": {0, 0, 1_500_000, 2_500_000}}
	m.DeriveTraffic("sys1", "node1", nil, ni2)

	// simulate reboot: counters drop
	ni3 := map[string][4]uint64{"eth0": {0, 0, 50_000, 30_000}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni3)
	if info == nil {
		t.Fatal("expected non-nil")
	}
	if info.CycleTxBytes != 550_000 {
		t.Errorf("expected CycleTx=550000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 530_000 {
		t.Errorf("expected CycleRx=530000, got %d", info.CycleRxBytes)
	}
}

// TestCounterResetZero verifies that a counter dropping to zero (e.g. NIC
// driver re-init) does not create a spike: delta should be 0.
func TestCounterResetZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1}}`)
	m := NewVPSTrafficManager(dir)

	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 500_000, 600_000}})
	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 700_000, 800_000}})

	info := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 0, 0}})
	// counter dropped to 0 → delta must be 0, not the old total
	if info.CycleTxBytes != 200_000 || info.CycleRxBytes != 200_000 {
		t.Errorf("zero counter should not spike: ctx=%d crx=%d", info.CycleTxBytes, info.CycleRxBytes)
	}
}

func TestMonthReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	m.mu.Lock()
	m.states["sys1"] = &VPSTrafficState{
		SystemID:   "sys1",
		SystemName: "node1",
		LastNicRx:  map[string]uint64{"eth0": 1_000_000},
		LastNicTx:  map[string]uint64{"eth0": 2_000_000},
		CycleRx:    500_000,
		CycleTx:    800_000,
		TotalRx:    3_000_000,
		TotalTx:    5_000_000,
		CycleKey:   "2025-06-01",
		UpdatedAt:  time.Now().Unix(),
	}
	m.mu.Unlock()

	// Cross-cycle sample: counters grew since last observation
	ni := map[string][4]uint64{"eth0": {0, 0, 2_100_000, 1_100_000}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni)
	if info == nil {
		t.Fatal("expected non-nil")
	}

	// Delta: tx = 2_100_000 - 2_000_000 = 100_000, rx = 1_100_000 - 1_000_000 = 100_000
	// Cross-cycle delta belongs to new cycle
	if info.CycleTxBytes != 100_000 {
		t.Errorf("cross-cycle CycleTx: want 100000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 100_000 {
		t.Errorf("cross-cycle CycleRx: want 100000, got %d", info.CycleRxBytes)
	}
	// Total must always accumulate (old total + delta)
	if info.TotalTxBytes != 5_100_000 {
		t.Errorf("TotalTx: want 5100000, got %d", info.TotalTxBytes)
	}
	if info.TotalRxBytes != 3_100_000 {
		t.Errorf("TotalRx: want 3100000, got %d", info.TotalRxBytes)
	}
}

func TestPerNodeResetDay(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"default":{"resetDay":1,"billingMode":"max_rx_tx"},"systems":{"us":{"resetDay":16}}}`
	t.Setenv("VPS_TRAFFIC_CONFIG", cfg)
	m := NewVPSTrafficManager(dir)

	ncDefault := m.getNodeConfig("sys1", "hzcsj", nil)
	if ncDefault.ResetDay != 1 {
		t.Errorf("expected default resetDay=1, got %d", ncDefault.ResetDay)
	}
	ncUS := m.getNodeConfig("sys2", "us", nil)
	if ncUS.ResetDay != 16 {
		t.Errorf("expected us resetDay=16, got %d", ncUS.ResetDay)
	}
}

func TestInvalidConfigFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `invalid json!!!`)
	m := NewVPSTrafficManager(dir)

	// Manager must still work with built-in defaults
	ni := map[string][4]uint64{"eth0": {0, 0, 100, 200}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni)
	if info == nil {
		t.Fatal("invalid config should fall back to defaults, not disable")
	}
	if info.BillingMode != BillingModeMaxRxTx {
		t.Errorf("expected default billing mode, got %s", info.BillingMode)
	}
	if info.ResetDay != 1 {
		t.Errorf("expected default resetDay=1, got %d", info.ResetDay)
	}
}

func TestEmptyConfigUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", "")
	m := NewVPSTrafficManager(dir)

	ni := map[string][4]uint64{"eth0": {0, 0, 100, 200}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni)
	if info == nil {
		t.Fatal("empty config should still enable traffic tracking with defaults")
	}
	if info.BillingMode != BillingModeMaxRxTx {
		t.Errorf("expected default billing mode, got %s", info.BillingMode)
	}
}

func TestStatePersistenceAndReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	ni1 := map[string][4]uint64{"eth0": {0, 0, 100_000, 200_000}}
	m.DeriveTraffic("sys1", "node1", nil, ni1)
	ni2 := map[string][4]uint64{"eth0": {0, 0, 150_000, 300_000}}
	m.DeriveTraffic("sys1", "node1", nil, ni2)

	m2 := NewVPSTrafficManager(dir)
	state, ok := m2.states["sys1"]
	if !ok {
		t.Fatal("expected state to be loaded from disk")
	}
	if state.CycleTx != 50_000 || state.CycleRx != 100_000 {
		t.Errorf("reloaded state mismatch: CycleTx=%d CycleRx=%d", state.CycleTx, state.CycleRx)
	}
	if _, err := os.Stat(filepath.Join(dir, trafficStateFile)); os.IsNotExist(err) {
		t.Error("state file should exist on disk")
	}
}

func TestProjection(t *testing.T) {
	start := time.Date(2025, 7, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(2025, 8, 1, 0, 0, 0, 0, time.Local)
	mid := start.Add(time.Duration(end.Sub(start).Seconds()/2) * time.Second)

	p := computeProjected(500_000, start, end, mid)
	if p < 990_000 || p > 1_010_000 {
		t.Errorf("expected projected ~1000000, got %d", p)
	}

	early := start.Add(30 * time.Second)
	pe := computeProjected(500_000, start, end, early)
	if pe != 0 {
		t.Errorf("expected 0 projection when too early, got %d", pe)
	}
}

func TestMultipleInterfaces(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"sum_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	ni1 := map[string][4]uint64{
		"eth0": {0, 0, 100_000, 200_000},
		"eth1": {0, 0, 50_000, 80_000},
	}
	m.DeriveTraffic("sys1", "node1", nil, ni1)

	ni2 := map[string][4]uint64{
		"eth0": {0, 0, 120_000, 250_000},
		"eth1": {0, 0, 70_000, 100_000},
	}
	info := m.DeriveTraffic("sys1", "node1", nil, ni2)
	if info == nil {
		t.Fatal("expected non-nil")
	}
	if info.CycleTxBytes != 40_000 {
		t.Errorf("expected CycleTx=40000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 70_000 {
		t.Errorf("expected CycleRx=70000, got %d", info.CycleRxBytes)
	}
}

// TestSingleNicReset verifies per-NIC counter-reset handling.
func TestSingleNicReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1}}`)
	m := NewVPSTrafficManager(dir)

	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 1_000_000, 2_000_000}})
	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 1_200_000, 2_400_000}})

	// eth0 reboots, counter goes to 10_000
	info := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{"eth0": {0, 0, 10_000, 20_000}})
	// delta1: tx=200k, rx=400k.  delta2 (reset): tx=10k, rx=20k
	if info.CycleTxBytes != 210_000 {
		t.Errorf("single NIC reset CycleTx: want 210000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 420_000 {
		t.Errorf("single NIC reset CycleRx: want 420000, got %d", info.CycleRxBytes)
	}
}

// TestMultiNicOneReset: two NICs, one resets while the other grows normally.
func TestMultiNicOneReset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1}}`)
	m := NewVPSTrafficManager(dir)

	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 1_000_000, 2_000_000},
		"eth1": {0, 0, 500_000, 600_000},
	})

	info := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 1_100_000, 2_200_000}, // normal growth +100k tx, +200k rx
		"eth1": {0, 0, 5_000, 8_000},          // reset: delta = 5k tx, 8k rx
	})
	// eth0: tx delta=100k, rx delta=200k
	// eth1: tx delta=5k (reset), rx delta=8k (reset)
	if info.CycleTxBytes != 105_000 {
		t.Errorf("multi NIC one reset CycleTx: want 105000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 208_000 {
		t.Errorf("multi NIC one reset CycleRx: want 208000, got %d", info.CycleRxBytes)
	}
}

// TestNicDisappears: a NIC that was present before vanishes. Must not cause
// a negative or positive spike.
func TestNicDisappears(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1}}`)
	m := NewVPSTrafficManager(dir)

	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 100_000, 200_000},
		"eth1": {0, 0, 50_000, 80_000},
	})

	// eth1 disappears
	info := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 120_000, 250_000},
	})
	// only eth0 delta should count: tx +20k, rx +50k
	if info.CycleTxBytes != 20_000 {
		t.Errorf("NIC disappear CycleTx: want 20000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 50_000 {
		t.Errorf("NIC disappear CycleRx: want 50000, got %d", info.CycleRxBytes)
	}
}

// TestNewNicAppears: a NIC not previously seen shows up. Its existing
// counter must not be added as a delta spike.
func TestNewNicAppears(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1}}`)
	m := NewVPSTrafficManager(dir)

	m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 100_000, 200_000},
	})

	// eth1 appears with existing large counters
	info := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 120_000, 250_000},   // normal: tx +20k, rx +50k
		"eth1": {0, 0, 5_000_000, 8_000_000}, // new NIC → baseline only
	})
	// only eth0 delta should count
	if info.CycleTxBytes != 20_000 {
		t.Errorf("new NIC appear CycleTx: want 20000, got %d", info.CycleTxBytes)
	}
	if info.CycleRxBytes != 50_000 {
		t.Errorf("new NIC appear CycleRx: want 50000, got %d", info.CycleRxBytes)
	}

	// third sample: now both NICs contribute deltas
	info2 := m.DeriveTraffic("sys1", "n", nil, map[string][4]uint64{
		"eth0": {0, 0, 130_000, 260_000},
		"eth1": {0, 0, 5_010_000, 8_020_000},
	})
	// eth0: tx +10k, rx +10k.  eth1: tx +10k, rx +20k
	if info2.CycleTxBytes != 40_000 {
		t.Errorf("new NIC 2nd sample CycleTx: want 40000, got %d", info2.CycleTxBytes)
	}
	if info2.CycleRxBytes != 80_000 {
		t.Errorf("new NIC 2nd sample CycleRx: want 80000, got %d", info2.CycleRxBytes)
	}
}

func TestInvalidDefaultBillingModeFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"bogus_mode"}}`)
	m := NewVPSTrafficManager(dir)

	if m.config.Default.BillingMode != BillingModeMaxRxTx {
		t.Errorf("invalid default billingMode should fall back: got %s", m.config.Default.BillingMode)
	}
	ni := map[string][4]uint64{"eth0": {0, 0, 100, 200}}
	info := m.DeriveTraffic("sys1", "node1", nil, ni)
	if info.BillingMode != BillingModeMaxRxTx {
		t.Errorf("DeriveTraffic should return canonical mode: got %s", info.BillingMode)
	}
}

func TestInvalidSystemBillingModeFallback(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"default":{"resetDay":1,"billingMode":"sum_rx_tx"},"systems":{"us":{"billingMode":"nonsense"}}}`
	t.Setenv("VPS_TRAFFIC_CONFIG", cfg)
	m := NewVPSTrafficManager(dir)

	ncUS := m.getNodeConfig("sys2", "us", nil)
	if ncUS.BillingMode != BillingModeMaxRxTx {
		t.Errorf("invalid system billingMode should fall back to max_rx_tx: got %s", ncUS.BillingMode)
	}
	ncOther := m.getNodeConfig("sys1", "hzcsj", nil)
	if ncOther.BillingMode != BillingModeSumRxTx {
		t.Errorf("valid default billingMode should be preserved: got %s", ncOther.BillingMode)
	}
}

func TestValidBillingModeOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"default":{"resetDay":1,"billingMode":"max_rx_tx"},"systems":{"us":{"billingMode":"tx_only"}}}`
	t.Setenv("VPS_TRAFFIC_CONFIG", cfg)
	m := NewVPSTrafficManager(dir)

	ncUS := m.getNodeConfig("sys2", "us", nil)
	if ncUS.BillingMode != BillingModeTxOnly {
		t.Errorf("valid override should take effect: got %s", ncUS.BillingMode)
	}
	ni := map[string][4]uint64{"eth0": {0, 0, 500, 300}}
	m.DeriveTraffic("sys2", "us", nil, ni)
	ni2 := map[string][4]uint64{"eth0": {0, 0, 700, 400}}
	info := m.DeriveTraffic("sys2", "us", nil, ni2)
	if info.BillingMode != BillingModeTxOnly {
		t.Errorf("DeriveTraffic should return tx_only: got %s", info.BillingMode)
	}
	// tx_only: billable = cycleTx only = 200
	if info.BillableBytes != 200 {
		t.Errorf("tx_only billable should be 200, got %d", info.BillableBytes)
	}
}

func TestDBConfigOverridesEnvSystem(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"default":{"resetDay":1,"quotaBytes":1000,"billingMode":"max_rx_tx"},"systems":{"us":{"resetDay":16}}}`
	t.Setenv("VPS_TRAFFIC_CONFIG", cfg)
	m := NewVPSTrafficManager(dir)

	dbTraffic := &VPSTrafficNodeConfig{ResetDay: 20, QuotaBytes: 5000}
	nc := m.getNodeConfig("sys2", "us", dbTraffic)
	if nc.ResetDay != 20 {
		t.Errorf("DB config should override env system resetDay: got %d", nc.ResetDay)
	}
	if nc.QuotaBytes != 5000 {
		t.Errorf("DB config should override env default quota: got %d", nc.QuotaBytes)
	}
	if nc.BillingMode != BillingModeMaxRxTx {
		t.Errorf("billingMode should inherit from env default: got %s", nc.BillingMode)
	}
}

func TestDBConfigOverridesEnvDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	dbTraffic := &VPSTrafficNodeConfig{BillingMode: "tx_only"}
	nc := m.getNodeConfig("sys1", "hzcsj", dbTraffic)
	if nc.BillingMode != BillingModeTxOnly {
		t.Errorf("DB billingMode should override env default: got %s", nc.BillingMode)
	}
	if nc.ResetDay != 1 {
		t.Errorf("resetDay should inherit from env default: got %d", nc.ResetDay)
	}
}

func TestDBConfigInvalidFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx"}}`)
	m := NewVPSTrafficManager(dir)

	dbTraffic := &VPSTrafficNodeConfig{ResetDay: 50, BillingMode: "nonsense"}
	nc := m.getNodeConfig("sys1", "node1", dbTraffic)
	if nc.ResetDay != 28 {
		t.Errorf("DB resetDay >28 should be clamped: got %d", nc.ResetDay)
	}
	if nc.BillingMode != BillingModeMaxRxTx {
		t.Errorf("invalid DB billingMode should fall back to max_rx_tx: got %s", nc.BillingMode)
	}
}

func TestDBConfigEmptyDoesNotOverride(t *testing.T) {
	dir := t.TempDir()
	cfg := `{"default":{"resetDay":5,"quotaBytes":9999,"billingMode":"sum_rx_tx"}}`
	t.Setenv("VPS_TRAFFIC_CONFIG", cfg)
	m := NewVPSTrafficManager(dir)

	dbTraffic := &VPSTrafficNodeConfig{}
	nc := m.getNodeConfig("sys1", "node1", dbTraffic)
	if nc.ResetDay != 5 {
		t.Errorf("empty DB resetDay should not override: got %d", nc.ResetDay)
	}
	if nc.QuotaBytes != 9999 {
		t.Errorf("empty DB quota should not override: got %d", nc.QuotaBytes)
	}
	if nc.BillingMode != BillingModeSumRxTx {
		t.Errorf("empty DB billingMode should not override: got %s", nc.BillingMode)
	}
}

func TestDeriveTrafficWithDBConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VPS_TRAFFIC_CONFIG", `{"default":{"resetDay":1,"billingMode":"max_rx_tx","quotaBytes":1000000}}`)
	m := NewVPSTrafficManager(dir)

	dbTraffic := &VPSTrafficNodeConfig{ResetDay: 16, BillingMode: "tx_only"}

	ni := map[string][4]uint64{"eth0": {0, 0, 100_000, 200_000}}
	m.DeriveTraffic("sys1", "us", dbTraffic, ni)

	ni2 := map[string][4]uint64{"eth0": {0, 0, 150_000, 300_000}}
	info := m.DeriveTraffic("sys1", "us", dbTraffic, ni2)
	if info.ResetDay != 16 {
		t.Errorf("expected DB resetDay=16, got %d", info.ResetDay)
	}
	if info.BillingMode != BillingModeTxOnly {
		t.Errorf("expected DB billingMode tx_only, got %s", info.BillingMode)
	}
	if info.BillableBytes != 50_000 {
		t.Errorf("tx_only billable should be 50000 (cycleTx), got %d", info.BillableBytes)
	}
	if info.QuotaBytes != 1_000_000 {
		t.Errorf("quota should inherit from env default, got %d", info.QuotaBytes)
	}
}

func TestConfigMarshalRoundtrip(t *testing.T) {
	cfg := VPSTrafficConfig{
		Default: VPSTrafficNodeConfig{
			ResetDay:    1,
			QuotaBytes:  2199023255552,
			BillingMode: BillingModeMaxRxTx,
		},
		Systems: map[string]VPSTrafficNodeConfig{
			"us": {ResetDay: 16},
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var cfg2 VPSTrafficConfig
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatal(err)
	}
	if cfg2.Default.ResetDay != 1 || cfg2.Systems["us"].ResetDay != 16 {
		t.Error("config roundtrip mismatch")
	}
}
