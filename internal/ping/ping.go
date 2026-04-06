package ping

import (
	"iperf-tool/internal/model"
)

// Result holds parsed ping summary statistics.
type Result struct {
	PacketsSent int
	PacketsRecv int
	PacketLoss  float64
	MinMs       float64
	AvgMs       float64
	MaxMs       float64
}

// ToModel converts a ping Result to the model representation.
func (r *Result) ToModel() *model.PingResult {
	if r == nil {
		return nil
	}
	return &model.PingResult{
		PacketsSent: r.PacketsSent,
		PacketsRecv: r.PacketsRecv,
		PacketLoss:  r.PacketLoss,
		MinMs:       r.MinMs,
		AvgMs:       r.AvgMs,
		MaxMs:       r.MaxMs,
	}
}
