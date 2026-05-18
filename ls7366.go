package main

import (
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"

	"github.com/warthog618/go-gpiocdev"
	"golang.org/x/sys/unix"
)

// LS7366R command bytes (HARDWARE.md).
const (
	ls7366WriteMDR0 = 0x88
	ls7366WriteMDR1 = 0x90
	ls7366ReadMDR0  = 0x48
	ls7366ReadMDR1  = 0x50
	ls7366ReadSTR   = 0x70
	ls7366ClrCNTR   = 0x20
	ls7366ReadCNTR  = 0x60

	ls7366MDR0 = 0x03 // x4 quadrature, free-run, index disabled
	ls7366MDR1 = 0x00 // 32-bit counter, counting enabled

	spiDevPath   = "/dev/spidev0.0"
	spiSpeedHz   = 1000000
	spiMode      = 0
	spiBits      = 8
	spiChipName = "gpiochip0"
)

// CS GPIO order: U1 (X), U2 (X'), U3 (Y), U4 (Z).
var ls7366CSGPIOs = []int{8, 7, 5, 6}

const (
	spiIOCMagic         = 'k'
	spiIOCWrite         = 1 << 30
	spiNoCS             = 0x40
	spiIocTransferBytes = int(unsafe.Sizeof(spiIocTransfer{}))
)

func spiIOW(nr, size int) uintptr {
	return uintptr(spiIOCWrite) | uintptr(size)<<16 | uintptr(spiIOCMagic)<<8 | uintptr(nr)
}

type spiIocTransfer struct {
	txBuf          uint64
	rxBuf          uint64
	len            uint32
	speedHz        uint32
	delayUsecs     uint16
	bitsPerWord    uint8
	csChange       uint8
	txNbits        uint8
	rxNbits        uint8
	wordDelayUsecs uint8
	pad            uint8
}

// CounterBank drives four LS7366R chips on SPI0 with manual chip selects.
type CounterBank struct {
	spiFd   int
	csLines *gpiocdev.Lines
	mu      sync.Mutex
}

var counterBank *CounterBank

// chipDiag records the last SPI read attempt per chip (for /api/encoder/debug).
type chipDiag struct {
	readOK      int
	readFail    int
	lastErr     string
	lastRX      [5]byte
}

var chipDiags [4]chipDiag

func spiIOCMessage(n int) uintptr {
	return spiIOW(0, n*spiIocTransferBytes)
}

func openCounterBank() (*CounterBank, error) {
	fd, err := unix.Open(spiDevPath, unix.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w (enable SPI in /boot/firmware/config.txt, reboot)", spiDevPath, err)
	}

	mode32 := uint32(spiMode | spiNoCS)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), spiIOW(5, 4), uintptr(unsafe.Pointer(&mode32))); errno != 0 {
		mode := uint8(spiMode | spiNoCS)
		if _, _, errno2 := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), spiIOW(1, 1), uintptr(unsafe.Pointer(&mode))); errno2 != 0 {
			unix.Close(fd)
			return nil, fmt.Errorf("SPI_IOC_WR_MODE: %v", errno2)
		}
	}

	bpw := uint8(spiBits)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), spiIOW(3, 1), uintptr(unsafe.Pointer(&bpw))); errno != 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("SPI_IOC_WR_BITS_PER_WORD: %v", errno)
	}

	speed := uint32(spiSpeedHz)
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), spiIOW(4, 4), uintptr(unsafe.Pointer(&speed))); errno != 0 {
		// Per-transfer speed_hz still applies; some kernels omit global max-speed ioctl.
		fmt.Fprintf(os.Stderr, "Note: SPI_IOC_WR_MAX_SPEED_HZ: %v (using per-transfer speed)\n", errno)
	}

	csLines, err := gpiocdev.RequestLines(spiChipName, ls7366CSGPIOs,
		gpiocdev.AsOutput(1, 1, 1, 1),
		gpiocdev.WithConsumer("ls7366-cs"),
	)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf(
			"CS GPIO: %w (SPI driver may own GPIO 8/7 — add dtoverlay=spi0-2cs,cs0_pin=12,cs1_pin=13 to config.txt and reboot, or re-run install.sh)",
			err,
		)
	}

	bank := &CounterBank{spiFd: fd, csLines: csLines}
	if err := bank.initAll(); err != nil {
		bank.Close()
		return nil, err
	}
	return bank, nil
}

