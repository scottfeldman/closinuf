package main

import (
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

var (
	counter       int   // total accumulated counts (signed for direction)
	lastState     uint8 // previous A/B state (2 bits)
	lastReadTime  time.Time
	lastReadCount int
	rpm           float64
	mu            sync.RWMutex // protects counter, rpm, lastReadTime, lastReadCount
)

type EncoderData struct {
	Count int     `json:"count"`
	RPM   float64 `json:"rpm"`
}

func main() {
	const (
		chipName = "gpiochip0"
		offsetA  = 17 // GPIO17
		offsetB  = 18 // GPIO18
	)

	// Quadrature table: +1 = CW, -1 = CCW, 0 = no/invalid change
	deltaTable := [16]int{
		0, -1, +1, 0,
		+1, 0, 0, -1,
		-1, 0, 0, +1,
		0, +1, -1, 0,
	}

	const countsPerRev = 2400.0 // 600 PPR × 4 (full quadrature)

	var lines *gpiocdev.Lines

	handler := func(evt gpiocdev.LineEvent) {
		values := make([]int, 2)
		if err := lines.Values(values); err != nil {
			return
		}

		currentState := uint8((values[0] << 1) | values[1])

		mu.Lock()
		if currentState != lastState {
			idx := int(lastState)<<2 | int(currentState)
			delta := deltaTable[idx]

			if delta != 0 {
				counter += delta
			}

			lastState = currentState
		}
		mu.Unlock()
	}

	var err error
	lines, err = gpiocdev.RequestLines(chipName,
		[]int{offsetA, offsetB},
		gpiocdev.AsInput,
		gpiocdev.WithPullUp,
		gpiocdev.WithEventHandler(handler),
		gpiocdev.WithBothEdges,
		gpiocdev.WithConsumer("rotary-encoder"),
	)
	if err != nil {
		// If GPIO fails, continue anyway (for development/testing)
		// In production, you might want to exit or handle differently
		// fmt.Fprintf(os.Stderr, "failed to request lines: %v\n", err)
		// os.Exit(1)
	} else {
		defer lines.Close()
	}

	// Initialize timing
	mu.Lock()
	lastReadTime = time.Now()
	lastReadCount = counter
	mu.Unlock()

	// Periodic RPM calculation goroutine
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond) // Update RPM every 100ms
		defer ticker.Stop()

		for range ticker.C {
			mu.Lock()
			now := time.Now()
			currentCount := counter

			deltaCounts := currentCount - lastReadCount
			elapsedSec := now.Sub(lastReadTime).Seconds()

			if elapsedSec > 0 {
				rpm = (float64(deltaCounts) / countsPerRev) * (60.0 / elapsedSec)
			}

			lastReadCount = currentCount
			lastReadTime = now
			mu.Unlock()
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
		mu.RLock()
		count := counter
		rpmValue := rpm
		mu.RUnlock()

		c.Type("html")
		return Page(count, rpmValue).Render(c)
	})

	// API endpoint to get encoder data (JSON)
	app.Get("/api/encoder", func(c *fiber.Ctx) error {
		mu.RLock()
		data := EncoderData{
			Count: counter,
			RPM:   rpm,
		}
		mu.RUnlock()

		return c.JSON(data)
	})

	// HTMX endpoint that returns HTML fragment
	app.Get("/api/encoder/htmx", func(c *fiber.Ctx) error {
		mu.RLock()
		count := counter
		rpmValue := rpm
		mu.RUnlock()

		c.Type("html")
		return EncoderFragment(count, rpmValue).Render(c)
	})

	// Start server in goroutine
	go func() {
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

func Page(count int, rpmValue float64) g.Node {
	return HTML(
		Head(
			Meta(Charset("utf-8")),
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			TitleEl(g.Text("Rotary Encoder Monitor")),
			Script(Src("https://unpkg.com/htmx.org@2.0.3/dist/htmx.min.js")),
			StyleEl(g.Raw(`
				body {
					font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
					max-width: 800px;
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
				.metric {
					margin: 1.5rem 0;
					padding: 1.5rem;
					background: #f8f9fa;
					border-radius: 8px;
					border-left: 4px solid #007bff;
				}
				.metric-label {
					font-size: 0.9rem;
					color: #666;
					text-transform: uppercase;
					letter-spacing: 0.5px;
					margin-bottom: 0.5rem;
				}
				.metric-value {
					font-size: 2.5rem;
					font-weight: bold;
					color: #007bff;
					font-variant-numeric: tabular-nums;
				}
				.metric-unit {
					font-size: 1.2rem;
					color: #999;
					margin-left: 0.5rem;
				}
			`)),
		),
		Body(
			Div(Class("container"),
				H1(g.Text("Rotary Encoder Monitor")),
				EncoderFragment(count, rpmValue),
			),
		),
	)
}

func EncoderFragment(count int, rpmValue float64) g.Node {
	return Div(
		hx.Get("/api/encoder/htmx"),
		hx.Trigger("every 200ms"),
		hx.Swap("outerHTML"),
		hx.Target("this"),
		ID("encoder-data"),
		Div(Class("metric"),
			Div(Class("metric-label"), g.Text("Count")),
			Div(Class("metric-value"),
				g.Textf("%d", count),
				Span(Class("metric-unit"), g.Text("counts")),
			),
		),
		Div(Class("metric"),
			Div(Class("metric-label"), g.Text("RPM")),
			Div(Class("metric-value"),
				g.Textf("%.1f", rpmValue),
				Span(Class("metric-unit"), g.Text("rpm")),
			),
		),
	)
}
