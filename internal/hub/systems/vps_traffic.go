package systems

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/henrygd/beszel/internal/entities/system"
)

const (
	BillingModeMaxRxTx = "max_rx_tx"
	BillingModeSumRxTx = "sum_rx_tx"
	BillingModeTxOnly  = "tx_only"
	BillingModeRxOnly  = "rx_only"

	trafficStateFile = "vps_traffic_state.json"
	minCycleElapsed  = 3600 // seconds before projection is meaningful
)

type VPSTrafficConfig struct {
	Default VPSTrafficNodeConfig            `json:"default"`
	Systems map[string]VPSTrafficNodeConfig `json:"systems"`
}

type VPSTrafficNodeConfig struct {
	ResetDay    int    `json:"resetDay,omitempty"`
	QuotaBytes  uint64 `json:"quotaBytes,omitempty"`
	BillingMode string `json:"billingMode,omitempty"`
}

type VPSTrafficState struct {
	SystemID   string            `json:"systemId"`
	SystemName string            `json:"systemName"`
	LastNicRx  map[string]uint64 `json:"lastNicRx,omitempty"`
	LastNicTx  map[string]uint64 `json:"lastNicTx,omitempty"`
	CycleRx    uint64            `json:"cycleRx"`
	CycleTx    uint64            `json:"cycleTx"`
	TotalRx    uint64            `json:"totalRx"`
	TotalTx    uint64            `json:"totalTx"`
	CycleKey   string            `json:"cycleKey"`
	UpdatedAt  int64             `json:"updatedAt"`
}

type VPSTrafficManager struct {
	mu      sync.Mutex
	config  VPSTrafficConfig
	states  map[string]*VPSTrafficState
	dataDir string
}

func defaultConfig() VPSTrafficConfig {
	return VPSTrafficConfig{
		Default: VPSTrafficNodeConfig{
			ResetDay:    1,
			BillingMode: BillingModeMaxRxTx,
		},
	}
}

func NewVPSTrafficManager(dataDir string) *VPSTrafficManager {
	m := &VPSTrafficManager{
		states:  make(map[string]*VPSTrafficState),
		dataDir: dataDir,
		config:  defaultConfig(),
	}
	m.loadConfig()
	m.loadState()
	return m
}

func (m *VPSTrafficManager) loadConfig() {
	raw := os.Getenv("VPS_TRAFFIC_CONFIG")
	if raw == "" {
		return
	}
	var cfg VPSTrafficConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		slog.Warn("VPS_TRAFFIC_CONFIG parse error, using defaults", "err", err)
		return
	}
	if cfg.Default.ResetDay == 0 {
		cfg.Default.ResetDay = 1
	}
	cfg.Default.BillingMode = validBillingMode(cfg.Default.BillingMode)
	cfg.Default.ResetDay = clampResetDay(cfg.Default.ResetDay)
	for k, v := range cfg.Systems {
		if v.ResetDay != 0 {
			v.ResetDay = clampResetDay(v.ResetDay)
		}
		if v.BillingMode != "" {
			v.BillingMode = validBillingMode(v.BillingMode)
		}
		cfg.Systems[k] = v
	}
	m.config = cfg
}

func (m *VPSTrafficManager) loadState() {
	if m.dataDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, trafficStateFile))
	if err != nil {
		return
	}
	var states []*VPSTrafficState
	if err := json.Unmarshal(data, &states); err != nil {
		slog.Warn("vps_traffic_state.json parse error", "err", err)
		return
	}
	for _, s := range states {
		m.states[s.SystemID] = s
	}
}

