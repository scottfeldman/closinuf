# closinuf — Hardware Design

This document captures the **encoder counter board** that sits between the four
quadrature encoders and the Raspberry Pi. The board is a **Raspberry Pi 4 HAT**:
it stacks directly onto the Pi's 40‑pin GPIO header via `J1` and takes all of
its power from that header (3.3 V and 5 V). Counting is done in hardware by two
**LS7466** dual‑axis 24‑bit quadrature counter ICs, read by the Pi over **SPI0**.

The PCB sources for this design live in [`pcb/`](pcb/). This file is the
canonical reference for the schematic; if the two ever disagree, treat this
document as the spec.

---

## 1. System overview

```
                                                  +3.3V
                                                    │
                                                    ▼
   ┌──────────────┐                          ┌──────────────┐
   │              │   SPI0 (shared bus)      │  LS7466  U1  │── Encoder X   (axis-x)
   │   Raspberry  │  MOSI/MISO/SCLK ──┬─────►│              │── Encoder X′  (axis-y)
   │     Pi       │                   │      └──────────────┘
   │              │     CE0 ──────────┘ U1   ┌──────────────┐
   │              │     CE1 ─────────────────┤  LS7466  U2  │── Encoder Y   (axis-x)
   │              │                          │              │── Encoder Z   (axis-y)
   │  GPIO 26 ◄───┼── foot switch            └──────────────┘
   └──────────────┘
```

Two ICs total, each handling two encoders on independent 24‑bit counters. The
Pi periodically issues `RD_CNTR` for the desired axis and reads 3 bytes back.

### Counter range

The LS7466 `CNTR` is 24 bits. With 600 PPR encoders at x4 quadrature
(2400 counts/rev) on a 50 mm wheel (≈157.08 mm/rev), interpreting `CNTR` as
signed two's complement:

- Half range: \(2^{23}\) = 8 388 608 counts ≈ **3 495 revolutions** in either
  direction from zero
- Linear travel: ≈ **549 m (≈1801 ft) one‑way** before wrap

For this machine that's vastly more headroom than needed.

---

## 2. Bill of materials

| Ref       | Qty | Part                                  | Suggested MPN                  | KiCad footprint                                      | Notes |
|-----------|-----|---------------------------------------|--------------------------------|------------------------------------------------------|-------|
| U1, U2    | 2   | **LS7466‑S** (SOIC‑16)                | `LS7466-S`                     | `Package_SO:SOIC-16_3.9x9.9mm_P1.27mm`               | Dual‑axis 24‑bit quadrature counter. SOIC‑16 narrow body, 1.27 mm pitch. |
| C1, C2    | 2   | 0.1 µF, X7R, 25 V, 0603, ±10 %        | Murata `GRM188R71E104KA01D`    | `Capacitor_SMD:C_0603_1608Metric`                    | Decoupling, **at pin 16** of each chip. |
| C3        | 1   | 10 µF, X5R, 10 V, 0805, ±10 %         | Murata `GRM21BR61A106KE19L`    | `Capacitor_SMD:C_0805_2012Metric`                    | Bulk on the 3.3 V rail. Use ≥10 V part to avoid DC‑bias derating loss at 3.3 V. |
| R1        | 1   | 4.7 kΩ, 1 %, 1/10 W, 0603             | Yageo `RC0603FR-074K7L`        | `Resistor_SMD:R_0603_1608Metric`                     | Foot‑switch pull‑up to 3.3 V. |
| R2–R9     | 8   | 4.7 kΩ, 1 %, 1/10 W, 0603             | Yageo `RC0603FR-074K7L`        | `Resistor_SMD:R_0603_1608Metric`                     | Pull‑ups on every encoder A and B (2 per encoder × 4 encoders). |
| J1        | 1   | 2×20 0.1″ socket                      | Samtec `SSW-120-01-T-D` (or any 2×20 2.54 mm socket) | `Connector_PinSocket_2.54mm:PinSocket_2x20_P2.54mm_Vertical` | Pi GPIO header connector. |
| J2        | 1   | 4‑pos PCB terminal block, 5 mm pitch, horizontal entry | Phoenix Contact `PT 1,5/ 4-5,0-H` (1935284) | `TerminalBlock_Phoenix:TerminalBlock_Phoenix_PT-1,5-4-5.0-H_1x04_P5.00mm_Horizontal` | Foot switch — only 2 of the 4 positions are wired (GPIO 26 + GND). |
| J3–J6     | 4   | 4‑pos PCB terminal block, 5 mm pitch, horizontal entry | Phoenix Contact `PT 1,5/ 4-5,0-H` (1935284) | `TerminalBlock_Phoenix:TerminalBlock_Phoenix_PT-1,5-4-5.0-H_1x04_P5.00mm_Horizontal` | Encoder cables (X, X′, Y, Z). Each carries A, B, **+5 V**, GND. |
| —         | —   | Optional: 4× (100 Ω + 1 nF)           | —                              | —                                                    | RC snubber on each A/B if encoder cables are long (>1 m). |
| —         | —   | Optional: 1× 4.7 kΩ + 1 GPIO          | —                              | —                                                    | Pull‑up for wire‑OR’d `FLAG/` interrupt if you ever wire it. |

