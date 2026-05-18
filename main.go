package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	g "maragu.dev/gomponents"
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