func (b *CounterBank) Close() {
	if b.csLines != nil {
		b.csLines.Close()
		b.csLines = nil
	}
	if b.spiFd >= 0 {
		unix.Close(b.spiFd)
		b.spiFd = -1
	}
}

func (b *CounterBank) transfer(chip int, tx, rx []byte) error {
	if len(tx) != len(rx) {
		return fmt.Errorf("tx/rx length mismatch")
	}
	if err := b.csLines.SetValues(csMask(chip)); err != nil {
		return err
	}
	defer b.csLines.SetValues([]int{1, 1, 1, 1})

	txPtr := uintptr(0)
	rxPtr := uintptr(0)
	if len(tx) > 0 {
		txPtr = uintptr(unsafe.Pointer(&tx[0]))
		rxPtr = uintptr(unsafe.Pointer(&rx[0]))
	}

	tr := spiIocTransfer{
		txBuf:       uint64(txPtr),
		rxBuf:       uint64(rxPtr),
		len:         uint32(len(tx)),
		speedHz:     spiSpeedHz,
		bitsPerWord: spiBits,
	}

	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(b.spiFd), spiIOCMessage(1), uintptr(unsafe.Pointer(&tr))); errno != 0 {
		return fmt.Errorf("SPI transfer chip %d: %v", chip, errno)
	}
	return nil
}

func csMask(chip int) []int {
	m := []int{1, 1, 1, 1}
	m[chip] = 0
	return m
}

func (b *CounterBank) writeReg(chip int, cmd, data byte) error {
	tx := []byte{cmd, data}
	rx := make([]byte, 2)
	return b.transfer(chip, tx, rx)
}

func (b *CounterBank) command(chip int, cmd byte) error {
	tx := []byte{cmd}
	rx := make([]byte, 1)
	return b.transfer(chip, tx, rx)
}

func (b *CounterBank) verifyChip(chip int) error {
	mdr1, err := b.readReg8(chip, ls7366ReadMDR1)
	if err != nil {
		return fmt.Errorf("U%d READ_MDR1: %w", chip+1, err)
	}
	if mdr1 != ls7366MDR1 {
		return fmt.Errorf("U%d READ_MDR1: got 0x%02x want 0x%02x (no SPI response?)", chip+1, mdr1, ls7366MDR1)
	}

	mdr0, err := b.readReg8(chip, ls7366ReadMDR0)
	if err != nil {
		return fmt.Errorf("U%d READ_MDR0: %w", chip+1, err)
	}
	if mdr0 != ls7366MDR0 {
		return fmt.Errorf("U%d READ_MDR0: got 0x%02x want 0x%02x", chip+1, mdr0, ls7366MDR0)
	}

	count, err := b.ReadCounter(chip)
	if err != nil {
		return fmt.Errorf("U%d READ_CNTR: %w", chip+1, err)
	}
	if count != 0 {
		return fmt.Errorf("U%d READ_CNTR after clear: got %d want 0", chip+1, count)
	}

	fmt.Fprintf(os.Stderr, "U%d SPI OK (MDR0=0x%02x MDR1=0x%02x CNTR=0)\n", chip+1, mdr0, mdr1)
	return nil
}

func (b *CounterBank) initChip(chip int) error {
	if err := b.writeReg(chip, ls7366WriteMDR1, ls7366MDR1); err != nil {
		return err
	}
	if err := b.writeReg(chip, ls7366WriteMDR0, ls7366MDR0); err != nil {
		return err
	}
	if err := b.command(chip, ls7366ClrCNTR); err != nil {
		return err
	}
	return b.verifyChip(chip)
}

