package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os/exec"
)

// playBeep plays a short tone on the default ALSA output (non-blocking).
func playBeep() {
	go func() {
		cmd := exec.Command("aplay", "-q", "-t", "wav", "-")
		in, err := cmd.StdinPipe()
		if err != nil {
			return
		}
		go func() {
			_ = writeBeepWAV(in, 880, 0.1)
			_ = in.Close()
		}()
		_ = cmd.Run()
	}()
}

func writeBeepWAV(w io.Writer, freqHz float64, durationSec float64) error {
	const (
		sampleRate    = 8000
		bitsPerSample = 16
		numChannels   = 1
	)
	n := int(sampleRate * durationSec)
	if n < 1 {
		return fmt.Errorf("duration too short")
	}
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := n * blockAlign
	riffSize := 36 + uint32(dataSize)

	var hdr [44]byte
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], riffSize)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1)
	binary.LittleEndian.PutUint16(hdr[22:24], numChannels)
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(hdr[34:36], bitsPerSample)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(dataSize))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	const amp = 0.22 * 32767
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(amp * math.Sin(2*math.Pi*freqHz*t))
		if err := binary.Write(w, binary.LittleEndian, sample); err != nil {
			return err
		}
	}
	return nil
}
