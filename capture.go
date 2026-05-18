package main

import (
	"fmt"
	"sync"
	"time"
)

type point struct {
	x float64
	y float64
	z float64
}

var (
	points             []point
	pointsMu           sync.RWMutex
	lastPointAddedTime time.Time
)

func addCapturePoint() {
	data := getEncoderData()
	pointsMu.Lock()
	points = append(points, point{
		x: data.X.Distance,
		y: data.Y.Distance,
		z: data.Z.Distance,
	})
	pointsMu.Unlock()
	lastPointAddedTime = time.Now()
}

func clearCapturePoints() {
	pointsMu.Lock()
	points = []point{}
	pointsMu.Unlock()
}

func capturePointCount() int {
	pointsMu.RLock()
	n := len(points)
	pointsMu.RUnlock()
	return n
}

func capturePointsASC() (string, error) {
	pointsMu.Lock()
	defer pointsMu.Unlock()
	if len(points) == 0 {
		return "", fmt.Errorf("no points to save")
	}
	var asc string
	for _, p := range points {
		asc += fmt.Sprintf("%.6f %.6f %.6f\n", p.x, p.y, p.z)
	}
	return asc, nil
}

// captureAllowedSince reports whether at least d has passed since the last capture.
func captureAllowedSince(d time.Duration) bool {
	return time.Since(lastPointAddedTime) >= d
}
