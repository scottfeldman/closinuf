# closinuf — Hardware Design

This document captures the **encoder counter board** that sits between the four
quadrature encoders and the Raspberry Pi. The board is a **Raspberry Pi 4 HAT**:
it stacks directly onto the Pi's 40‑pin GPIO header via `J1` and takes all of
its power from that header (3.3 V and 5 V). Counting is done in hardware by four
**LS7366R** single‑channel 32‑bit quadrature counter ICs (one encoder each), read by the Pi over **SPI0**.

The PCB sources for this design live in [`pcb/`](pcb/). This file is the
canonical reference for the schematic; if the two ever disagree, treat this
document as the spec.

---

## 1. System overview

```
                                                  +3.3V
                                                    │
                                                    ▼
   ┌──────────────┐   SPI0 + GPCLK0           ┌──────────────┐
   │              │   MOSI/MISO/SCLK ──┬─────►│ LS7366R  U1  │── Encoder X
   │   Raspberry  │   SS/ per chip      │      └──────────────┘
   │     Pi       │                     │      ┌──────────────┐
   │              │   GPIO4 (GPCLK0) ───┼─────►│ LS7366R  U2  │── Encoder X′
   │              │   (pin 7) ─ fCKi    │      └──────────────┘
   │              │   all four ICs       │      ┌──────────────┐
   │              │                     ├─────►│ LS7366R  U3  │── Encoder Y
   │              │                     │      └──────────────┘
   │              │                     │      ┌──────────────┐
   │              │                     └─────►│ LS7366R  U4  │── Encoder Z
   │  GPIO 26 ◄───┼── foot switch               └──────────────┘
   └──────────────┘
```

Four ICs total (one quadrature counter each). The Pi drives a shared **filter
clock** into every chip’s **fCKi** from **GPCLK0 on GPIO4** (header pin 7), and
periodically selects a chip, issues `READ_CNTR`, and reads 3 or 4 bytes
(depending on `MDR1` counter width).

### Counter range

The LS7366R `CNTR` is programmable for 8 / 16 / 24 / 32‑bit operation via `MDR1`.
If you use **3‑byte (24‑bit) mode** to match the previous LS7466 behavior, with
600 PPR encoders at x4 quadrature (2400 counts/rev) on a 50 mm wheel (≈157.08 mm/rev),
interpreting `CNTR` as signed two's complement:

- Half range: \(2^{23}\) = 8 388 608 counts ≈ **3 495 revolutions** in either
  direction from zero
- Linear travel: ≈ **549 m (≈1801 ft) one‑way** before wrap

In **4‑byte (32‑bit) mode**, signed headroom is \(2^{31}\) counts — far more than
this application needs.

---

## 2. Bill of materials

| Ref       | Qty | Part                                  | Suggested MPN                  | KiCad footprint                                      | Notes |
|-----------|-----|---------------------------------------|--------------------------------|------------------------------------------------------|-------|
| U1–U4     | 4   | **LS7366R‑S** (SOIC‑14)               | `LS7366R-S`                    | `Package_SO:SOIC-14_3.9x8.7mm_P1.27mm`               | Single‑channel 32‑bit quadrature counter with SPI. Listings often show **LS7366‑R** / **LS7366R** for the RoHS **DIP‑14** variant; use **‑S** for surface mount. |
| C1–C4     | 4   | 0.1 µF, X7R, 25 V, 0603, ±10 %        | Murata `GRM188R71E104KA01D`    | `Capacitor_SMD:C_0603_1608Metric`                    | Decoupling, **at pin 14 (VDD)** of each chip. |
| C5        | 1   | 10 µF, X5R, 10 V, 0805, ±10 %         | Murata `GRM21BR61A106KE19L`    | `Capacitor_SMD:C_0805_2012Metric`                    | Bulk on the 3.3 V rail. Use ≥10 V part to avoid DC‑bias derating loss at 3.3 V. |
| R1        | 1   | 4.7 kΩ, 1 %, 1/10 W, 0603             | Yageo `RC0603FR-074K7L`        | `Resistor_SMD:R_0603_1608Metric`                     | Foot‑switch pull‑up to 3.3 V. |
| R2–R9     | 8   | 4.7 kΩ, 1 %, 1/10 W, 0603             | Yageo `RC0603FR-074K7L`        | `Resistor_SMD:R_0603_1608Metric`                     | Pull‑ups on every encoder A and B (2 per encoder × 4 encoders). |
| J1        | 1   | 2×20 0.1″ socket                      | Samtec `SSW-120-01-T-D` (or any 2×20 2.54 mm socket) | `Connector_PinSocket_2.54mm:PinSocket_2x20_P2.54mm_Vertical` | Pi GPIO header connector. |
| J2        | 1   | 4‑pos PCB terminal block, 5 mm pitch, horizontal entry | Phoenix Contact `PT 1,5/ 4-5,0-H` (1935284) | `TerminalBlock_Phoenix:TerminalBlock_Phoenix_PT-1,5-4-5.0-H_1x04_P5.00mm_Horizontal` | Foot switch — only 2 of the 4 positions are wired (GPIO 26 + GND). |
| J3–J6     | 4   | 4‑pos PCB terminal block, 5 mm pitch, horizontal entry | Phoenix Contact `PT 1,5/ 4-5,0-H` (1935284) | `TerminalBlock_Phoenix:TerminalBlock_Phoenix_PT-1,5-4-5.0-H_1x04_P5.00mm_Horizontal` | Encoder cables (X, X′, Y, Z). Each carries A, B, **+5 V**, GND. |
| —         | —   | Optional: 4× (100 Ω + 1 nF)           | —                              | —                                                    | RC snubber on each A/B if encoder cables are long (>1 m). |
| —         | —   | Optional: 1× 4.7 kΩ + 1 GPIO          | —                              | —                                                    | Pull‑up for wire‑OR’d `FLAG/` interrupt if you ever wire it. |

