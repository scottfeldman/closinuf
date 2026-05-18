package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/warthog618/go-gpiocdev"
)

const (
	pointButtonChip   = "gpiochip0"
	pointButtonOffset = 26 // GPIO26 — NO foot switch (falling edge = press)
	buttonDebounce    = 500 * time.Millisecond
)

var (
	btnEventMu      sync.Mutex
	btnPressHandled bool
)

// initPointButton wires GPIO26 for physical capture.
func initPointButton() error {
	_, err := gpiocdev.RequestLines(pointButtonChip,
		[]int{pointButtonOffset},
		gpiocdev.AsInput,
		gpiocdev.WithEventHandler(onPointButtonEvent),
		gpiocdev.WithBothEdges,
		gpiocdev.WithConsumer("point-button"),
	)
	if err != nil {
		return fmt.Errorf("point button GPIO%d: %w", pointButtonOffset, err)
	}
	return nil
}

func onPointButtonEvent(evt gpiocdev.LineEvent) {
	btnEventMu.Lock()
	defer btnEventMu.Unlock()

	// External pull-up: HIGH idle, LOW when pressed (NO).
	if evt.Type == gpiocdev.LineEventFallingEdge {
		if btnPressHandled {
			return
		}
		if !captureAllowedSince(buttonDebounce) {
			return
		}
		btnPressHandled = true
		addCapturePoint()
		playBeep()
		return
	}

	if evt.Type == gpiocdev.LineEventRisingEdge {
		btnPressHandled = false
	}
}