func (b *CounterBank) initAll() error {
	for chip := 0; chip < 4; chip++ {
		if err := b.initChip(chip); err != nil {
			return fmt.Errorf("init U%d: %w", chip+1, err)
		}
	}
	return nil
}

func parseCounterRX(rx []byte) (int32, int32) {
	if len(rx) < 5 {
		return 0, 0
	}
	a := int32(uint32(rx[1])<<24 | uint32(rx[2])<<16 | uint32(rx[3])<<8 | uint32(rx[4]))
	b := int32(uint32(rx[0])<<24 | uint32(rx[1])<<16 | uint32(rx[2])<<8 | uint32(rx[3]))
	return a, b
}

func (b *CounterBank) readReg8(chip int, cmd byte) (byte, error) {
	tx := []byte{cmd, 0xff}
	rx := make([]byte, 2)
	if err := b.transfer(chip, tx, rx); err != nil {
		return 0, err
	}
	return rx[1], nil
}

func (b *CounterBank) ReadCounterRaw(chip int) (count int32, rx []byte, err error) {
	tx := make([]byte, 5)
	rx = make([]byte, 5)
	tx[0] = ls7366ReadCNTR
	for i := 1; i < 5; i++ {
		tx[i] = 0xff
	}
	if err := b.transfer(chip, tx, rx); err != nil {
		return 0, rx, err
	}
	count, _ = parseCounterRX(rx)
	return count, rx, nil
}

func (b *CounterBank) ReadCounter(chip int) (int32, error) {
	count, rx, err := b.ReadCounterRaw(chip)
	if chip >= 0 && chip < len(chipDiags) {
		d := &chipDiags[chip]
		if err != nil {
			d.readFail++
			d.lastErr = err.Error()
		} else {
			d.readOK++
			d.lastErr = ""
			copy(d.lastRX[:], rx)
		}
	}
	return count, err
}

func (b *CounterBank) ReadStatus(chip int) (byte, error) {
	return b.readReg8(chip, ls7366ReadSTR)
}

func (b *CounterBank) ClearAll() error {
	for chip := 0; chip < 4; chip++ {
		if err := b.command(chip, ls7366ClrCNTR); err != nil {
			return fmt.Errorf("clear U%d: %w", chip+1, err)
		}
	}
	return nil
}

func initCounters() error {
	// SPI and chip init first, then GPCLK (DIV must be written after a full kill/BUSY cycle).
	bank, err := openCounterBank()
	if err != nil {
		return err
	}
	if err := initGPCLK(); err != nil {
		bank.Close()
		return fmt.Errorf("GPCLK0 on GPIO4: %w", err)
	}
	if !gpclkEnabledInHW() {
		bank.Close()
		return fmt.Errorf("GPCLK0 not enabled after setup")
	}
	counterBank = bank
	fmt.Fprintf(os.Stderr, "LS7366R counters initialized on SPI0 (32-bit mode)\n")
	return nil
}

func pollCountersForever() {
	const interval = 50 * time.Millisecond
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		if counterBank == nil {
			continue
		}
		counterBank.mu.Lock()
		for chip, enc := range encoders {
			if enc == nil {
				continue
			}
			count, err := counterBank.ReadCounter(chip)
			if err != nil {
				if chipDiags[chip].readFail == 1 || chipDiags[chip].readFail%100 == 0 {
					fmt.Fprintf(os.Stderr, "U%d READ_CNTR: %v\n", chip+1, err)
				}
				continue
			}
			enc.mu.Lock()
			enc.counter = int(count)
			enc.mu.Unlock()
		}
		counterBank.mu.Unlock()
	}
}

func clearHardwareCounters() error {
	if counterBank == nil {
		return fmt.Errorf("counter bank not initialized")
	}
	counterBank.mu.Lock()
	defer counterBank.mu.Unlock()
	return counterBank.ClearAll()
}
