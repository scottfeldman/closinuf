package main

import (
	"math"
	"sync"
	"time"
)

type encoder struct {
	counter       int // hardware counter value (signed)
	lastReadTime  time.Time
	lastReadCount int
	rpm           float64
	label         string
	chip          int // 0..3 → U1..U4
	mu            sync.RWMutex
}

const (
	countsPerRev       = 2400.0                  // 600 PPR × 4 (full quadrature)
	wheelDiameter      = 50.0                    // wheel diameter in mm
	wheelCircumference = math.Pi * wheelDiameter // ≈ 157.08mm
)

type encoderData struct {
	X  encoderValues `json:"x"`
	Xp encoderValues `json:"x'"`
	Y  encoderValues `json:"y"`
	Z  encoderValues `json:"z"`
}

type encoderValues struct {
	Count    int     `json:"count"`
	RPM      float64 `json:"rpm"`
	Distance float64 `json:"distance"` // distance in mm from zero
	Label    string  `json:"label"`
}

var encoders [4]*encoder // X=0, X'=1, Y=2, Z=3

// initEncoders sets up the four axes, LS7366R counters, and the poll loop.
func initEncoders() error {
	now := time.Now()
	encoders[0] = &encoder{label: "X", chip: 0, lastReadTime: now}
	encoders[1] = &encoder{label: "X'", chip: 1, lastReadTime: now}
	encoders[2] = &encoder{label: "Y", chip: 2, lastReadTime: now}
	encoders[3] = &encoder{label: "Z", chip: 3, lastReadTime: now}

	if err := initCounters(); err != nil {
		return err
	}
	go pollCountersForever()
	return nil
}

func zeroEncoderCounts() {
	for _, enc := range encoders {
		enc.mu.Lock()
		enc.counter = 0
		enc.lastReadCount = 0
		enc.mu.Unlock()
	}
}

func getEncoderData() encoderData {
	var data encoderData
	for i, enc := range encoders {
		enc.mu.RLock()
		count := enc.counter
		rpm := enc.rpm
		label := enc.label
		enc.mu.RUnlock()

		distance := (float64(count) / countsPerRev) * wheelCircumference

		values := encoderValues{
			Count:    count,
			RPM:      rpm,
			Distance: distance,
			Label:    label,
		}

		switch i {
		case 0:
			data.X = values
		case 1:
			data.Xp = values
		case 2:
			data.Y = values
		case 3:
			data.Z = values
		}
	}
	return data
}
