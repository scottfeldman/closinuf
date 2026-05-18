package main

import (
	"encoding/binary"
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

	bcmClkPassword = 0x5A000000

	offGP0CTL = 0x70
	offGP0DIV = 0x74

	// GPCLK0 source: 1 = oscillator (19.2 MHz). DIVI=2, DIVF=546 → ~9 MHz.
	gpclkSrcOsc  = 1
	gpclkDivInt  = 2
	gpclkDivFrac = 546
	gpclkDivVal  = bcmClkPassword | (uint32(gpclkDivInt) << 12) | gpclkDivFrac
	gpclkDivMask = uint32(0x00ffffff) // readback: no password byte
)

// initGPCLK programs GPCLK0 on GPIO4 (~9 MHz) for LS7366R fCKi (Pi 4 / BCM2711).
// Requires root (/dev/mem). The systemd service runs closinuf as root.
func initGPCLK() error {
	if err := programGPCLK(); err != nil {
		return err
	}
	ctl, div, err := readGPCLK0Regs()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "GPCLK0_CTL=0x%08x GPCLK0_DIV=0x%08x after setup\n", ctl, div)
	return nil
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

	// GPIO4 → ALT0 (GPCLK0).
	gpio[0] = (gpio[0] & ^(uint32(7) << 12)) | (uint32(4) << 12)

	// Stop GPCLK0 — must wait for BUSY to assert then clear before DIV sticks.
	writeClkReg(clkMem, offGP0CTL, bcmClkPassword|0x20)
	time.Sleep(time.Millisecond)

	for i := 0; i < 100; i++ {
		if readClkReg(clkMem, offGP0CTL)&0x80 != 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}

	for i := 0; i < 1000; i++ {
		if readClkReg(clkMem, offGP0CTL)&0x80 == 0 {
			break
		}
		if i == 999 {
			return fmt.Errorf("GPCLK0 busy after kill")
		}
		time.Sleep(time.Millisecond)
	}

	writeClkReg(clkMem, offGP0DIV, gpclkDivVal)
	div := readClkReg(clkMem, offGP0DIV) & gpclkDivMask
	if div != gpclkDivVal&gpclkDivMask {
		return fmt.Errorf("GPCLK0_DIV write failed: got 0x%08x want 0x%08x", div, gpclkDivVal&gpclkDivMask)
	}

	writeClkReg(clkMem, offGP0CTL, bcmClkPassword|gpclkSrcOsc|0x10)

	if readClkReg(clkMem, offGP0CTL)&0x10 == 0 {
		return fmt.Errorf("GPCLK0 enable bit not set after programming")
	}

	fmt.Fprintf(os.Stderr, "GPCLK0 enabled on GPIO4 (~9 MHz, OSC) for LS7366R fCKi\n")
	return nil
}

func writeClkReg(clkMem []byte, off int, val uint32) {
	binary.LittleEndian.PutUint32(clkMem[off:off+4], val)
}

func readClkReg(clkMem []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(clkMem[off : off+4])
}

func readGPCLK0Regs() (ctl, div uint32, err error) {
	file, err := os.OpenFile("/dev/mem", os.O_RDONLY|unix.O_SYNC, 0)
	if err != nil {
		return 0, 0, fmt.Errorf("open /dev/mem: %w", err)
	}
	defer file.Close()

	clkMem, err := unix.Mmap(int(file.Fd()), int64(bcm2711CLKBase), gpclkMapSize, unix.PROT_READ, unix.MAP_SHARED)
	if err != nil {
		return 0, 0, fmt.Errorf("mmap clock: %w", err)
	}
	defer unix.Munmap(clkMem)

	return readClkReg(clkMem, offGP0CTL), readClkReg(clkMem, offGP0DIV), nil
}

func gpclkEnabledInHW() bool {
	ctl, div, err := readGPCLK0Regs()
	if err != nil {
		return false
	}
	return ctl&0x10 != 0 && div&gpclkDivMask == gpclkDivVal&gpclkDivMask
}
