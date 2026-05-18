#!/usr/bin/env bash
# Program GPCLK0 on GPIO4 (~9 MHz) for LS7366R fCKi. BCM2711 / Pi 4.
# Prefer closinuf's built-in initGPCLK; this is a fallback for manual use or ExecStartPre.
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
	echo "Run as root: sudo $0" >&2
	exit 1
fi

exec python3 <<'PY'
import mmap
import os
import time

GPIO_BASE = 0xFE200000
CLK_BASE = 0xFE101000
BLOCK = 4096
PASSWORD = 0x5A000000
CM_GP0CTL = 0x70
CM_GP0DIV = 0x74
STAMP = "/run/closinuf-gpclk.ok"

fd = os.open("/dev/mem", os.O_RDWR | os.O_SYNC)
gpio = mmap.mmap(fd, BLOCK, mmap.MAP_SHARED, mmap.PROT_READ | mmap.PROT_WRITE, offset=GPIO_BASE)
clk = mmap.mmap(fd, BLOCK, mmap.MAP_SHARED, mmap.PROT_READ | mmap.PROT_WRITE, offset=CLK_BASE)
os.close(fd)

def r32(m, off):
    return int.from_bytes(m[off : off + 4], "little")

def w32(m, off, val):
    m[off : off + 4] = val.to_bytes(4, "little")

# GPIO4 ALT0
v = r32(gpio, 0)
w32(gpio, 0, (v & ~(7 << 12)) | (4 << 12))

w32(clk, CM_GP0CTL, PASSWORD | 0x20)
for _ in range(1000):
    if r32(clk, CM_GP0CTL) & 0x80 == 0:
        break
    time.sleep(0.001)

w32(clk, CM_GP0DIV, PASSWORD | (2 << 12) | 546)
w32(clk, CM_GP0CTL, PASSWORD | 1 | 0x10)

if r32(clk, CM_GP0CTL) & 0x10:
    with open(STAMP, "w", encoding="ascii") as f:
        f.write("ok\n")
    print("closinuf: GPCLK0 enabled on GPIO4 (~9 MHz, OSC)")
else:
    raise SystemExit("closinuf: GPCLK0 enable failed")
PY