All passives are surface‑mount: caps and resistors in 0603 (with `C3` in 0805 for
better DC‑bias performance). MPNs above are stocked at Digi‑Key / Mouser / LCSC
and are interchangeable with the equivalent parts from Kemet, Panasonic, Vishay,
TDK, Samsung, or Yageo at the same package and dielectric. The KiCad footprints
in the table are the standard parts shipped with KiCad's stock libraries; the
schematic in `pcb/encoder.kicad_sch` already has them assigned for every
component (U1, U2, C1–C3, R1–R9, J1–J6).

---

## 3. Raspberry Pi pin assignment

40‑pin header. Only the pins relevant to this board are listed.

| Function                     | RPi GPIO (BCM) | Header pin |
|------------------------------|----------------|-----------:|
| SPI0 MOSI                    | GPIO 10        | 19         |
| SPI0 MISO                    | GPIO 9         | 21         |
| SPI0 SCLK                    | GPIO 11        | 23         |
| SPI0 CE0  → U1 SS/  (X, X′)  | GPIO 8         | 24         |
| SPI0 CE1  → U2 SS/  (Y,  Z)  | GPIO 7         | 26         |
| **Foot switch** (`J2`)       | GPIO 26        | 37         |
| 3.3 V supply (LS7466 VDD)    | —              | 1, 17      |
| 5 V supply (encoder modules) | —              | 2, 4       |
| GND                          | —              | 6, 9, 14, 20, 25, 30, 34, 39 |

Notes:

- SPI1 is **not used**. With LS7466 the four encoders fit on two chips on a
  single bus.
- LS7466 chips, the pull‑up resistors, and the foot‑switch network all run
  from the Pi's **3.3 V** rail (header pins 1 / 17). The four encoder modules
  run from the Pi's **5 V** rail (header pins 2 / 4); their open‑collector
  A/B outputs are pulled up to 3.3 V at the LS7466 end so signal levels stay
  inside the chip's input range. Two LS7466s plus the pull‑ups draw under
  5 mA from 3.3 V; encoder current is dominated by the encoder modules
  themselves (typically tens of mA each — check your encoder spec).
- If you ever want bus isolation between the X/X′ pair and the Y/Z pair, you
  can move U2 to SPI1 CE0 (GPIO 18) and add `dtoverlay=spi1-2cs`. The single‑bus
  layout below is recommended.

### `/boot/firmware/config.txt`

```ini
dtparam=spi=on
```

---

## 4. LS7466 wiring (per chip)

Pinout (SOIC‑16 / TSSOP‑16, top view):

```
                  LS7466
                ┌────────────┐
        SS/   1 ┤•           ├ 16   VDD
        SCK   2 ┤            ├ 15   FLAGy/
        MISO  3 ┤            ├ 14   CEy
        MOSI  4 ┤            ├ 13   Zy
        Ax    5 ┤            ├ 12   By
        Bx    6 ┤            ├ 11   Ay
        Zx    7 ┤            ├ 10   FLAGx/
        GND   8 ┤            ├  9   CEx
                └────────────┘
```

Each chip carries two **independent** counters: the chip's "x‑axis" (pins
5/6/7/9/10) and its "y‑axis" (pins 11/12/13/14/15). These names belong to the
chip — they are not the machine X/Y. Wiring per chip:

| Pin | Net                                                              |
|-----|------------------------------------------------------------------|
| 1   | `SSn` from this chip's bus CE pin (SPI0 CE0 for U1, CE1 for U2)  |
| 2   | SPI0 SCLK (shared)                                               |
| 3   | SPI0 MISO (shared)                                               |
| 4   | SPI0 MOSI (shared)                                               |
| 5   | Encoder **A** for axis‑x, with **4.7 kΩ pull‑up to 3.3 V**       |
| 6   | Encoder **B** for axis‑x, with **4.7 kΩ pull‑up to 3.3 V**       |
| 7   | `Zx` — unused: **tie to GND** (Z disabled in MCR0).              |
| 8   | GND                                                              |
| 9   | `CEx` — **tie to 3.3 V** (counting always enabled). Internal pull-up exists, but tying high is more robust. |
| 10  | `FLAGx/` — NC (or wire‑OR with U2 `FLAG/` to a single Pi GPIO)   |
| 11  | Encoder **A** for axis‑y, with **4.7 kΩ pull‑up to 3.3 V**       |
| 12  | Encoder **B** for axis‑y, with **4.7 kΩ pull‑up to 3.3 V**       |
| 13  | `Zy` — unused: **tie to GND**                                    |
| 14  | `CEy` — **tie to 3.3 V**                                         |
| 15  | `FLAGy/` — NC                                                    |
| 16  | `+3.3 V`, **0.1 µF to GND right at the pin**                     |