func (m *VPSTrafficManager) saveState() {
	if m.dataDir == "" {
		return
	}
	states := make([]*VPSTrafficState, 0, len(m.states))
	for _, s := range m.states {
		states = append(states, s)
	}
	data, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal traffic state", "err", err)
		return
	}
	tmpPath := filepath.Join(m.dataDir, trafficStateFile+".tmp")
	finalPath := filepath.Join(m.dataDir, trafficStateFile)

	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		slog.Warn("failed to create traffic state tmp file", "err", err)
		return
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		slog.Warn("failed to write traffic state tmp file", "err", err)
		return
	}
	if err := f.Sync(); err != nil {
		slog.Warn("failed to fsync traffic state tmp file", "err", err)
	}
	f.Close()

	if err := os.Rename(tmpPath, finalPath); err != nil {
		slog.Warn("failed to rename traffic state file", "err", err)
		return
	}
	if dir, err := os.Open(m.dataDir); err == nil {
		_ = dir.Sync()
		dir.Close()
	}
}

func (m *VPSTrafficManager) getNodeConfig(systemID, systemName string, dbTraffic *VPSTrafficNodeConfig) VPSTrafficNodeConfig {
	result := m.config.Default
	if nc, ok := m.config.Systems[systemName]; ok {
		mergeNodeConfig(&result, &nc)
	}
	if nc, ok := m.config.Systems[systemID]; ok {
		mergeNodeConfig(&result, &nc)
	}
	if dbTraffic != nil {
		validated := *dbTraffic
		if validated.ResetDay != 0 {
			validated.ResetDay = clampResetDay(validated.ResetDay)
		}
		if validated.BillingMode != "" {
			validated.BillingMode = validBillingMode(validated.BillingMode)
		}
		mergeNodeConfig(&result, &validated)
	}
	return result
}

func mergeNodeConfig(base, override *VPSTrafficNodeConfig) {
	if override.ResetDay != 0 {
		base.ResetDay = override.ResetDay
	}
	if override.QuotaBytes != 0 {
		base.QuotaBytes = override.QuotaBytes
	}
	if override.BillingMode != "" {
		base.BillingMode = override.BillingMode
	}
}

// DeriveTraffic computes VPSTrafficInfo from stats.ni and persists state.
// dbTraffic is the per-system DB-level config (highest priority); nil if not set.
func (m *VPSTrafficManager) DeriveTraffic(systemID, systemName string, dbTraffic *VPSTrafficNodeConfig, ni map[string][4]uint64) *system.VPSTrafficInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	nc := m.getNodeConfig(systemID, systemName, dbTraffic)
	now := time.Now()
	cycleKey := computeCycleKey(now, nc.ResetDay)
	cycleStart := computeCycleStart(now, nc.ResetDay)

	state, exists := m.states[systemID]
	if !exists {
		state = &VPSTrafficState{
			SystemID:   systemID,
			SystemName: systemName,
			LastNicRx:  make(map[string]uint64),
			LastNicTx:  make(map[string]uint64),
			CycleKey:   cycleKey,
			UpdatedAt:  now.Unix(),
		}
		for name, v := range ni {
			state.LastNicTx[name] = v[2]
			state.LastNicRx[name] = v[3]
		}
		m.states[systemID] = state
		m.saveState()
		return m.buildInfo(state, nc, cycleStart, now)
	}

	state.SystemName = systemName

	// Per-NIC delta computation:
	//  - known NIC, counter grew      → normal delta
	//  - known NIC, counter decreased  → counter reset; use current value if > 0
	//  - new NIC (not in LastNicRx)    → initialize baseline only, delta = 0
	//  - disappeared NIC               → ignored, no negative compensation
	var deltaRx, deltaTx uint64
	if state.LastNicRx != nil {
		for name, v := range ni {
			currTx, currRx := v[2], v[3]
			if prevRx, ok := state.LastNicRx[name]; ok {
				if currRx >= prevRx {
					deltaRx += currRx - prevRx
				} else if currRx > 0 {
					deltaRx += currRx
				}
			}
			if prevTx, ok := state.LastNicTx[name]; ok {
				if currTx >= prevTx {
					deltaTx += currTx - prevTx
				} else if currTx > 0 {
					deltaTx += currTx
				}
			}
		}
	}

	// Update per-NIC baselines to current snapshot
	state.LastNicRx = make(map[string]uint64, len(ni))
	state.LastNicTx = make(map[string]uint64, len(ni))
	for name, v := range ni {
		state.LastNicTx[name] = v[2]
		state.LastNicRx[name] = v[3]
	}

	// Historical totals always accumulate, even across billing cycles
	state.TotalRx += deltaRx
	state.TotalTx += deltaTx

	if state.CycleKey != cycleKey {
		// Billing cycle boundary: cross-cycle delta goes to new cycle
		state.CycleRx = deltaRx
		state.CycleTx = deltaTx
		state.CycleKey = cycleKey
	} else {
		state.CycleRx += deltaRx
		state.CycleTx += deltaTx
	}

	state.UpdatedAt = now.Unix()
	m.saveState()
	return m.buildInfo(state, nc, cycleStart, now)
}

