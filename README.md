# closinuf

**3D point catcher** — live 3D coordinates from four quadrature encoders on a Raspberry Pi, with a small web UI, foot‑switch capture, and FreeCAD‑friendly export.

## What it does

- Tracks **X**, **X'**, **Y**, **Z** from dedicated rotary encoders.  
- **Capture Point** in the browser or a **GPIO foot switch** appends the current **(X, Y, Z)** to a list (mm internally).
- **Save** downloads an **ASC** point cloud file, which can be imported into FreeCAD as a point cloud. 
- **Units** cycles mm → m → in → ft. **Zero** clears counts and points.
- Optional **short beep** on capture if speakers are available.

## Hardware

See [HARDWARE.md](HARDWARE.md).

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

3. Reboot if the installer prompts you to (it offers when boot config changed).

What the script does:

- Enables **SPI** and relocates kernel CE pins off GPIO 8/7 (`dtoverlay=spi0-2cs,cs0_pin=12,cs1_pin=13`) in `/boot/firmware/config.txt` (backs up first; skips lines already present).
- Installs **`golang-go`** via `apt` if `go` is missing.
- Builds **`./closinuf`** in the repo.
- Adds the install user to **`spi`** and **`gpio`** groups when those groups exist.
- Installs **closinuf** **systemd** services: **`closinuf.service`** (app on :3000), **`closinuf-browser.service`** (open Chromium after boot)
- Runs **`closinuf-setup-gpclk.sh`** as **ExecStartPre** (root) for GPCLK0 ~9 MHz on GPIO4; `main` programs GPCLK only when run as root
- Offers to **reboot** when SPI/GPCLK settings were added (`sudo ./install.sh --reboot` to reboot without prompting)

Useful commands:

```bash
systemctl status closinuf closinuf-browser
journalctl -u closinuf -f
```

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

Fiber, HTMX, gomponents, **LS7366R** counters over **SPI0**, **go-gpiocdev** (chip selects + foot switch).

## License

MIT — see [LICENSE](LICENSE).

## Author

Scott Feldman (2026)