### Encoder mapping

| Machine axis | Chip | Chip axis | Pins on chip |
|--------------|------|-----------|--------------|
| X            | U1   | axis‑x    | A=5, B=6     |
| X′           | U1   | axis‑y    | A=11, B=12   |
| Y            | U2   | axis‑x    | A=5, B=6     |
| Z            | U2   | axis‑y    | A=11, B=12   |

### SPI mode

Per the LS7466 datasheet, Fig. 7 / setup notes:

- **SPI Mode 0** (CPOL = 0, CPHA = 0)
- MSB first
- SCK idles low; both MOSI shift and MISO shift happen on the falling edge of SCK
  (the master samples MISO on the rising edge of SCK).
- Communication cycle = 1 to 4 bytes, framed by SS/ low → high. First byte is
  always the **instruction byte**:
  - Bits [7:6] = opcode (00 RST, 01 RD, 10 WR, 11 LOAD)
  - Bits [5:3] = register select (MCR0/1, IDR, CNTR, ODR, SSTR, DSTR)
  - Bits [2:1] = axis select (00 = x, 01 = y, 1x = both — `RD` ignores `both`)
  - Bit  [0]   = 1 ⇒ auto‑transfer DSTR→SSTR on `RD_CNTR` (handy for status correlation)
- SCK ≤ 8 MHz at 3.3 V is plenty.

### Recommended register configuration

For each axis (write **per chip per axis**, i.e. four writes total for MCR0
and four for MCR1, or two writes using `axis = both`):

| Register | Value  | Meaning |
|----------|--------|---------|
| `MCR0`   | `0x03` | Z disabled, free‑run, **x4 quadrature** |
| `MCR1`   | `0x00` | Flags off, dynamic flag mode, counting enabled, **3‑byte (24‑bit) mode** |

Op-codes (from the datasheet):

```
WR_MCR0xy = 0x8c   ; write MCR0 to both axes of a chip
WR_MCR1xy = 0x94   ; write MCR1 to both axes of a chip
RST_CNTRx = 0x20   ; clear axis-x counter
RST_CNTRy = 0x22   ; clear axis-y counter
RD_CNTRx  = 0x60   ; read axis-x CNTR (returns 3 bytes in 3-byte mode)
RD_CNTRy  = 0x62   ; read axis-y CNTR (returns 3 bytes in 3-byte mode)
```

Initialization sequence per chip:

```
SS/↓  WR_MCR1xy 0x00  SS/↑      ; set 24-bit mode first
SS/↓  WR_MCR0xy 0x03  SS/↑      ; x4 quadrature, free-run
SS/↓  RST_CNTRx        SS/↑     ; zero axis-x
SS/↓  RST_CNTRy        SS/↑     ; zero axis-y
```

Periodic read of one axis:

```
SS/↓  RD_CNTRx  0x00 0x00 0x00  SS/↑   ; clock out 3 bytes of counter
```

Sign‑extend the 24‑bit value to a Go `int32`:

```go
raw := uint32(b[0])<<16 | uint32(b[1])<<8 | uint32(b[2])
if raw&0x800000 != 0 {
    raw |= 0xff000000      // sign-extend
}
count := int32(raw)
```

---

## 5. Encoders

Each encoder is connected via a 4‑pin cable: **A**, **B**, **+5 V**, **GND**,
landed on a 4‑position screw terminal (`J3` = X, `J4` = X′, `J5` = Y, `J6` = Z).
The encoder modules themselves run from the Pi's **+5 V** rail (header pin 2 or 4);
their A/B outputs are NPN open‑collector and are pulled up to **3.3 V** at the
LS7466 end by `R2`–`R9`, so the signal seen by the chip is a clean 3.3 V CMOS
level — never above the LS7466's `VDD`.

Encoder Z (index) is not used; the corresponding `Zx` / `Zy` pin on the chip
is tied to GND and disabled in `MCR0`. If you ever want to add index/homing
later, lift the GND tie, add a pull‑up to 3.3 V, and route the encoder Z to
that pin — then reprogram `MCR0` to one of the index modes (e.g. `RCNT` to
reset `CNTR` on Z).

Per the firmware defaults (`main.go`): 600 PPR, x4 quadrature → 2400
counts/rev; 50 mm wheel diameter, ≈157.08 mm/rev, ≈0.0654 mm/count.

