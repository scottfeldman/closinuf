package main

import (
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

type encoderChipDebug struct {
	Chip         int     `json:"chip"`
	Label        string  `json:"label"`
	IC           string  `json:"ic"`
	CachedCount  int     `json:"cached_count"`
	LiveCount    int     `json:"live_count"`
	LiveCountAlt int     `json:"live_count_alt_rx0_3"`
	RXHex        string  `json:"rx_hex"`
	MDR0         string  `json:"mdr0"`
	MDR1         string  `json:"mdr1"`
	STR          string  `json:"str"`
	LiveError    string  `json:"live_error,omitempty"`
	PollReadsOK  int     `json:"poll_reads_ok"`
	PollReadsFail int    `json:"poll_reads_fail"`
	LastPollErr  string  `json:"last_poll_error,omitempty"`
}

type encoderDebugResponse struct {
	Chips        []encoderChipDebug `json:"chips"`
	Hint         string             `json:"hint"`
}

func readEncoderDebug() encoderDebugResponse {
	labels := []string{"X", "X'", "Y", "Z"}
	resp := encoderDebugResponse{
		Hint: "Turn one encoder and call again (or /api/encoder/debug/probe). If live_count stays 0 but mdr0/mdr1 match, check encoder 5V and A/B wiring to the screw terminals.",
	}

	if counterBank == nil {
		resp.Hint = "counter bank not initialized"
		return resp
	}

	counterBank.mu.Lock()
	defer counterBank.mu.Unlock()

	for chip := 0; chip < 4; chip++ {
		d := encoderChipDebug{
			Chip:  chip,
			Label: labels[chip],
			IC:    fmt.Sprintf("U%d", chip+1),
		}

		if enc := encoders[chip]; enc != nil {
			enc.mu.RLock()
			d.CachedCount = enc.counter
			enc.mu.RUnlock()
		}

		d.PollReadsOK = chipDiags[chip].readOK
		d.PollReadsFail = chipDiags[chip].readFail
		d.LastPollErr = chipDiags[chip].lastErr

		count, rx, err := counterBank.ReadCounterRaw(chip)
		if err != nil {
			d.LiveError = err.Error()
		} else {
			d.LiveCount = int(count)
			d.RXHex = hex.EncodeToString(rx)
			_, alt := parseCounterRX(rx)
			d.LiveCountAlt = int(alt)
		}

		if mdr0, err := counterBank.readReg8(chip, ls7366ReadMDR0); err == nil {
			d.MDR0 = fmt.Sprintf("0x%02x", mdr0)
		} else {
			d.MDR0 = err.Error()
		}
		if mdr1, err := counterBank.readReg8(chip, ls7366ReadMDR1); err == nil {
			d.MDR1 = fmt.Sprintf("0x%02x", mdr1)
		} else {
			d.MDR1 = err.Error()
		}
		if str, err := counterBank.ReadStatus(chip); err == nil {
			d.STR = fmt.Sprintf("0x%02x", str)
		} else {
			d.STR = err.Error()
		}

		resp.Chips = append(resp.Chips, d)
	}
	return resp
}

type encoderProbeResponse struct {
	Seconds  float64            `json:"seconds"`
	Start    []int              `json:"start_counts"`
	End      []int              `json:"end_counts"`
	Delta    []int              `json:"delta"`
	Labels   []string           `json:"labels"`
	Hint     string             `json:"hint"`
}

func probeEncoderMotion(seconds float64) encoderProbeResponse {
	labels := []string{"X", "X'", "Y", "Z"}
	out := encoderProbeResponse{
		Seconds: seconds,
		Labels:  labels,
		Hint:    "Rotate encoders during the probe window. Non-zero delta means that axis is counting.",
	}
	if counterBank == nil {
		out.Hint = "counter bank not initialized"
		return out
	}
	if seconds < 0.2 {
		seconds = 0.2
	}
	if seconds > 10 {
		seconds = 10
	}

	counterBank.mu.Lock()
	defer counterBank.mu.Unlock()

	start := make([]int, 4)
	end := make([]int, 4)
	for chip := 0; chip < 4; chip++ {
		c, err := counterBank.ReadCounter(chip)
		if err != nil {
			out.Hint = fmt.Sprintf("U%d read failed: %v", chip+1, err)
			return out
		}
		start[chip] = int(c)
	}

	time.Sleep(time.Duration(seconds * float64(time.Second)))

	for chip := 0; chip < 4; chip++ {
		c, err := counterBank.ReadCounter(chip)
		if err != nil {
			out.Hint = fmt.Sprintf("U%d read failed: %v", chip+1, err)
			return out
		}
		end[chip] = int(c)
		out.Delta = append(out.Delta, end[chip]-start[chip])
	}
	out.Start = start
	out.End = end
	return out
}

func registerEncoderDebugRoutes(app *fiber.App) {
	app.Get("/api/encoder/debug", func(c *fiber.Ctx) error {
		return c.JSON(readEncoderDebug())
	})

	app.Get("/api/encoder/debug/probe", func(c *fiber.Ctx) error {
		sec := c.QueryFloat("seconds", 2)
		return c.JSON(probeEncoderMotion(sec))
	})
}
