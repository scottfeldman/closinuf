package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	. "maragu.dev/gomponents/html"
)

func main() {
	if err := initEncoders(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}
	if err := initPointButton(); err != nil {
		fmt.Fprintf(os.Stderr, "Fatal: %v\n", err)
		os.Exit(1)
	}

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// CORS middleware
	app.Use(cors.New())

	// Serve static HTML page
	app.Get("/", func(c *fiber.Ctx) error {
		data := getEncoderData()
		unit := c.Query("unit", "mm") // Default to mm
		c.Type("html")
		return page(data, unit).Render(c)
	})

	// HTMX endpoint that returns HTML fragment
	app.Get("/api/encoder/htmx", func(c *fiber.Ctx) error {
		data := getEncoderData()
		unit := c.Query("unit", "mm") // Default to mm
		c.Type("html")
		return encoderFragment(data, unit).Render(c)
	})

	// Cycle units endpoint - redirects to page with new unit
	app.Get("/api/units/cycle", func(c *fiber.Ctx) error {
		currentUnit := c.Query("unit", "mm")
		if currentUnit == "" {
			currentUnit = "mm"
		}

		// Cycle: mm -> m -> in -> ft -> mm
		var nextUnit string
		switch currentUnit {
		case "mm":
			nextUnit = "m"
		case "m":
			nextUnit = "in"
		case "in":
			nextUnit = "ft"
		case "ft":
			nextUnit = "mm"
		default:
			nextUnit = "mm"
		}

		// Redirect to page with new unit parameter
		c.Set("HX-Redirect", "/?unit="+nextUnit)
		playBeep()
		return c.SendStatus(200)
	})

	// Zero endpoint to reset all encoder counts and clear points
	app.Post("/api/encoder/zero", func(c *fiber.Ctx) error {
		if err := clearHardwareCounters(); err != nil {
			return c.Status(500).SendString(err.Error())
		}
		zeroEncoderCounts()
		clearCapturePoints()
		playBeep()
		return c.SendStatus(200)
	})

	app.Post("/api/points/add", func(c *fiber.Ctx) error {
		addCapturePoint()
		playBeep()
		return c.SendStatus(200)
	})

	app.Get("/api/points/count", func(c *fiber.Ctx) error {
		c.Type("html")
		return g.Text(fmt.Sprintf("Points: %d", capturePointCount())).Render(c)
	})

	// Check and save points endpoint - validates points before saving
	app.Get("/api/points/check-save", func(c *fiber.Ctx) error {
		filename := c.Query("filename", "points.asc")
		if filename == "" {
			filename = "points.asc"
		}
		// Add .asc extension if not present
		if len(filename) < 4 || filename[len(filename)-4:] != ".asc" {
			filename += ".asc"
		}

		count := capturePointCount()

		if count == 0 {
			c.Type("html")
			// Return empty response for main swap, error message via oob
			return g.Raw(`<div id="save-error" hx-swap-oob="true" class="save-error">No points to save. Please capture some points first.</div>`).Render(c)
		}

		// If points exist, clear any error message and redirect to actual save endpoint
		c.Type("html")
		c.Set("HX-Redirect", "/api/points/save?filename="+filename)
		return g.Raw(`<div id="save-error" hx-swap-oob="true" style="display: none;"></div>`).Render(c)
	})

	// Save points endpoint - saves to ASC file (FreeCAD point cloud format)
	app.Get("/api/points/save", func(c *fiber.Ctx) error {
		filename := c.Query("filename", "points.asc")
		if filename == "" {
			filename = "points.asc"
		}
		// Add .asc extension if not present
		if len(filename) < 4 || filename[len(filename)-4:] != ".asc" {
			filename += ".asc"
		}

		ascData, err := capturePointsASC()
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "No points to save"})
		}

		playBeep()
		// Set headers for file download
		c.Set("Content-Type", "text/plain")
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		return c.SendString(ascData)
	})

	// Start server in goroutine
	go func() {
		os.Stdout.WriteString("Server is running, listening on :3000\n")
		if err := app.Listen(":3000"); err != nil {
			os.Stderr.WriteString("Failed to start server: " + err.Error() + "\n")
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	os.Stdout.WriteString("\nShutting down...\n")
}

// playBeep plays a short tone on the default audio output (non-blocking).
func playBeep() {
	go func() {
		f, err := os.CreateTemp("", "closinuf-beep-*.wav")
		if err != nil {
			tryPaplayBell()
			return
		}
		path := f.Name()
		remove := func() { os.Remove(path) }
		if err := writeBeepWAV(f, 880, 0.1); err != nil {
			f.Close()
			remove()
			tryPaplayBell()
			return
		}
		if err := f.Close(); err != nil {
			remove()
			tryPaplayBell()
			return
		}
		defer remove()
		if exec.Command("aplay", "-q", path).Run() != nil {
			tryPaplayBell()
		}
	}()
}

func tryPaplayBell() {
	_ = exec.Command("paplay", "/usr/share/sounds/freedesktop/stereo/bell.oga").Run()
}

func writeBeepWAV(w io.Writer, freqHz float64, durationSec float64) error {
	const (
		sampleRate    = 8000
		bitsPerSample = 16
		numChannels   = 1
	)
	n := int(sampleRate * durationSec)
	if n < 1 {
		return fmt.Errorf("duration too short")
	}
	byteRate := sampleRate * numChannels * bitsPerSample / 8
	blockAlign := numChannels * bitsPerSample / 8
	dataSize := n * blockAlign
	riffSize := 36 + uint32(dataSize)

	var hdr [44]byte
	copy(hdr[0:4], "RIFF")
	binary.LittleEndian.PutUint32(hdr[4:8], riffSize)
	copy(hdr[8:12], "WAVE")
	copy(hdr[12:16], "fmt ")
	binary.LittleEndian.PutUint32(hdr[16:20], 16)
	binary.LittleEndian.PutUint16(hdr[20:22], 1)
	binary.LittleEndian.PutUint16(hdr[22:24], numChannels)
	binary.LittleEndian.PutUint32(hdr[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(hdr[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(hdr[32:34], uint16(blockAlign))
	binary.LittleEndian.PutUint16(hdr[34:36], bitsPerSample)
	copy(hdr[36:40], "data")
	binary.LittleEndian.PutUint32(hdr[40:44], uint32(dataSize))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	const amp = 0.22 * 32767
	for i := 0; i < n; i++ {
		t := float64(i) / float64(sampleRate)
		sample := int16(amp * math.Sin(2*math.Pi*freqHz*t))
		if err := binary.Write(w, binary.LittleEndian, sample); err != nil {
			return err
		}
	}
	return nil
}

func page(data encoderData, unit string) g.Node {
	return HTML(
		Head(
			Meta(Charset("utf-8")),
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			TitleEl(g.Text("Rotary Encoder Monitor")),
			Script(Src("https://unpkg.com/htmx.org@2.0.3/dist/htmx.min.js")),
			StyleEl(g.Raw(`
				@import url('https://fonts.googleapis.com/css2?family=Orbitron:wght@400;700;900&display=swap');
				* {
					box-sizing: border-box;
				}
				html {
					scroll-padding-bottom: clamp(10rem, 48vh, 440px);
				}
				body {
					font-family: 'Courier New', 'Courier', monospace;
					min-height: 100vh;
					display: flex;
					flex-direction: column;
					justify-content: center;
					align-items: center;
					margin: 0;
					padding: 2rem;
					padding-bottom: clamp(10rem, 48vh, 440px);
					background: #0a0a0a;
					color: #00ff41;
					text-shadow: 0 0 2px #00ff41, 0 0 4px rgba(0, 255, 65, 0.35);
				}
				.container {
					position: relative;
					max-width: 1000px;
					width: 100%;
					background: #0d0d0d;
					border-radius: 8px;
					padding: 1.5rem;
					border: 2px solid #00ff41;
					box-shadow: 0 0 20px rgba(0, 255, 65, 0.3), inset 0 0 20px rgba(0, 255, 65, 0.05);
				}
				h1 {
					margin-top: 0;
					color: #00ff41;
					text-shadow: 0 0 2px #00ff41, 0 0 6px rgba(0, 255, 65, 0.4);
					font-family: 'Orbitron', monospace;
					font-weight: 700;
				}
				.encoder-display {
					display: flex;
					gap: 1rem;
					margin-bottom: 1rem;
					flex-wrap: wrap;
					justify-content: center;
				}
				.encoder-card {
					background: #0a0a0a;
					border-radius: 6px;
					padding: 1rem;
					border: 1px solid #00ff41;
					box-shadow: 0 0 10px rgba(0, 255, 65, 0.2), inset 0 0 10px rgba(0, 255, 65, 0.05);
					min-width: 200px;
					flex: 1;
					text-align: center;
				}
				.encoder-label {
					font-weight: bold;
					color: #00ff41;
					font-size: 1.2rem;
					margin-bottom: 0.5rem;
					text-shadow: 0 0 2px #00ff41, 0 0 5px rgba(0, 255, 65, 0.35);
					font-family: 'Orbitron', monospace;
					font-weight: 700;
				}
				.encoder-chip-ref {
					font-size: 0.75rem;
					color: #009922;
					margin-top: 0.25rem;
					text-shadow: 0 0 1px #009922;
					font-family: 'Courier New', monospace;
					font-weight: 400;
				}
				.encoder-distance {
					font-size: 2rem;
					font-weight: 700;
					color: #00ff41;
					line-height: 1.2;
					margin-bottom: 0.5rem;
					font-variant-numeric: tabular-nums;
					text-shadow: 0 0 2px #00ff41, 0 0 6px rgba(0, 255, 65, 0.4);
					font-family: 'Courier New', monospace;
				}
				.encoder-delta {
					font-size: 2rem;
					font-weight: 700;
					line-height: 1.2;
					margin-bottom: 0.5rem;
					font-variant-numeric: tabular-nums;
					font-family: 'Courier New', monospace;
				}
				.encoder-delta-zero {
					color: #00ff41;
					text-shadow: 0 0 2px #00ff41, 0 0 5px rgba(0, 255, 65, 0.35);
				}
				.encoder-delta-nonzero {
					color: #ff4444;
					text-shadow: 0 0 2px #ff4444, 0 0 5px rgba(255, 68, 68, 0.45);
				}
				.encoder-delta-nonzero .encoder-unit-large {
					color: #ff4444;
					text-shadow: 0 0 2px #ff4444, 0 0 5px rgba(255, 68, 68, 0.45);
				}
				.encoder-delta .encoder-unit-large {
					font-size: 1.5rem;
					margin-left: 0.25rem;
				}
				.encoder-unit-large {
					font-size: 1.5rem;
					color: #00ff41;
					margin-left: 0.25rem;
					font-weight: 400;
					text-shadow: 0 0 2px #00ff41;
				}
				.encoder-details {
					display: flex;
					flex-direction: column;
					gap: 0.25rem;
					font-size: 0.85rem;
					color: #00cc33;
					text-shadow: 0 0 1px #00cc33;
				}
				.encoder-detail-item {
					font-variant-numeric: tabular-nums;
				}
				.encoder-unit-small {
					color: #00cc33;
					margin-left: 0.15rem;
					text-shadow: 0 0 1px #00cc33;
				}
				.encoder-other-units {
					font-size: 0.75rem;
					color: #009922;
					margin-top: 0.25rem;
					text-shadow: 0 0 1px #009922;
				}
				.units-button, .zero-button, .point-button, .save-button {
					background: #0a0a0a;
					color: #00ff41;
					border: 2px solid #00ff41;
					padding: 0.75rem 1.5rem;
					border-radius: 4px;
					font-size: 1rem;
					font-weight: 600;
					cursor: pointer;
					transition: all 0.15s ease;
					font-family: 'Courier New', monospace;
					text-shadow: 0 0 2px #00ff41;
					box-shadow: 0 0 10px rgba(0, 255, 65, 0.3);
					position: relative;
					-webkit-tap-highlight-color: transparent;
				}
				.units-button:hover, .zero-button:hover, .point-button:hover, .save-button:hover {
					background: rgba(0, 255, 65, 0.1);
					box-shadow: 0 0 15px rgba(0, 255, 65, 0.5);
					text-shadow: 0 0 2px #00ff41, 0 0 5px rgba(0, 255, 65, 0.35);
				}
				.units-button:active, .zero-button:active, .point-button:active, .save-button:active {
					background: rgba(0, 255, 65, 0.25);
					box-shadow: 0 0 25px rgba(0, 255, 65, 0.8), 0 0 40px rgba(0, 255, 65, 0.4);
					text-shadow: 0 0 3px #00ff41, 0 0 7px rgba(0, 255, 65, 0.4);
					transform: scale(0.98);
					border-color: #00ff88;
				}
				.point-button {
					background: rgba(0, 255, 65, 0.15);
					border-color: #00ff41;
					box-shadow: 0 0 15px rgba(0, 255, 65, 0.4);
				}
				.point-button:hover {
					background: rgba(0, 255, 65, 0.25);
					box-shadow: 0 0 20px rgba(0, 255, 65, 0.6);
				}
				.point-button:active {
					background: rgba(0, 255, 65, 0.35);
					box-shadow: 0 0 30px rgba(0, 255, 65, 0.9), 0 0 50px rgba(0, 255, 65, 0.5);
				}
				.save-button {
					background: rgba(255, 200, 0, 0.1);
					border-color: #ffc800;
					color: #ffc800;
					text-shadow: 0 0 2px #ffc800;
					box-shadow: 0 0 10px rgba(255, 200, 0, 0.3);
				}
				.save-button:hover {
					background: rgba(255, 200, 0, 0.2);
					box-shadow: 0 0 15px rgba(255, 200, 0, 0.5);
					text-shadow: 0 0 2px #ffc800, 0 0 5px rgba(255, 200, 0, 0.45);
				}
				.save-button:active {
					background: rgba(255, 200, 0, 0.3);
					box-shadow: 0 0 25px rgba(255, 200, 0, 0.8), 0 0 40px rgba(255, 200, 0, 0.4);
					text-shadow: 0 0 3px #ffc800, 0 0 7px rgba(255, 200, 0, 0.45);
					border-color: #ffd700;
				}
				.button-container {
					text-align: center;
					margin-top: 2rem;
					display: flex;
					gap: 1rem;
					justify-content: center;
					align-items: center;
					flex-wrap: wrap;
				}
				.points-count {
					font-size: 1rem;
					color: #00ff41;
					font-weight: 500;
					padding: 0.75rem 1rem;
					background: #0a0a0a;
					border-radius: 6px;
					border: 1px solid #00ff41;
					box-shadow: 0 0 8px rgba(0, 255, 65, 0.2);
					text-shadow: 0 0 2px #00ff41;
				}
				.filename-input {
					padding: 0.75rem 1rem;
					border: 2px solid #00ff41;
					border-radius: 6px;
					font-size: 1rem;
					width: 150px;
					background: #0a0a0a;
					color: #00ff41;
					font-family: 'Courier New', monospace;
					text-shadow: 0 0 2px #00ff41;
					transition: all 0.2s;
					box-shadow: 0 0 8px rgba(0, 255, 65, 0.2);
				}
				.filename-input:focus {
					outline: none;
					border-color: #00ff41;
					box-shadow: 0 0 15px rgba(0, 255, 65, 0.5);
					text-shadow: 0 0 3px #00ff41;
				}
				.filename-input::placeholder {
					color: #009922;
					text-shadow: 0 0 1px #009922;
				}
				.save-group {
					display: flex;
					gap: 0.5rem;
					align-items: center;
				}
				.save-error {
					position: absolute;
					top: 50%;
					left: 50%;
					transform: translate(-50%, -50%);
					color: #ff0000;
					background: rgba(0, 0, 0, 0.95);
					border: 3px solid #ff0000;
					padding: 0.75rem 1rem;
					border-radius: 6px;
					text-align: center;
					white-space: nowrap;
					z-index: 1000;
					box-shadow: 0 0 20px rgba(255, 0, 0, 0.8), inset 0 0 10px rgba(255, 0, 0, 0.2);
					animation: fadeOut 0.5s ease-out 5s forwards;
					text-shadow: 0 0 2px #ff0000, 0 0 5px rgba(255, 0, 0, 0.45);
					font-weight: bold;
				}
				@keyframes fadeOut {
					from {
						opacity: 1;
					}
					to {
						opacity: 0;
						visibility: hidden;
					}
				}
			`)),
		),
		Body(
			Div(Class("container"),
				encoderFragment(data, unit),
				Div(Class("button-container"),
					Button(
						Class("point-button"),
						hx.Post("/api/points/add"),
						hx.Trigger("click"),
						hx.Swap("none"),
						hx.Target("#points-count"),
						hx.On("htmx:afterRequest", "htmx.trigger('#points-count', 'htmx:trigger')"),
						g.Text("Capture Point"),
					),
					Span(
						ID("points-count"),
						Class("points-count"),
						hx.Get("/api/points/count"),
						hx.Trigger("every 1s"),
						hx.Swap("innerHTML"),
						g.Text("Points: 0"),
					),
					Div(
						Class("save-group"),
						Input(
							ID("filename-input"),
							Name("filename"),
							Type("text"),
							Class("filename-input"),
							Value("points.asc"),
							Placeholder("filename.asc"),
						),
						Button(
							ID("save-button"),
							Class("save-button"),
							hx.Get("/api/points/check-save"),
							hx.Include("#filename-input"),
							hx.Swap("none"),
							hx.On("htmx:afterRequest", "htmx.trigger('#points-count', 'htmx:trigger')"),
							g.Text("Save"),
						),
					),
					Div(
						ID("save-error"),
						g.Attr("style", "display: none;"),
					),
					Div(
						g.Attr("style", "width: 100%; flex-basis: 100%;"),
					),
					Button(
						Class("units-button"),
						hx.Get("/api/units/cycle"),
						hx.Vals("js:{unit: new URLSearchParams(window.location.search).get('unit') || 'mm'}"),
						hx.Trigger("click"),
						hx.Swap("none"),
						g.Text("Units"),
					),
					Button(
						Class("zero-button"),
						hx.Post("/api/encoder/zero"),
						hx.Trigger("click"),
						hx.Swap("none"),
						hx.On("htmx:afterRequest", "document.getElementById('points-count').dispatchEvent(new Event('htmx:trigger'))"),
						g.Text("Zero All Counts"),
					),
				),
			),
		),
	)
}

func chipRef(label string) string {
	switch label {
	case "X":
		return "U1"
	case "X'":
		return "U2"
	case "Y":
		return "U3"
	case "Z":
		return "U4"
	default:
		return ""
	}
}

func encoderFragment(data encoderData, unit string) g.Node {
	return Div(
		hx.Get("/api/encoder/htmx"),
		hx.Trigger("every 200ms"),
		hx.Vals("js:{unit: new URLSearchParams(window.location.search).get('unit') || 'mm'}"),
		hx.Swap("outerHTML"),
		hx.Target("this"),
		ID("encoder-data"),
		Div(Class("encoder-display"),
			encoderDisplayXMerged(data.X, data.Xp, unit),
			encoderDisplay("Y", data.Y, unit),
			encoderDisplay("Z", data.Z, unit),
		),
	)
}

func formatFeetInchesFraction(mm float64) string {
	// Handle negative values
	isNegative := mm < 0
	absMM := mm
	if isNegative {
		absMM = -mm
	}

	// Convert mm to inches
	totalInches := absMM / 25.4
	feet := int(totalInches / 12)
	inches := totalInches - float64(feet*12)

	// Convert fractional part to nearest 1/16
	sixteenths := int(inches * 16)
	wholeInches := sixteenths / 16
	fractionalSixteenths := sixteenths % 16

	// Build the sign prefix
	sign := ""
	if isNegative {
		sign = "-"
	}

	if fractionalSixteenths == 0 {
		if feet > 0 {
			return fmt.Sprintf("%s%d' %d\"", sign, feet, wholeInches)
		}
		return fmt.Sprintf("%s%d\"", sign, wholeInches)
	}

	// Simplify fraction
	var num, den int
	switch fractionalSixteenths {
	case 1, 3, 5, 7, 9, 11, 13, 15:
		// Can't simplify odd numbers
		num = fractionalSixteenths
		den = 16
	case 2, 6, 10, 14:
		num = fractionalSixteenths / 2
		den = 8
	case 4, 12:
		num = fractionalSixteenths / 4
		den = 4
	case 8:
		num = 1
		den = 2
	}

	if feet > 0 {
		if wholeInches > 0 {
			return fmt.Sprintf("%s%d' %d-%d/%d\"", sign, feet, wholeInches, num, den)
		}
		return fmt.Sprintf("%s%d' %d/%d\"", sign, feet, num, den)
	}
	if wholeInches > 0 {
		return fmt.Sprintf("%s%d-%d/%d\"", sign, wholeInches, num, den)
	}
	return fmt.Sprintf("%s%d/%d\"", sign, num, den)
}

// distanceReadout formats distanceMM for the selected unit (primary display, unit suffix, other units line).
func distanceReadout(distanceMM float64, selectedUnit string) (selectedDisplay string, unitLabel g.Node, otherUnitsLine string) {
	distanceM := distanceMM / 1000.0
	distanceInches := distanceMM / 25.4
	distanceFeetInches := formatFeetInchesFraction(distanceMM)

	var selectedValue float64
	var selectedLabel string
	switch selectedUnit {
	case "m":
		selectedValue = distanceM
		selectedLabel = "m"
	case "in":
		selectedValue = distanceInches
		selectedLabel = "in"
	case "ft":
		selectedValue = 0
		selectedLabel = "ft"
	default:
		selectedValue = distanceMM
		selectedLabel = "mm"
	}

	if selectedUnit == "ft" {
		selectedDisplay = distanceFeetInches
	} else if selectedUnit == "m" {
		selectedDisplay = fmt.Sprintf("%.3f", selectedValue)
	} else if selectedUnit == "in" {
		selectedDisplay = fmt.Sprintf("%.3f", selectedValue)
	} else {
		selectedDisplay = fmt.Sprintf("%.2f", selectedValue)
	}

	otherUnits := []string{}
	if selectedUnit != "mm" {
		otherUnits = append(otherUnits, fmt.Sprintf("%.2f mm", distanceMM))
	}
	if selectedUnit != "m" {
		otherUnits = append(otherUnits, fmt.Sprintf("%.3f m", distanceM))
	}
	if selectedUnit != "in" {
		otherUnits = append(otherUnits, fmt.Sprintf("%.3f in", distanceInches))
	}
	if selectedUnit != "ft" {
		otherUnits = append(otherUnits, distanceFeetInches)
	}

	if selectedUnit != "ft" {
		unitLabel = Span(Class("encoder-unit-large"), g.Text(" "+selectedLabel))
	}
	otherUnitsLine = strings.Join(otherUnits, " | ")
	return selectedDisplay, unitLabel, otherUnitsLine
}

// deltaReadout formats signed delta (X' − X) in mm for the selected unit.
func deltaReadout(deltaMM float64, selectedUnit string) (text string, unitLabel g.Node) {
	switch selectedUnit {
	case "ft":
		return formatFeetInchesFraction(deltaMM), nil
	case "m":
		text = fmt.Sprintf("%+.3f", deltaMM/1000.0)
		unitLabel = Span(Class("encoder-unit-large"), g.Text(" m"))
	case "in":
		text = fmt.Sprintf("%+.3f", deltaMM/25.4)
		unitLabel = Span(Class("encoder-unit-large"), g.Text(" in"))
	default:
		text = fmt.Sprintf("%+.2f", deltaMM)
		unitLabel = Span(Class("encoder-unit-large"), g.Text(" mm"))
	}
	return text, unitLabel
}

func encoderDisplayXMerged(x, xp encoderValues, selectedUnit string) g.Node {
	mainText, mainUnitLabel, otherUnitsLine := distanceReadout(x.Distance, selectedUnit)
	deltaMM := xp.Distance - x.Distance
	isZero := math.Abs(deltaMM) < 1e-6
	deltaText, deltaUnitLabel := deltaReadout(deltaMM, selectedUnit)
	deltaCardClass := "encoder-delta encoder-delta-zero"
	if !isZero {
		deltaCardClass = "encoder-delta encoder-delta-nonzero"
	}
	return Div(
		Class("encoder-card"),
		Div(
			Class("encoder-label"),
			g.Text("X"),
		),
		Div(
			Class("encoder-distance"),
			g.Text(mainText),
			mainUnitLabel,
		),
		Div(
			Class("encoder-label"),
			g.Text("Δ (X′−X)"),
		),
		Div(
			Class(deltaCardClass),
			g.Text(deltaText),
			deltaUnitLabel,
		),
		Div(
			Class("encoder-details"),
			Span(
				Class("encoder-detail-item"),
				g.Textf("%d", x.Count),
				Span(Class("encoder-unit-small"), g.Text(" counts")),
				g.Text(" | "),
				g.Textf("%.1f", x.RPM),
				Span(Class("encoder-unit-small"), g.Text(" rpm")),
			),
			Span(
				Class("encoder-detail-item encoder-other-units"),
				g.Text(otherUnitsLine),
			),
			Span(
				Class("encoder-detail-item encoder-chip-ref"),
				g.Textf("%s · %s", chipRef("X"), chipRef("X'")),
			),
		),
	)
}

func encoderDisplay(label string, values encoderValues, selectedUnit string) g.Node {
	selectedDisplay, unitLabel, otherUnitsLine := distanceReadout(values.Distance, selectedUnit)
	return Div(
		Class("encoder-card"),
		Div(
			Class("encoder-label"),
			g.Text(label),
		),
		Div(
			Class("encoder-distance"),
			g.Text(selectedDisplay),
			unitLabel,
		),
		Div(
			Class("encoder-details"),
			Span(
				Class("encoder-detail-item"),
				g.Textf("%d", values.Count),
				Span(Class("encoder-unit-small"), g.Text(" counts")),
				g.Text(" | "),
				g.Textf("%.1f", values.RPM),
				Span(Class("encoder-unit-small"), g.Text(" rpm")),
			),
			Span(
				Class("encoder-detail-item encoder-other-units"),
				g.Text(otherUnitsLine),
			),
			Span(
				Class("encoder-detail-item encoder-chip-ref"),
				g.Text(chipRef(label)),
			),
		),
	)
}
