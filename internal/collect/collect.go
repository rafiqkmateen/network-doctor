// Package collect reads facts from the machine. Everything here only looks;
// nothing changes a setting, driver, device or program.
package collect

import (
	"context"
	"errors"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ErrUnsupported means this system has no collector for that fact yet.
var ErrUnsupported = errors.New("not available on this system yet")

// AdapterInfo describes the adapter carrying the default route.
type AdapterInfo struct {
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Kind          core.Kind `json:"kind"`
	Up            bool      `json:"up"`
	LinkMbps      int       `json:"link_mbps"`
	Duplex        string    `json:"duplex"`
	Errors        int64     `json:"errors"`
	Gateway       string    `json:"gateway"`
	IP            string    `json:"ip"`
	PrefixLength  int       `json:"prefix_length"`
	DriverVersion string    `json:"driver_version"`
	DriverAgeDays int       `json:"driver_age_days"`
}

// WiFiInfo is the current Wi-Fi link and the networks nearby.
type WiFiInfo struct {
	SignalPct     int    `json:"signal_pct"`
	Band          string `json:"band"`
	Channel       int    `json:"channel"`
	OtherChannels []int  `json:"other_channels"`
}

// PingResult is the outcome of a ping run.
type PingResult struct {
	AvgMs   float64 `json:"avg_ms"`
	LossPct float64 `json:"loss_pct"`
}

// Throughput is a short sample of what this computer is sending and getting.
type Throughput struct {
	BusyPct int         `json:"busy_pct"`
	RxMBps  float64     `json:"rx_mbps"`
	TxMBps  float64     `json:"tx_mbps"`
	Procs   []core.Proc `json:"procs"`
}

// Source is everything the checks need from a machine.
type Source interface {
	Adapter(ctx context.Context) (AdapterInfo, error)
	WiFi(ctx context.Context) (WiFiInfo, error)
	Ping(ctx context.Context, host string, count int) (PingResult, error)
	Throughput(ctx context.Context, linkMbps int) (Throughput, error)
	Events(ctx context.Context, days int) ([]core.Event, error)
	Devices(ctx context.Context, ip string, prefix int) ([]core.Device, error)
	Settings(ctx context.Context) (core.Settings, error)
	Speed(ctx context.Context) (down, up float64, err error)
	DNSWorks(ctx context.Context) bool
}