func (m *VPSTrafficManager) buildInfo(state *VPSTrafficState, nc VPSTrafficNodeConfig, cycleStart time.Time, now time.Time) *system.VPSTrafficInfo {
	nextCycle := nextCycleStart(now, nc.ResetDay)
	daysLeft := int(nextCycle.Sub(now).Hours() / 24)

	billable := computeBillable(state.CycleRx, state.CycleTx, nc.BillingMode)
	projected := computeProjected(billable, cycleStart, nextCycle, now)

	return &system.VPSTrafficInfo{
		ResetDay:       uint8(nc.ResetDay),
		CycleStart:     cycleStart.Format("2006-01-02"),
		DaysLeft:       daysLeft,
		CycleRxBytes:   state.CycleRx,
		CycleTxBytes:   state.CycleTx,
		TotalRxBytes:   state.TotalRx,
		TotalTxBytes:   state.TotalTx,
		QuotaBytes:     nc.QuotaBytes,
		BillableBytes:  billable,
		ProjectedBytes: projected,
		BillingMode:    nc.BillingMode,
		UpdatedUnix:    state.UpdatedAt,
	}
}

func computeBillable(rx, tx uint64, mode string) uint64 {
	switch mode {
	case BillingModeSumRxTx:
		return rx + tx
	case BillingModeTxOnly:
		return tx
	case BillingModeRxOnly:
		return rx
	default:
		if rx > tx {
			return rx
		}
		return tx
	}
}

func computeProjected(billable uint64, cycleStart, nextCycle time.Time, now time.Time) uint64 {
	elapsed := now.Sub(cycleStart).Seconds()
	if elapsed < minCycleElapsed {
		return 0
	}
	total := nextCycle.Sub(cycleStart).Seconds()
	if total <= 0 {
		return 0
	}
	return uint64(float64(billable) / elapsed * total)
}

func computeCycleKey(now time.Time, resetDay int) string {
	cs := computeCycleStart(now, resetDay)
	return cs.Format("2006-01-02")
}

func computeCycleStart(now time.Time, resetDay int) time.Time {
	y, mo, d := now.Date()
	loc := now.Location()
	if d >= resetDay {
		return time.Date(y, mo, resetDay, 0, 0, 0, 0, loc)
	}
	return time.Date(y, mo-1, resetDay, 0, 0, 0, 0, loc)
}

func nextCycleStart(now time.Time, resetDay int) time.Time {
	y, mo, d := now.Date()
	loc := now.Location()
	if d >= resetDay {
		return time.Date(y, mo+1, resetDay, 0, 0, 0, 0, loc)
	}
	return time.Date(y, mo, resetDay, 0, 0, 0, 0, loc)
}

func validBillingMode(mode string) string {
	switch mode {
	case BillingModeMaxRxTx, BillingModeSumRxTx, BillingModeTxOnly, BillingModeRxOnly:
		return mode
	default:
		if mode != "" {
			slog.Warn("unknown billingMode, falling back to max_rx_tx", "billingMode", mode)
		}
		return BillingModeMaxRxTx
	}
}

func clampResetDay(d int) int {
	if d < 1 {
		return 1
	}
	if d > 28 {
		return 28
	}
	return d
}
