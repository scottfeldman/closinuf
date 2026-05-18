#!/usr/bin/env bash
# Verify GPCLK0 on GPIO4 (header pin 7) for LS7366R fCKi.
# Run on the Pi: sudo ./scripts/check-gpclk.sh
set -euo pipefail

if [[ "${EUID}" -ne 0 ]]; then
	echo "Run as root: sudo $0" >&2
	exit 1
fi

read_reg() {
	python3 - "$1" <<'PY'
import mmap, os, sys
addr = int(sys.argv[1], 0)
page = addr & ~0xFFF
off = addr - page
fd = os.open("/dev/mem", os.O_RDWR | os.O_SYNC)
m = mmap.mmap(fd, 4096, mmap.MAP_SHARED, mmap.PROT_READ, offset=page)
val = int.from_bytes(m[off : off + 4], "little")
print(f"0x{val:08x}")
os.close(fd)
PY
}

echo "=== GPIO4 pin function (expect GPCLK0 / ALT0) ==="
if command -v pinctrl >/dev/null 2>&1; then
	pinctrl get 4
	if pinctrl get 4 | grep -qE 'GPCLK|ALT|alt4'; then
		echo "OK: GPIO4 mux looks like GPCLK"
	else
		echo "WARN: GPIO4 is not GPCLK — run: sudo systemctl restart closinuf"
	fi
fi

echo
echo "=== GPCLK0 clock manager ==="
ctl=$(read_reg 0xfe101070)
motion_div=$(read_reg 0xfe101074)
fsel=$(read_reg 0xfe200000)

echo "GPCLK0_CTL  = ${ctl}"
echo "GPCLK0_DIV  = ${motion_div}"
echo "GPIO FSEL0  = ${fsel}"

ctl_val=$((ctl))
if (( (ctl_val & 0x10) != 0 )); then
	echo "OK: GPCLK0 enable bit (CTL bit 4) is set"
else
	echo "FAIL: GPCLK0 not enabled — run: sudo systemctl restart closinuf"
fi

div_val=$((motion_div))
if (( (motion_div & 0x00ffff) == 0x2222 )); then
	echo "OK: GPCLK0_DIV looks programmed (0x00002222)"
else
	echo "FAIL: GPCLK0_DIV not programmed (want 0x00002222) — run: sudo systemctl restart closinuf"
fi

fsel_val=$((fsel))
pin4_fsel=$(( (fsel_val >> 12) & 7 ))
echo "GPIO4 FSEL = ${pin4_fsel} (4 = ALT0 / GPCLK0)"
if (( pin4_fsel == 4 )); then
	echo "OK: GPIO4 FSEL is ALT0"
else
	echo "WARN: GPIO4 FSEL is not ALT0"
fi

echo
echo "=== Scope ==="
echo "Pin 7 should show ~9.6 MHz when GPCLK0 is enabled."
echo
echo "=== If GPCLK failed ==="
echo "  sudo systemctl restart closinuf"
echo "  curl -s http://127.0.0.1:3000/api/encoder | jq   # turn encoders"