The LS7466's max quadrature input rate at 3.3 V is **1.3 MHz**, which at
600 PPR works out to ≈130 000 RPM — far beyond anything this machine produces.

---

## 6. Foot switch

Connector **J2** (4‑pos screw terminal, 2 of the 4 positions wired):

| Net      | Connection                                       |
|----------|--------------------------------------------------|
| Switch.1 | GPIO 26 (header pin 37), 4.7 kΩ pull‑up (`R1`) to 3.3 V |
| Switch.2 | GND (header pin 39)                              |

Normally open, momentary. Software treats falling edge as "capture point",
with debounce and ≥500 ms minimum spacing in firmware.

---

## 7. Detailed wiring diagram

```
                           Raspberry Pi 40-pin header (J1)
   ┌───────────────────────────────────────────────────────────────────┐
   │ +3V3 (pin 1, 17) ──────────────► +3V3 rail ────► U1, U2 pin 16
   │                                                └─► all 4k7 pull-ups (R1..R9)
   │                                                └─► CEx (pin 9), CEy (pin 14)
   │ +5V  (pin 2, 4)  ──────────────► +5V rail  ────► J3..J6 (encoder modules)
   │ GND  (pin 6,9,...)─────────────► GND rail  ────► U1, U2 pin 8
   │                                                └─► Zx (pin 7), Zy (pin 13)
   │                                                └─► J2 foot switch
   │                                                └─► J3..J6 encoder GND
   │
   │ ── SPI0 ─────────────────────────────────────────────────────────── │
   │ GPIO10 (pin 19) MOSI ─────►  U1 pin 4,  U2 pin 4
   │ GPIO9  (pin 21) MISO ◄─────  U1 pin 3,  U2 pin 3
   │ GPIO11 (pin 23) SCLK ─────►  U1 pin 2,  U2 pin 2
   │ GPIO8  (pin 24) CE0  ─────►  U1 pin 1   (X, X')
   │ GPIO7  (pin 26) CE1  ─────►  U2 pin 1   (Y,  Z)
   │
   │ GPIO26 (pin 37) ◄── J2 foot switch ── GND;  R1=4.7kΩ to +3V3
   └────────────────────────────────────────────────────────────────────┘

  Each LS7466 (Ux), identical wiring on both:

                          +3.3 V
                            │
                ┌───────────┼───────────┐
              [4k7]       [4k7]         │
                │           │           │
   Encoder.A  ──┴──► pin 5  │           │           (axis-x A)
   Encoder.B  ─────► pin 6  │           │           (axis-x B)
                            │           │
   Encoder'.A ──────────────┴──► pin 11 │           (axis-y A)
   Encoder'.B ─────────────────► pin 12 │           (axis-y B)
                                        │
                            +3.3V ──────┴── pin 9  (CEx)
                            +3.3V ────────── pin 14 (CEy)
                            +3.3V ──┬─────── pin 16 (VDD)
                                    │
                                 [0.1µF]    ◄── decoupling (C1 / C2), at pin 16
                                    │
                                   GND
                                    │
                            GND ────┴─────── pin 8  (GND)
                            GND ───────────► pin 7  (Zx, unused)
                            GND ───────────► pin 13 (Zy, unused)

   Pin 1  (SS/)   : SPI0 CEx (CE0 for U1, CE1 for U2)
   Pin 2  (SCK)   : SPI0 SCLK (shared)
   Pin 3  (MISO)  : SPI0 MISO (shared)
   Pin 4  (MOSI)  : SPI0 MOSI (shared)
   Pin 10 (FLAGx/): NC
   Pin 15 (FLAGy/): NC

  +3V3 rail also carries C3 (10 µF bulk) to GND, placed near J1.
```

---

## 8. Layout / signal‑integrity notes

- **Decoupling first.** Each LS7466 gets its own 0.1 µF directly across
  pins 16 ↔ 8 with the shortest possible loop. One 10 µF bulk cap somewhere
  on the 3.3 V rail is enough.
- **Encoder traces.** Pull‑ups should sit near the LS7466 end (the receiver),
  not at the connector — that gives the cleanest edges into the on‑chip filter.
- **Ground.** Single ground plane. Star ground back to the Pi via the header's
  GND pins; don't share encoder return current with the Pi power return if
  encoder cables are long.
- **ESD.** Encoder inputs go to a connector and out into the world; if you
  expect rough handling, add a TVS or 5 V Zener on each A/B line, or use the
  optional 100 Ω + 1 nF RC snubber listed in the BOM.

## 9. Reference

- LS7466 datasheet: <https://lsicsi.com/wp-content/uploads/2024/04/LS7466.pdf>
- BCM2711 / RPi GPIO reference: <https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#gpio>