All passives are surface‑mount: caps and resistors in 0603 (with `C5` in 0805 for
better DC‑bias performance). MPNs above are stocked at Digi‑Key / Mouser / LCSC
and are interchangeable with the equivalent parts from Kemet, Panasonic, Vishay,
TDK, Samsung, or Yageo at the same package and dielectric. The KiCad footprints
in the table are the standard parts shipped with KiCad's stock libraries; the
schematic in `pcb/encoder.kicad_sch` should assign them for every component
(U1–U4, C1–C4, C5, R1–R9, J1–J6) once updated for LS7366R.

---

## 3. Raspberry Pi pin assignment

40‑pin header. Only the pins relevant to this board are listed.

| Function                     | RPi GPIO (BCM) | Header pin |
|------------------------------|----------------|-----------:|
| SPI0 MOSI                    | GPIO 10        | 19         |
| SPI0 MISO                    | GPIO 9         | 21         |
| SPI0 SCLK                    | GPIO 11        | 23         |
| SS/ → U1 (encoder X)         | GPIO 8 (CE0)   | 24         |
| SS/ → U2 (encoder X′)        | GPIO 7 (CE1)   | 26         |
| SS/ → U3 (encoder Y)         | GPIO 5         | 29         |
| SS/ → U4 (encoder Z)         | GPIO 6         | 31         |
| **GPCLK0** → all **fCKi**    | GPIO 4         | 7          |
| **Foot switch** (`J2`)       | GPIO 26        | 37         |
| 3.3 V supply (LS7366R VDD)   | —              | 1, 17      |
| 5 V supply (encoder modules) | —              | 2, 4       |
| GND                          | —              | 6, 9, 14, 20, 25, 30, 34, 39 |

Notes:

- SPI1 is **not used**. All four LS7366R devices share **SPI0** MOSI, MISO, and
  SCLK; only **SS/** is unique per chip. Linux exposes **CE0** and **CE1** as
  GPIO 8 and 7; **U3** and **U4** use GPIO 5 and 6 as **manual** chip selects
  (drive high when idle, assert low during a transfer for that IC only).
- **Filter clock:** Tie **fCKi** (pin 2) on **U1–U4** together and connect to
  **GPIO4 / GPCLK0** (header pin 7). Configure the Pi to output a continuous
  square wave in the MHz range (see below). Per the datasheet, the internal
  filter clock \(f_f\) must satisfy \(f_f \ge 4 f_{QA}\) where \(f_{QA}\) is the
  maximum frequency on encoder **A** in quadrature mode; at 3.3 V, \(f_{QA}\) is
  rated up to **4.5 MHz**, so a **9.6 MHz** (or higher) GPCLK is a comfortable
  choice. Leave **fCKO** (pin 1) **unconnected** when **fCKi** is driven by the
  Pi (no crystal between pins 1 and 2).
- LS7366R devices, the pull‑up resistors, and the foot‑switch network all run
  from the Pi's **3.3 V** rail (header pins 1 / 17). The four encoder modules
  run from the Pi's **5 V** rail (header pins 2 / 4); their open‑collector
  A/B outputs are pulled up to 3.3 V at the LS7366R end so signal levels stay
  inside the chip's input range. Four LS7366Rs plus the pull‑ups still keep
  3.3 V load modest; encoder current is dominated by the encoder modules
  themselves (typically tens of mA each — check your encoder spec).
- GPIO 5 / 6 for **U3** / **U4** **SS/** can be reassigned if they conflict with
  another HAT — any spare GPIO with suitable 3.3 V I/O is fine.

### `/boot/firmware/config.txt`

```ini
dtparam=spi=on
```

**GPCLK on GPIO4 (pin 7):** there is no single standard `config.txt` line on all
Pi OS images. Typical approaches: a small **device‑tree overlay** that claims
GPCLK0 on GPIO4 at the desired frequency, or **user‑space** setup via `pigpio`,
`libgpiod`, or direct CM / `clk` register programming after boot. Aim for a
stable MHz‑range clock on pin 7 before or as soon as encoder reads begin.
See [GPCLK / pinout](https://pinout.xyz/pinout/gpclk) and the LS7366R `fCKi`
filter requirements above.

---

## 4. LS7366R wiring (per chip)

Pinout (DIP‑14 / SOIC‑14, top view — per LSI/CSI datasheet):

```
                 LS7366R
               ┌────────────┐
      fCKO   1 ┤            ├ 14   VDD
      fCKi   2 ┤            ├ 13   CNT_EN
       VSS   3 ┤            ├ 12   A
       SS/   4 ┤            ├ 11   B
       SCK   5 ┤            ├ 10   INDEX/
      MISO   6 ┤            ├  9   DFLAG/
      MOSI   7 ┤            ├  8   LFLAG/
               └────────────┘
```

Wiring is **identical** for U1–U4 except **SS/** and which encoder A/B pair
connects to pins 12 / 11.

| Pin | Net |
|-----|-----|
| 1   | **fCKO** — **NC** (Pi drives **fCKi**; no crystal). |
| 2   | **fCKi** — **GPCLK0 / GPIO4** (header pin 7), **shared** by U1–U4. |
| 3   | GND (`VSS`) |
| 4   | **SS/** — chip select (GPIO 8 / 7 / 5 / 6 for U1–U4 respectively). |
| 5   | SPI0 SCLK (shared) |
| 6   | SPI0 MISO (shared) |
| 7   | SPI0 MOSI (shared) |
| 8   | `LFLAG/` — NC |
| 9   | `DFLAG/` — NC |
| 10  | `INDEX/` — **tie to 3.3 V** (index disabled in `MDR0`; active‑low pin). |
| 11  | Encoder **B**, **4.7 kΩ pull‑up to 3.3 V** |
| 12  | Encoder **A**, **4.7 kΩ pull‑up to 3.3 V** |
| 13  | `CNT_EN` — **tie to 3.3 V** (count enable; internal pull‑up exists). |
| 14  | **+3.3 V**, **0.1 µF to GND** at the pin |

### Encoder mapping

| Machine axis | Chip | Encoder A | Encoder B |
|--------------|------|-----------|-----------|
| X            | U1   | pin 12    | pin 11    |
| X′           | U2   | pin 12    | pin 11    |
| Y            | U3   | pin 12    | pin 11    |
| Z            | U4   | pin 12    | pin 11    |

### SPI mode

Per the LS7366R datasheet (Figure 2 / setup notes):

- **SPI Mode 0** (CPOL = 0, CPHA = 0): SCK idles low.
- MSB first on MOSI and MISO.
- Framed by **SS/** low → … transfer … → **SS/** high; only one device’s **SS/**
  must be low at a time so **MISO** can be shared.
- At **3.3 V**, the datasheet specifies **120 ns** minimum SCK high and low times,
  implying roughly **≤ ~4 MHz** SCK unless you verify timing at your supply and
  temperature.

### Recommended register configuration

Configure **each** of U1–U4 the same way. Example for **3‑byte (24‑bit)** counter
width (closest to the old LS7466 setup):

| Register | Value  | Meaning |
|----------|--------|---------|
| `MDR0`   | `0x03` | x4 quadrature, free‑running, index disabled, filter divide = 1 |
| `MDR1`   | `0x01` | 3‑byte counter mode, counting enabled, flags off |

Use **4‑byte mode** (`MDR1` = `0x00` for width nibble) if you want the full
32‑bit counter; adjust read length to four data bytes after `READ_CNTR`.

Instruction bytes (datasheet / application listing):

```
WRITE_MDR0 = 0x88
WRITE_MDR1 = 0x90
CLR_CNTR   = 0x20
READ_CNTR  = 0x60   ; latches CNTR → OTR, then clocks out OTR on MISO
```

Initialization sequence **per chip**:

```
SS/↓  WRITE_MDR1 0x01  SS/↑     ; 3-byte mode, counting enabled
SS/↓  WRITE_MDR0 0x03  SS/↑     ; x4 quadrature, free-run, no index
SS/↓  CLR_CNTR          SS/↑   ; clear counter
```

Periodic read (3‑byte mode):

```
SS/↓  READ_CNTR  then clock 3 dummy / read bytes on MISO  SS/↑
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
LS7366R end by `R2`–`R9`, so the signal seen by the chip is a clean 3.3 V CMOS
level — never above the LS7366R's `VDD`.

Encoder Z (index) is not used; each chip's **`INDEX/`** pin is tied to **3.3 V**
and index is disabled in `MDR0`. If you ever want homing on index, lift the
3.3 V tie, add a pull‑up, route the encoder index to **`INDEX/`**, and set the
`MDR0` index field to the desired mode (load / reset / load OTR).

Per the firmware defaults (`main.go`): 600 PPR, x4 quadrature → 2400
counts/rev; 50 mm wheel diameter, ≈157.08 mm/rev, ≈0.0654 mm/count.

The LS7366R's max quadrature input rate at 3.3 V is **4.5 MHz** on A/B (with
`fCKi` and filter settings that meet \(f_f \ge 4 f_{QA}\)), which at 600 PPR is
still far beyond anything this machine produces.

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
   │ +3V3 (pin 1, 17) ──────────────► +3V3 rail ────► U1..U4 pin 14
   │                                                └─► all 4k7 pull-ups (R1..R9)
   │                                                └─► CNT_EN (pin 13), INDEX/ (pin 10)
   │ +5V  (pin 2, 4)  ──────────────► +5V rail  ────► J3..J6 (encoder modules)
   │ GND  (pin 6,9,...)─────────────► GND rail  ────► U1..U4 pin 3 (VSS)
   │                                                └─► J2 foot switch
   │                                                └─► J3..J6 encoder GND
   │
   │ ── SPI0 ─────────────────────────────────────────────────────────── │
   │ GPIO10 (pin 19) MOSI ─────►  U1..U4 pin 7
   │ GPIO9  (pin 21) MISO ◄─────  U1..U4 pin 6
   │ GPIO11 (pin 23) SCLK ─────►  U1..U4 pin 5
   │ GPIO8  (pin 24) CE0  ─────►  U1 pin 4   (encoder X)
   │ GPIO7  (pin 26) CE1  ─────►  U2 pin 4   (encoder X′)
   │ GPIO5  (pin 29)        ───►  U3 pin 4   (encoder Y)   manual SS/
   │ GPIO6  (pin 31)        ───►  U4 pin 4   (encoder Z)   manual SS/
   │
   │ GPIO4  (pin 7)  GPCLK0 ───►  U1..U4 pin 2 (fCKi), shared
   │
   │ GPIO26 (pin 37) ◄── J2 foot switch ── GND;  R1=4.7kΩ to +3V3
   └────────────────────────────────────────────────────────────────────┘

  Each LS7366R (Ux): one encoder; pin 1 (fCKO) NC; pin 2 (fCKi) = shared GPCLK.

                          +3.3 V
                            │
                ┌───────────┴───────────┐
                │                       │
   Encoder.A  ──┴──► pin 12   +3.3V ────► pin 10 (INDEX/)
   Encoder.B  ─────► pin 11   +3.3V ────► pin 13 (CNT_EN)
                            +3.3V ──┬─────── pin 14 (VDD)
                                    │
                                 [0.1µF]    ◄── decoupling (C1..C4), at pin 14
                                    │
                                   GND
                                    │
                            GND ────┴─────── pin 3  (VSS)

   Pin 4  (SS/)   : GPIO 8 / 7 / 5 / 6 for U1..U4
   Pin 5  (SCK)   : SPI0 SCLK (shared)
   Pin 6  (MISO)  : SPI0 MISO (shared)
   Pin 7  (MOSI)  : SPI0 MOSI (shared)
   Pin 8  (LFLAG/): NC
   Pin 9  (DFLAG/): NC

  +3V3 rail also carries C5 (10 µF bulk) to GND, placed near J1.
```

---

## 8. Layout / signal‑integrity notes

- **Decoupling first.** Each LS7366R gets its own 0.1 µF directly across
  pins 14 ↔ 3 with the shortest possible loop. One 10 µF bulk cap somewhere
  on the 3.3 V rail is enough.
- **fCKi routing.** Keep the **GPCLK** net short and matched to all four **fCKi**
  inputs; optional series damping (e.g. 22 Ω) at the source can calm reflections.
- **Encoder traces.** Pull‑ups should sit near the LS7366R end (the receiver),
  not at the connector — that gives the cleanest edges into the on‑chip filter.
- **Ground.** Single ground plane. Star ground back to the Pi via the header's
  GND pins; don't share encoder return current with the Pi power return if
  encoder cables are long.
- **ESD.** Encoder inputs go to a connector and out into the world; if you
  expect rough handling, add a TVS or 5 V Zener on each A/B line, or use the
  optional 100 Ω + 1 nF RC snubber listed in the BOM.

## 9. Reference

- LS7366R datasheet: <https://lsicsi.com/wp-content/uploads/2021/06/LS7366R.pdf>
- BCM2711 / RPi GPIO reference: <https://www.raspberrypi.com/documentation/computers/raspberry-pi.html#gpio>
