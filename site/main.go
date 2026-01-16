package main

import (
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

var (
	encoders [3]*Encoder // X=0, Y=1, Z=2
)

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

type DataPoint struct {
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	Z         float64   `json:"z"`
	Timestamp time.Time `json:"timestamp"`
}

var (
	history    []DataPoint
	historyMu  sync.RWMutex
	maxHistory = 500 // maximum number of data points to keep
)

func main() {
	const (
		chipName = "gpiochip0"
		// GPIO pins for each encoder: [A, B]
		xOffsetA = 17 // GPIO17
		xOffsetB = 18 // GPIO18
		yOffsetA = 19 // GPIO19
		yOffsetB = 20 // GPIO20
		zOffsetA = 21 // GPIO21
		zOffsetB = 22 // GPIO22
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

		func(enc *Encoder, offsetA, offsetB int, label string) {
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
				[]int{offsetA, offsetB},
				gpiocdev.AsInput,
				gpiocdev.WithPullUp,
				gpiocdev.WithEventHandler(handler),
				gpiocdev.WithBothEdges,
				gpiocdev.WithConsumer("rotary-encoder-"+label),
			)
			if err != nil {
				// If GPIO fails, continue anyway (for development/testing)
				// In production, you might want to exit or handle differently
			}
		}(enc, cfg.offsetA, cfg.offsetB, cfg.label)
	}

	// Periodic RPM calculation goroutine for all encoders
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // Update RPM every 100ms
		defer ticker.Stop()

		for range ticker.C {
			var distances [3]float64
			for i, enc := range encoders {
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

				// Calculate distance for this encoder
				distances[i] = (float64(currentCount) / countsPerRev) * wheelCircumference

				enc.lastReadCount = currentCount
				enc.lastReadTime = now
				enc.mu.Unlock()
			}

			// Add data point to history
			historyMu.Lock()
			history = append(history, DataPoint{
				X:         distances[0],
				Y:         distances[1],
				Z:         distances[2],
				Timestamp: time.Now(),
			})
			// Keep only the last maxHistory points
			if len(history) > maxHistory {
				history = history[len(history)-maxHistory:]
			}
			historyMu.Unlock()
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

	// Zero endpoint to reset all encoder counts
	app.Post("/api/encoder/zero", func(c *fiber.Ctx) error {
		for _, enc := range encoders {
			if enc != nil {
				enc.mu.Lock()
				enc.counter = 0
				enc.mu.Unlock()
			}
		}
		// Clear history when zeroing
		historyMu.Lock()
		history = nil
		historyMu.Unlock()
		return c.SendStatus(200)
	})

	// API endpoint to get historical data for 3D plot
	app.Get("/api/encoder/history", func(c *fiber.Ctx) error {
		historyMu.RLock()
		points := make([]DataPoint, len(history))
		copy(points, history)
		historyMu.RUnlock()
		return c.JSON(points)
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
			Script(Src("https://cdn.plot.ly/plotly-2.27.0.min.js")),
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
					background: #dc3545;
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
					background: #c82333;
				}
				.zero-button:active {
					background: #bd2130;
				}
				.button-container {
					text-align: center;
					margin-top: 2rem;
				}
				.plot-container {
					margin-top: 2rem;
					background: white;
					border-radius: 8px;
					padding: 1.5rem;
					box-shadow: 0 1px 3px rgba(0,0,0,0.1);
				}
				#plot3d {
					width: 100%;
					height: 600px;
				}
			`)),
			Script(g.Raw(`
				let plotInitialized = false;
				
				function updatePlot() {
					const plotDiv = document.getElementById('plot3d');
					if (!plotDiv) {
						console.error('plot3d div not found');
						return;
					}
					
					if (typeof Plotly === 'undefined') {
						console.log('Waiting for Plotly to load...');
						setTimeout(updatePlot, 100);
						return;
					}
					
					fetch('/api/encoder/history')
						.then(response => response.json())
						.then(data => {
							if (data.length === 0) {
								// Initialize empty plot if no data yet
								if (!plotInitialized) {
									const emptyTrace = {
										x: [0],
										y: [0],
										z: [0],
										mode: 'markers',
										type: 'scatter3d',
										marker: { size: 1, color: 'rgba(0,0,0,0)' },
										name: 'Path'
									};
									const layout = {
										title: '3D Encoder Path',
										scene: {
											xaxis: { title: 'X (mm)' },
											yaxis: { title: 'Y (mm)' },
											zaxis: { title: 'Z (mm)' },
											camera: { eye: { x: 1.5, y: 1.5, z: 1.5 } },
											dragmode: 'orbit',
											hovermode: 'closest'
										},
										margin: { l: 0, r: 0, t: 40, b: 0 },
										height: 600
									};
									const config = {
										displayModeBar: true,
										displaylogo: false,
										modeBarButtonsToRemove: ['lasso2d', 'select2d'],
										responsive: true
									};
									Plotly.newPlot('plot3d', [emptyTrace], layout, config);
									plotInitialized = true;
									console.log('Plot initialized (empty)');
								}
								return;
							}
							
							const x = data.map(p => p.x);
							const y = data.map(p => p.y);
							const z = data.map(p => p.z);
							
							const trace = {
								x: x,
								y: y,
								z: z,
								mode: 'lines+markers',
								type: 'scatter3d',
								marker: {
									size: 4,
									color: z,
									colorscale: 'Viridis',
									showscale: true,
									colorbar: {
										title: 'Z (mm)'
									}
								},
								line: {
									color: 'rgba(0, 123, 255, 0.6)',
									width: 2
								},
								name: 'Path'
							};
							
							const layout = {
								title: '3D Encoder Path',
								scene: {
									xaxis: { title: 'X (mm)' },
									yaxis: { title: 'Y (mm)' },
									zaxis: { title: 'Z (mm)' },
									camera: {
										eye: { x: 1.5, y: 1.5, z: 1.5 }
									},
									dragmode: 'orbit',
									hovermode: 'closest'
								},
								margin: { l: 0, r: 0, t: 40, b: 0 },
								height: 600
							};
							
							const config = {
								displayModeBar: true,
								displaylogo: false,
								modeBarButtonsToRemove: ['lasso2d', 'select2d'],
								responsive: true
							};
							
							if (!plotInitialized) {
								const config = {
									displayModeBar: true,
									displaylogo: false,
									modeBarButtonsToRemove: ['lasso2d', 'select2d'],
									responsive: true
								};
								Plotly.newPlot('plot3d', [trace], layout, config);
								plotInitialized = true;
								console.log('Plot initialized with data');
							} else {
								Plotly.update('plot3d', { x: [x], y: [y], z: [z] }, {}, [0]);
							}
						})
						.catch(err => console.error('Error updating plot:', err));
				}
				
				// Wait for DOM and Plotly to load, then start updating
				function initPlot() {
					if (document.readyState === 'loading') {
						document.addEventListener('DOMContentLoaded', initPlot);
						return;
					}
					
					if (typeof Plotly !== 'undefined') {
						console.log('Initializing plot...');
						updatePlot();
						setInterval(updatePlot, 200);
					} else {
						setTimeout(initPlot, 100);
					}
				}
				initPlot();
			`)),
		),
		Body(
			Div(Class("container"),
				EncoderFragment(data),
			),
			Div(Class("container"),
				Div(Class("plot-container"),
					H2(g.Text("3D Encoder Path Visualization")),
					P(Class("plot-hint"), g.Text("Click and drag to rotate • Scroll to zoom • Double-click to reset view")),
					Div(ID("plot3d")),
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
		Div(Class("button-container"),
			Button(
				Class("zero-button"),
				hx.Post("/api/encoder/zero"),
				hx.Trigger("click"),
				hx.Swap("none"),
				g.Text("Zero All Counts"),
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
