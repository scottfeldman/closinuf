package main

import (
	"fmt"
	"math"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	g "github.com/maragudk/gomponents"
	hx "github.com/maragudk/gomponents-htmx"
	. "github.com/maragudk/gomponents/html"
	"github.com/warthog618/go-gpiocdev"
)

type Encoder struct {
	counter       int   // total accumulated counts (signed for direction)
	lastState     uint8 // previous A/B state (2 bits)
	lastReadTime  time.Time
	lastReadCount int
	rpm           float64
	label         string
	lines         *gpiocdev.Lines // GPIO lines for this encoder
	mu            sync.RWMutex    // protects this encoder's state
}

const (
	countsPerRev       = 2400.0                  // 600 PPR × 4 (full quadrature)
	wheelDiameter      = 50.0                    // wheel diameter in mm
	wheelCircumference = math.Pi * wheelDiameter // ≈ 157.08mm
)

type EncoderData struct {
	X EncoderValues `json:"x"`
	Y EncoderValues `json:"y"`
	Z EncoderValues `json:"z"`
}

type EncoderValues struct {
	Count    int     `json:"count"`
	RPM      float64 `json:"rpm"`
	Distance float64 `json:"distance"` // distance in mm from zero
	Label    string  `json:"label"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

var (
	encoders [3]*Encoder // X=0, Y=1, Z=2
	points   []Point     // accumulated points
	pointsMu sync.RWMutex
)

func main() {
	const (
		chipName = "gpiochip0"
		// GPIO pins for each encoder: [A, B]
		xOffsetA       = 17 // GPIO17
		xOffsetB       = 18 // GPIO18
		yOffsetA       = 19 // GPIO19
		yOffsetB       = 20 // GPIO20
		zOffsetA       = 21 // GPIO21
		zOffsetB       = 22 // GPIO22
		pointBtnOffset = 23 // GPIO23 - physical button for adding points
	)

	// Quadrature table: +1 = CW, -1 = CCW, 0 = no/invalid change
	deltaTable := [16]int{
		0, -1, +1, 0,
		+1, 0, 0, -1,
		-1, 0, 0, +1,
		0, +1, -1, 0,
	}

	// Initialize encoders
	encoders[0] = &Encoder{label: "X"} // X encoder
	encoders[1] = &Encoder{label: "Y"} // Y encoder
	encoders[2] = &Encoder{label: "Z"} // Z encoder

	// Initialize GPIO for each encoder
	encoderConfigs := []struct {
		label   string
		offsetA int
		offsetB int
		index   int
	}{
		{"X", xOffsetA, xOffsetB, 0},
		{"Y", yOffsetA, yOffsetB, 1},
		{"Z", zOffsetA, zOffsetB, 2},
	}

	for _, cfg := range encoderConfigs {
		enc := encoders[cfg.index]
		enc.mu.Lock()
		enc.lastReadTime = time.Now()
		enc.lastReadCount = enc.counter
		enc.mu.Unlock()

		handler := func(evt gpiocdev.LineEvent) {
			values := make([]int, 2)
			if err := enc.lines.Values(values); err != nil {
				return
			}

			currentState := uint8((values[0] << 1) | values[1])

			enc.mu.Lock()
			if currentState != enc.lastState {
				idx := int(enc.lastState)<<2 | int(currentState)
				delta := deltaTable[idx]

				if delta != 0 {
					enc.counter += delta
				}

				enc.lastState = currentState
			}
			enc.mu.Unlock()
		}

		var err error
		enc.lines, err = gpiocdev.RequestLines(chipName,
			[]int{cfg.offsetA, cfg.offsetB},
			gpiocdev.AsInput,
			gpiocdev.WithPullUp,
			gpiocdev.WithEventHandler(handler),
			gpiocdev.WithBothEdges,
			gpiocdev.WithConsumer("rotary-encoder-"+cfg.label),
		)
		if err != nil {
			// If GPIO fails, continue anyway (for development/testing)
			// In production, you might want to exit or handle differently
		}
	}

	// Setup GPIO for physical Point button
	var lastBtnPressTime time.Time
	const btnDebounceMs = 50 // 50ms debounce even for bounceless button

	pointBtnHandler := func(evt gpiocdev.LineEvent) {
		// Only process falling edge (button press, assuming pull-up)
		if evt.Type != gpiocdev.LineEventFallingEdge {
			return
		}

		// Simple debounce check
		now := time.Now()
		if now.Sub(lastBtnPressTime) < time.Millisecond*btnDebounceMs {
			return
		}
		lastBtnPressTime = now

		// Add point with current encoder coordinates
		data := getEncoderData()
		pointsMu.Lock()
		points = append(points, Point{
			X: data.X.Distance,
			Y: data.Y.Distance,
			Z: data.Z.Distance,
		})
		pointsMu.Unlock()
	}

	_, err := gpiocdev.RequestLines(chipName,
		[]int{pointBtnOffset},
		gpiocdev.AsInput,
		gpiocdev.WithPullUp,
		gpiocdev.WithEventHandler(pointBtnHandler),
		gpiocdev.WithBothEdges,
		gpiocdev.WithConsumer("point-button"),
	)
	if err != nil {
		os.Stderr.WriteString("Warning: Failed to initialize point button GPIO (GPIO" + fmt.Sprintf("%d", pointBtnOffset) + "): " + err.Error() + "\n")
		os.Stderr.WriteString("Physical button will not be available. Web interface Point button will still work.\n")
	}

	// Periodic RPM calculation goroutine for all encoders
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // Update RPM every 100ms
		defer ticker.Stop()

		for range ticker.C {
			for _, enc := range encoders {
				if enc == nil {
					continue
				}
				enc.mu.Lock()
				now := time.Now()
				currentCount := enc.counter

				deltaCounts := currentCount - enc.lastReadCount
				elapsedSec := now.Sub(enc.lastReadTime).Seconds()

				if elapsedSec > 0 {
					enc.rpm = (float64(deltaCounts) / countsPerRev) * (60.0 / elapsedSec)
				}

				enc.lastReadCount = currentCount
				enc.lastReadTime = now
				enc.mu.Unlock()
			}
		}
	}()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	// CORS middleware
	app.Use(cors.New())

	// Serve static HTML page
	app.Get("/", func(c *fiber.Ctx) error {
		data := getEncoderData()
		c.Type("html")
		return Page(data).Render(c)
	})

	// API endpoint to get encoder data (JSON)
	app.Get("/api/encoder", func(c *fiber.Ctx) error {
		data := getEncoderData()
		return c.JSON(data)
	})

	// HTMX endpoint that returns HTML fragment
	app.Get("/api/encoder/htmx", func(c *fiber.Ctx) error {
		data := getEncoderData()
		c.Type("html")
		return EncoderFragment(data).Render(c)
	})

	// Zero endpoint to reset all encoder counts and clear points
	app.Post("/api/encoder/zero", func(c *fiber.Ctx) error {
		for _, enc := range encoders {
			if enc != nil {
				enc.mu.Lock()
				enc.counter = 0
				enc.mu.Unlock()
			}
		}
		pointsMu.Lock()
		points = []Point{}
		pointsMu.Unlock()
		return c.SendStatus(200)
	})

	// Add point endpoint - saves current coordinates
	app.Post("/api/points/add", func(c *fiber.Ctx) error {
		data := getEncoderData()
		pointsMu.Lock()
		points = append(points, Point{
			X: data.X.Distance,
			Y: data.Y.Distance,
			Z: data.Z.Distance,
		})
		pointsMu.Unlock()
		return c.SendStatus(200)
	})

	// Get point count endpoint - returns HTML fragment or JSON
	app.Get("/api/points/count", func(c *fiber.Ctx) error {
		pointsMu.RLock()
		count := len(points)
		pointsMu.RUnlock()

		// Check if client wants JSON (for JavaScript fetch)
		if c.Get("Accept") == "application/json" || c.Query("json") == "true" {
			return c.JSON(fiber.Map{"count": count})
		}

		c.Type("html")
		return g.Text(fmt.Sprintf("Points: %d", count)).Render(c)
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

		pointsMu.RLock()
		count := len(points)
		pointsMu.RUnlock()

		// If no points, return error message using hx-swap-oob
		if count == 0 {
			c.Type("html")
			// Return empty response for main swap, error message via oob
			return g.Raw(`<div id="save-error" hx-swap-oob="true" class="save-error">No points to save. Please collect some points first.</div>`).Render(c)
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

		pointsMu.Lock()
		if len(points) == 0 {
			pointsMu.Unlock()
			return c.Status(400).JSON(fiber.Map{"error": "No points to save"})
		}

		// Create ASC content - space-separated X Y Z format for FreeCAD
		var ascData string
		for _, p := range points {
			ascData += fmt.Sprintf("%.6f %.6f %.6f\n", p.X, p.Y, p.Z)
		}

		// Clear points after saving
		points = []Point{}
		pointsMu.Unlock()

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

func getEncoderData() EncoderData {
	var data EncoderData
	for i, enc := range encoders {
		if enc == nil {
			continue
		}
		enc.mu.RLock()
		count := enc.counter
		rpm := enc.rpm
		label := enc.label
		enc.mu.RUnlock()

		// Calculate distance: (count / countsPerRev) × circumference
		distance := (float64(count) / countsPerRev) * wheelCircumference

		values := EncoderValues{
			Count:    count,
			RPM:      rpm,
			Distance: distance,
			Label:    label,
		}

		switch i {
		case 0:
			data.X = values
		case 1:
			data.Y = values
		case 2:
			data.Z = values
		}
	}
	return data
}

func Page(data EncoderData) g.Node {
	return HTML(
		Head(
			Meta(Charset("utf-8")),
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			TitleEl(g.Text("Rotary Encoder Monitor")),
			Script(Src("https://unpkg.com/htmx.org@2.0.3/dist/htmx.min.js")),
			Script(g.Raw(`
				function checkPointsCount() {
					return fetch('/api/points/count?json=true', {
						headers: { 'Accept': 'application/json' }
					})
						.then(r => r.json())
						.then(data => data.count);
				}
				document.addEventListener('DOMContentLoaded', function() {
					const saveLink = document.getElementById('save-link');
					const filenameInput = document.getElementById('filename-input');
					if (saveLink && filenameInput) {
						saveLink.addEventListener('click', async function(e) {
							e.preventDefault();
							// Check if there are any points before saving
							const count = await checkPointsCount();
							if (count === 0) {
								alert('No points to save. Please collect some points first.');
								return;
							}
							let filename = filenameInput.value.trim() || 'points.asc';
							// Add .asc extension if not present
							if (filename.length < 4 || filename.slice(-4) !== '.asc') {
								filename += '.asc';
							}
							window.location.href = '/api/points/save?filename=' + encodeURIComponent(filename);
							setTimeout(function() {
								htmx.trigger('#points-count', 'htmx:trigger');
							}, 500);
						});
					}
				});
			`)),
			StyleEl(g.Raw(`
				body {
					font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
					max-width: 1000px;
					margin: 0 auto;
					padding: 2rem;
					background: #f5f5f5;
				}
				.container {
					background: white;
					border-radius: 12px;
					padding: 2rem;
					box-shadow: 0 2px 8px rgba(0,0,0,0.1);
				}
				h1 {
					margin-top: 0;
					color: #333;
				}
				.encoder-table {
					width: 100%;
					border-collapse: collapse;
					margin-bottom: 2rem;
					background: white;
					border-radius: 8px;
					overflow: hidden;
					box-shadow: 0 1px 3px rgba(0,0,0,0.1);
					table-layout: fixed;
				}
				.encoder-table thead {
					background: #007bff;
					color: white;
				}
				.encoder-table th {
					padding: 1.25rem 1.5rem;
					text-align: left;
					font-weight: 600;
					text-transform: uppercase;
					font-size: 0.85rem;
					letter-spacing: 0.5px;
				}
				.encoder-table th:nth-child(1) {
					width: 15%;
				}
				.encoder-table th:nth-child(2) {
					width: 25%;
				}
				.encoder-table th:nth-child(3) {
					width: 25%;
				}
				.encoder-table th:nth-child(4) {
					width: 35%;
				}
				.encoder-table td {
					padding: 1rem 1.5rem;
					border-bottom: 1px solid #e9ecef;
					font-variant-numeric: tabular-nums;
					white-space: nowrap;
					overflow: hidden;
					text-overflow: ellipsis;
				}
				.encoder-table tbody tr:last-child td {
					border-bottom: none;
				}
				.encoder-table tbody tr:hover {
					background: #f8f9fa;
				}
				.encoder-label {
					font-weight: bold;
					color: #007bff;
					font-size: 1.1rem;
				}
				.encoder-value {
					font-size: 1.1rem;
					color: #333;
					display: inline-block;
					min-width: 80px;
				}
				.encoder-unit {
					font-size: 0.9rem;
					color: #999;
					margin-left: 0.25rem;
				}
				.zero-button {
					background: #007bff;
					color: white;
					border: none;
					padding: 0.75rem 1.5rem;
					border-radius: 6px;
					font-size: 1rem;
					font-weight: 600;
					cursor: pointer;
					transition: background 0.2s;
				}
				.zero-button:hover {
					background: #0056b3;
				}
				.zero-button:active {
					background: #004085;
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
				.point-button {
					background: #28a745;
					color: white;
					border: none;
					padding: 0.75rem 1.5rem;
					border-radius: 6px;
					font-size: 1rem;
					font-weight: 600;
					cursor: pointer;
					transition: background 0.2s;
				}
				.point-button:hover {
					background: #218838;
				}
				.point-button:active {
					background: #1e7e34;
				}
				.save-button {
					background: #ffc107;
					color: #212529;
					border: none;
					padding: 0.75rem 1.5rem;
					border-radius: 6px;
					font-size: 1rem;
					font-weight: 600;
					cursor: pointer;
					transition: background 0.2s;
				}
				.save-button:hover {
					background: #e0a800;
				}
				.save-button:active {
					background: #d39e00;
				}
				.points-count {
					font-size: 1rem;
					color: #666;
					font-weight: 500;
					padding: 0.75rem 1rem;
					background: #f8f9fa;
					border-radius: 6px;
				}
				.filename-input {
					padding: 0.75rem 1rem;
					border: 2px solid #dee2e6;
					border-radius: 6px;
					font-size: 1rem;
					width: 150px;
					transition: border-color 0.2s;
				}
				.filename-input:focus {
					outline: none;
					border-color: #007bff;
				}
				.save-group {
					display: flex;
					gap: 0.5rem;
					align-items: center;
					position: relative;
				}
				.save-error {
					position: absolute;
					top: 100%;
					left: 50%;
					transform: translateX(-50%);
					margin-top: 0.5rem;
					color: #dc3545;
					background: #f8d7da;
					border: 1px solid #f5c6cb;
					padding: 0.75rem 1rem;
					border-radius: 6px;
					text-align: center;
					white-space: nowrap;
					z-index: 10;
					box-shadow: 0 2px 8px rgba(0,0,0,0.15);
					animation: fadeOut 0.5s ease-out 5s forwards;
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
				EncoderFragment(data),
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
						Div(
							ID("save-error"),
							g.Attr("style", "display: none;"),
						),
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

func EncoderFragment(data EncoderData) g.Node {
	return Div(
		hx.Get("/api/encoder/htmx"),
		hx.Trigger("every 200ms"),
		hx.Swap("outerHTML"),
		hx.Target("this"),
		ID("encoder-data"),
		Table(Class("encoder-table"),
			THead(
				Tr(
					Th(g.Text("Encoder")),
					Th(g.Text("Count")),
					Th(g.Text("RPM")),
					Th(g.Text("Distance")),
				),
			),
			TBody(
				encoderRow("X", data.X),
				encoderRow("Y", data.Y),
				encoderRow("Z", data.Z),
			),
		),
	)
}

func encoderRow(label string, values EncoderValues) g.Node {
	return Tr(
		Td(
			Span(Class("encoder-label"), g.Text(label)),
		),
		Td(
			Span(Class("encoder-value"),
				g.Textf("%d", values.Count),
				Span(Class("encoder-unit"), g.Text("counts")),
			),
		),
		Td(
			Span(Class("encoder-value"),
				g.Textf("%.1f", values.RPM),
				Span(Class("encoder-unit"), g.Text("rpm")),
			),
		),
		Td(
			Span(Class("encoder-value"),
				g.Textf("%.2f", values.Distance),
				Span(Class("encoder-unit"), g.Text("mm")),
			),
		),
	)
}
