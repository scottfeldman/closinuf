# closinuf

**3D Point Catcher** - A real-time 3D coordinate tracking system using rotary encoders on Raspberry Pi.

## Overview

closinuf is a web-based application that tracks 3D positions using three rotary encoders (X, Y, Z axes) connected to a Raspberry Pi via GPIO. It provides real-time position monitoring, point capture, and export functionality for use with CAD software like FreeCAD.

## Features

- **Real-time Position Tracking**: Monitor X, Y, Z coordinates with live updates (200ms refresh rate)
- **Multiple Unit Support**: Display distances in millimeters (mm), meters (m), inches (in), or feet/inches (ft/in)
- **Point Capture**: Save 3D coordinates at any moment via web interface or physical button
- **RPM Monitoring**: Track rotation speed for each encoder axis
- **FreeCAD Export**: Save captured points in ASC format compatible with FreeCAD point clouds
- **Modern Web Interface**: Clean, terminal-style UI with HTMX for dynamic updates
- **Physical Button Support**: Optional GPIO-connected button for hands-free point capture
- **Zero/Reset Function**: Reset all encoder counts and clear captured points

## Hardware Requirements

- Raspberry Pi (with GPIO access)
- 3x Rotary Encoders (600 PPR recommended)
  - X-axis encoder: GPIO pins 2 & 3
  - Y-axis encoder: GPIO pins 5 & 6
  - Z-axis encoder: GPIO pins 17 & 27
- Optional: Physical button for point capture (GPIO pin 23)
- Encoder wheels: 50mm diameter (configurable in code)

## Software Requirements

- Go 1.25.3 or later
- Raspberry Pi OS (or compatible Linux distribution)
- GPIO access permissions (user should be in `gpio` group or run with appropriate permissions)

## Installation

1. **Clone the repository**:
   ```bash
   git clone <repository-url>
   cd closinuf
   ```

2. **Install dependencies**:
   ```bash
   cd site
   go mod download
   ```

3. **Build the application**:
   ```bash
   go build -o closinuf main.go
   ```

4. **Run the application**:
   ```bash
   sudo ./closinuf
   ```
   
   Note: `sudo` may be required for GPIO access depending on your system configuration.

5. **Access the web interface**:
   Open your browser and navigate to `http://<raspberry-pi-ip>:3000`

## Usage

### Web Interface

- **View Position**: The main page displays real-time X, Y, Z coordinates
- **Change Units**: Click the "Units" button to cycle through mm → m → in → ft
- **Capture Point**: Click "Capture Point" to save current coordinates
- **Zero Counts**: Click "Zero All Counts" to reset all encoders and clear points
- **Save Points**: Enter a filename and click "Save" to download points as an ASC file

### Physical Button

If a button is connected to GPIO 23:
- Press the button to capture a point at the current position
- Button includes debouncing to prevent duplicate captures

### API Endpoints

- `GET /` - Main web interface
- `GET /api/encoder` - Get encoder data as JSON
- `GET /api/encoder/htmx` - Get encoder display fragment (HTML)
- `GET /api/units/cycle` - Cycle through unit systems
- `POST /api/encoder/zero` - Reset all encoder counts and clear points
- `POST /api/points/add` - Add a point at current coordinates
- `GET /api/points/count` - Get number of captured points
- `GET /api/points/save?filename=<name>` - Download points as ASC file

## File Format

Points are saved in ASC (ASCII) format compatible with FreeCAD:
```
X Y Z
X Y Z
...
```

Each line contains space-separated floating-point coordinates in millimeters.

## Configuration

Key constants in `main.go`:
- `countsPerRev`: 2400 (600 PPR × 4 for quadrature encoding)
- `wheelDiameter`: 50.0 mm
- GPIO pin assignments (see Hardware Requirements section)

## Technical Details

- **Encoder Resolution**: 600 PPR (2400 counts per revolution with quadrature)
- **Update Rate**: 200ms for web display, 100ms for RPM calculation
- **Web Framework**: Fiber (Go)
- **Frontend**: HTMX for dynamic updates
- **GPIO Library**: go-gpiocdev

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Author

Scott Feldman (2026)
