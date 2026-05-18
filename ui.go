package main

import (
	"fmt"
	"math"
	"strings"

	g "maragu.dev/gomponents"
	hx "maragu.dev/gomponents-htmx"
	. "maragu.dev/gomponents/html"
)

const appTitle = "closinuf"

func page(data encoderData, unit string) g.Node {
	return HTML(
		Head(
			Meta(Charset("utf-8")),
			Meta(Name("viewport"), Content("width=device-width, initial-scale=1")),
			TitleEl(g.Text(appTitle)),
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
				H1(g.Text(appTitle)),
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
		),
	)
}
