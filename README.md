# closinuf

**3D point catcher** — live 3D coordinates from four quadrature encoders on a Raspberry Pi, with a small web UI, foot‑switch capture, and FreeCAD‑friendly export.

## What it does

- Tracks **X**, **X'**, **Y**, **Z** from dedicated rotary encoders.  
- **Capture Point** in the browser or a **GPIO foot switch** appends the current **(X, Y, Z)** to a list (mm internally).
- **Save** downloads an **ASC** point cloud file, which can be imported into FreeCAD as a point cloud. 
- **Units** cycles mm → m → in → ft. **Zero** clears counts and points.
- Optional **short beep** on capture if speakers are available.

## Hardware

| Axis | GPIO (A / B) |
|------|----------------|
| X    | 2 / 3          |
| X′   | 22 / 27        |
| Y    | 9 / 10         |
| Z    | 5 / 11         |

- Encoders: **600 PPR** quadrature (2400 counts/rev) and **50 mm** wheel diameter match the defaults in `main.go`; change constants there if your hardware differs.  Code assumes external 4.7K pull-up resistors for encoder inputs.
- Full board schematic, BOM, and pinout: see [HARDWARE.md](HARDWARE.md).
- **Foot / capture switch**: GPIO **26**, normally‑open, pulled **high** (e.g. 4.7 kΩ to 3.3 V).

## Install on the Pi (`install.sh`)

1. On the Pi, clone the repository and enter the project directory:

   ```bash
   git clone https://github.com/scottfeldman/closinuf.git
   cd closinuf
   ```

2. Run the install script as root:

   ```bash
   sudo ./install.sh
   ```

What the script does:

- Installs **`golang-go`** via `apt` if `go` is missing.
- Builds **`./closinuf`** in the repo.
- Installs **closinuf** **systemd** services: **`closinuf.service`** (app on :3000), **`closinuf-browser.service`** (open Chromium after boot)

Useful commands:

```bash
systemctl status closinuf closinuf-browser
journalctl -u closinuf -f
```

A reboot is recommended so graphical login and the browser unit start in a clean order. For GPIO, ensure the service user can use **`/dev/gpiochip*`** (e.g. `gpio` group or equivalent on your image).

**Touch keyboard:** fullscreen Chromium can block some on‑screen keyboards; if the filename field stays hidden behind the keyboard, try launching the browser **maximized** instead of fullscreen (edit the `chromium` line in `/usr/local/bin/closinuf-browser.sh`). The UI adds extra **bottom padding** so you can scroll content above the keyboard.

**Capture beep:** install **`alsa-utils`** (`aplay`) or ensure **`paplay`** works so the Pi can play the short tone on capture.

## Manual run (development)

```bash
git clone https://github.com/scottfeldman/closinuf.git
cd closinuf
go build -o closinuf .
./closinuf
```

Open `http://127.0.0.1:3000`. Use `sudo` only if your user cannot access GPIO.

## ASC export

One point per line: `X Y Z` in **millimeters** (space‑separated), suitable for FreeCAD point cloud import.

## Stack

Fiber, HTMX, gomponents, **go-gpiocdev** (`gpiochip` character device).

## License

MIT — see [LICENSE](LICENSE).

## Author

Scott Feldman (2026)
