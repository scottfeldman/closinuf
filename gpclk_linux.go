//go:build linux

package main

import (
	"fmt"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// BCM2711 (Raspberry Pi 4) peripheral bases for /dev/mem.
const (
	bcm2711PeriBase = 0xFE000000
	bcm2711GPIOBase = bcm2711PeriBase + 0x200000
	bcm2711CLKBase  = bcm2711PeriBase + 0x101000
	gpclkMapSize    = 4096
	gpclkStampFile  = "/run/closinuf-gpclk.ok"

	bcmClkPassword = 0x5A000000

	cmGP0CTL = 0x70 / 4
	cmGP0DIV = 0x74 / 4

	// GPCLK0 source: 1 = oscillator (19.2 MHz). DIVI=2, DIVF=546 → ~9 MHz.
	gpclkSrcOsc  = 1
	gpclkDivInt  = 2
	gpclkDivFrac = 546
)

// initGPCLK programs GPCLK0 on GPIO4 (~9 MHz) for LS7366R fCKi (Pi 4 / BCM2711).
// Under systemd, ExecStartPre runs closinuf-setup-gpclk.sh as root; the app must not
// touch /dev/mem as an unprivileged user (CAP_SYS_RAWIO can still SIGSEGV on access).
func initGPCLK() error {
	if gpclkReady() {
		return nil
	}
	if unix.Geteuid() != 0 {
		return fmt.Errorf("GPCLK not configured (closinuf-setup-gpclk.sh should run as ExecStartPre)")
	}
	if err := programGPCLK(); err != nil {
		return err
	}
	return os.WriteFile(gpclkStampFile, []byte("ok\n"), 0644)
}

func gpclkReady() bool {
	if _, err := os.Stat(gpclkStampFile); err == nil {
		return true
	}
	if unix.Geteuid() != 0 {
		return false
	}
	return gpclkEnabledInHW()
}

func programGPCLK() error {
	file, err := os.OpenFile("/dev/mem", os.O_RDWR|unix.O_SYNC, 0)
	if err != nil {
		return fmt.Errorf("open /dev/mem: %w", err)
	}
	defer file.Close()

	gpioMem, err := unix.Mmap(int(file.Fd()), int64(bcm2711GPIOBase), gpclkMapSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap GPIO: %w", err)
	}
	defer unix.Munmap(gpioMem)

	clkMem, err := unix.Mmap(int(file.Fd()), int64(bcm2711CLKBase), gpclkMapSize, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	if err != nil {
		return fmt.Errorf("mmap clock: %w", err)
	}
	defer unix.Munmap(clkMem)

	gpio := (*[gpclkMapSize / 4]uint32)(unsafe.Pointer(&gpioMem[0]))
	clk := (*[gpclkMapSize / 4]uint32)(unsafe.Pointer(&clkMem[0]))

	// GPIO4 → ALT0 (GPCLK0).
	gpio[0] = (gpio[0] & ^(uint32(7) << 12)) | (uint32(4) << 12)

	// Stop GPCLK0.
	clk[cmGP0CTL] = bcmClkPassword | 0x20
	deadline := time.Now().Add(100 * time.Millisecond)
	for clk[cmGP0CTL]&0x80 != 0 {
		if time.Now().After(deadline) {
			return fmt.Errorf("GPCLK0 busy after kill")
		}
	}

	clk[cmGP0DIV] = bcmClkPassword | (uint32(gpclkDivInt) << 12) | gpclkDivFrac
	clk[cmGP0CTL] = bcmClkPassword | gpclkSrcOsc | 0x10

	if clk[cmGP0CTL]&0x10 == 0 {
		return fmt.Errorf("GPCLK0 enable bit not set after programming")
	}

	fmt.Fprintf(os.Stderr, "GPCLK0 enabled on GPIO4 (~9 MHz, OSC) for LS7366R fCKi\n")
	return nil
}

func gpclkEnabledInHW() bool {
	file, err := os.OpenFile("/dev/mem", os.O_RDONLY|unix.O_SYNC, 0)
	if err != nil {
		return false
	}
	defer file.Close()

	clkMem, err := unix.Mmap(int(file.Fd()), int64(bcm2711CLKBase), gpclkMapSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return false
	}
	defer unix.Munmap(clkMem)

	clk := (*[gpclkMapSize / 4]uint32)(unsafe.Pointer(&clkMem[0]))
	return clk[cmGP0CTL]&0x10 != 0
}
