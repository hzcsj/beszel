package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/henrygd/beszel/agent/utils"
	"github.com/henrygd/beszel/internal/entities/system"
)

type VPSProbeConfig struct {
	Enabled         *bool             `json:"enabled,omitempty"`
	IntervalSeconds int               `json:"intervalSeconds"`
	TimeoutMs       int               `json:"timeoutMs"`
	WindowSize      int               `json:"windowSize"`
	Targets         map[string]string `json:"targets"`
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
	mu       sync.RWMutex
	config   VPSProbeConfig
	windows  map[string]*probeWindow
	latest   system.VPSProbeStats
	cancel   context.CancelFunc
	done     chan struct{}
	interval time.Duration
	timeout  time.Duration
}

var canonicalProbeKeys = [4]string{"hub", "ct", "cu", "cm"}

var defaultProbeTargets = map[string]string{
	"hub": "hzcsj.ikfly.com:20022",
	"ct":  "ct.tz.cloudcpp.com:80",
	"cu":  "cu.tz.cloudcpp.com:80",
	"cm":  "cm.tz.cloudcpp.com:80",
}

func newVPSProbeCollector() *VPSProbeCollector {
	enabled := true
	targets := make(map[string]string, len(defaultProbeTargets))
	for k, v := range defaultProbeTargets {
		targets[k] = v
	}
	cfg := VPSProbeConfig{
		IntervalSeconds: 5,
		TimeoutMs:       1000,
		WindowSize:      60,
		Targets:         targets,
	}

	if raw, exists := utils.GetEnv("BESZEL_AGENT_VPS_PROBE_CONFIG"); exists {
		var userCfg VPSProbeConfig
		if err := json.Unmarshal([]byte(raw), &userCfg); err != nil {
			slog.Warn("BESZEL_AGENT_VPS_PROBE_CONFIG parse error, using defaults", "err", err)
		} else {
			if userCfg.Enabled != nil && !*userCfg.Enabled {
				enabled = false
			}
			if userCfg.IntervalSeconds > 0 {
				cfg.IntervalSeconds = clampInt(userCfg.IntervalSeconds, 1, 300)
			}
			if userCfg.TimeoutMs > 0 {
				cfg.TimeoutMs = clampInt(userCfg.TimeoutMs, 100, 5000)
			}
			if userCfg.WindowSize > 0 {
				cfg.WindowSize = clampInt(userCfg.WindowSize, 10, 600)
			}
			for _, key := range canonicalProbeKeys {
				if addr, ok := userCfg.Targets[key]; ok && addr != "" {
					cfg.Targets[key] = addr
				}
			}
		}
	}

	if !enabled {
		return nil
	}

	c := &VPSProbeCollector{
		config:   cfg,
		windows:  make(map[string]*probeWindow, len(cfg.Targets)),
		latest:   make(system.VPSProbeStats, len(cfg.Targets)),
		interval: time.Duration(cfg.IntervalSeconds) * time.Second,
		timeout:  time.Duration(cfg.TimeoutMs) * time.Millisecond,
	}
	for key := range cfg.Targets {
		c.windows[key] = &probeWindow{
			samples: make([]probeSample, cfg.WindowSize),
		}
	}
	return c
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
		key     string
		latency float64
		ok      bool
	}
	results := make(chan result, len(c.config.Targets))

	for key, addr := range c.config.Targets {
		wg.Add(1)
		go func(k, a string) {
			defer wg.Done()
			lat, ok := c.probeTarget(ctx, a)
			results <- result{key: k, latency: lat, ok: ok}
		}(key, addr)
	}
	wg.Wait()
	close(results)

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().Unix()
	for r := range results {
		w := c.windows[r.key]
		w.samples[w.pos] = probeSample{latencyMs: r.latency, success: r.ok}
		w.pos = (w.pos + 1) % len(w.samples)
		if w.count < len(w.samples) {
			w.count++
		}

		var failed int
		var lastLat float64
		for i := 0; i < w.count; i++ {
			s := w.samples[i]
			if !s.success {
				failed++
			}
		}
		lossPct := float64(failed) / float64(w.count) * 100

		if r.ok {
			lastLat = r.latency
		}

		c.latest[r.key] = system.VPSProbeTargetStats{
			LatencyMs: lastLat,
			LossPct:   lossPct,
			Success:   r.ok,
			Samples:   uint16(w.count),
			Updated:   now,
			Target:    c.config.Targets[r.key],
		}
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
