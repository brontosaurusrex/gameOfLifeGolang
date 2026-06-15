package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"syscall/js"
	"time"
)

const (
	cellWidth       = 10
	cellHeight      = 10
	frameDelayMs    = 50
	defaultMaxGen   = 1000
	maxStoredFrames = 5000
)

var (
	width, height int
	grid, next    []int
	paused        = false
	genCount      = 0
	maxGen        = defaultMaxGen
	seed          int64
	ctx           js.Value
	doc           js.Value
	showOverlays  = false
	showPopup     = false
	statusLabel   js.Value
	instruction   js.Value
	stateHistory  map[uint64]int

	// Frame storage - simple list of grid copies
	frameHistory [][]int
	currentFrame int
)

func main() {
	c := make(chan struct{}, 0)

	// Parse URL parameters
	seed = time.Now().UnixNano()
	urlParams := js.Global().Get("window").Get("location").Get("search").String()

	if len(urlParams) > 0 {
		if idx := findParam(urlParams, "seed"); idx >= 0 {
			if parsedSeed, err := strconv.ParseInt(extractParamValue(urlParams, idx), 10, 64); err == nil {
				seed = parsedSeed
			}
		}
		if idx := findParam(urlParams, "maxgen"); idx >= 0 {
			if parsedMaxGen, err := strconv.Atoi(extractParamValue(urlParams, idx)); err == nil && parsedMaxGen > 0 {
				maxGen = parsedMaxGen
			}
		}
		if idx := findParam(urlParams, "showpopup"); idx >= 0 {
			showPopup = extractParamValue(urlParams, idx) == "true"
		}
	}

	// Setup DOM
	doc = js.Global().Get("document")
	body := doc.Get("body")
	body.Get("style").Set("margin", "0")
	body.Get("style").Set("padding", "0")
	body.Get("style").Set("overflow", "hidden")
	body.Get("style").Set("background", "black")

	// Get window size
	width = js.Global().Get("innerWidth").Int()/cellWidth + 1
	height = js.Global().Get("innerHeight").Int()/cellHeight + 1

	canvas := doc.Call("createElement", "canvas")
	canvas.Set("width", width*cellWidth)
	canvas.Set("height", height*cellHeight)
	canvas.Get("style").Set("display", "block")
	canvas.Get("style").Set("background", "black")
	canvas.Get("style").Set("border", "none")
	doc.Get("body").Call("appendChild", canvas)

	// UI Elements
	statusLabel = doc.Call("createElement", "div")
	statusLabel.Get("style").Set("position", "fixed")
	statusLabel.Get("style").Set("bottom", "10px")
	statusLabel.Get("style").Set("right", "10px")
	statusLabel.Get("style").Set("color", "rgba(255,255,255,0.3)")
	statusLabel.Get("style").Set("font-family", "monospace")
	statusLabel.Get("style").Set("font-size", "10px")
	statusLabel.Get("style").Set("background", "rgba(0,0,0,0.9)")
	statusLabel.Get("style").Set("padding", "5px")
	statusLabel.Get("style").Set("border-radius", "3px")
	statusLabel.Get("style").Set("display", "none")
	statusLabel.Set("textContent", fmt.Sprintf("Gen: 0 | Seed: %d | ▶", seed))
	doc.Get("body").Call("appendChild", statusLabel)

	instruction = doc.Call("createElement", "div")
	instruction.Get("style").Set("position", "fixed")
	instruction.Get("style").Set("bottom", "10px")
	instruction.Get("style").Set("left", "10px")
	instruction.Get("style").Set("color", "rgba(255,255,255,0.3)")
	instruction.Get("style").Set("font-family", "monospace")
	instruction.Get("style").Set("font-size", "10px")
	instruction.Get("style").Set("background", "rgba(0,0,0,0.9)")
	instruction.Get("style").Set("padding", "5px")
	instruction.Get("style").Set("border-radius", "3px")
	instruction.Get("style").Set("display", "none")
	instruction.Set("textContent", "H: UI | SPACE: Pause | , / . : Step frame")
	doc.Get("body").Call("appendChild", instruction)

	ctx = canvas.Call("getContext", "2d")

	// Keyboard handlers
	js.Global().Call("addEventListener", "keydown", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		event := args[0]
		key := event.Get("key").String()

		if key == " " || key == "Space" {
			paused = !paused
			event.Call("preventDefault")
			// When unpausing, clear frame history to save memory
			if !paused {
				frameHistory = nil
				currentFrame = 0
			} else {
				// When pausing, ensure current frame is at genCount
				currentFrame = genCount
			}
			updateStatus()
			return nil
		}

		if key == "," && paused {
			if currentFrame > 0 {
				currentFrame--
				loadFrame(currentFrame)
				event.Call("preventDefault")
			}
			return nil
		}

		if key == "." && paused {
			if currentFrame < genCount {
				currentFrame++
				loadFrame(currentFrame)
				event.Call("preventDefault")
			}
			return nil
		}

		if key == "h" || key == "H" {
			showOverlays = !showOverlays
			toggleOverlays()
			event.Call("preventDefault")
			return nil
		}

		return nil
	}))

	// Resize handler
	js.Global().Call("addEventListener", "resize", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
		newWidth := js.Global().Get("innerWidth").Int()/cellWidth + 1
		newHeight := js.Global().Get("innerHeight").Int()/cellHeight + 1
		if newWidth != width || newHeight != height {
			width = newWidth
			height = newHeight
			canvas.Set("width", width*cellWidth)
			canvas.Set("height", height*cellHeight)
			initGrid()
		}
		return nil
	}))

	// Initialize
	initGrid()

	// Animation loop
	var lastUpdate time.Time
	var animate func()
	animate = func() {
		if !paused {
			now := time.Now()
			if now.Sub(lastUpdate) >= time.Duration(frameDelayMs)*time.Millisecond {
				updateSimulation()
				drawGrid()
				lastUpdate = now
			}
		}
		js.Global().Call("requestAnimationFrame", js.FuncOf(func(this js.Value, args []js.Value) interface{} {
			animate()
			return nil
		}))
	}
	animate()

	<-c
}

func storeFrame() {
	// Store a copy of current grid
	frameCopy := make([]int, len(grid))
	copy(frameCopy, grid)

	// Ensure slice length
	for len(frameHistory) <= genCount {
		frameHistory = append(frameHistory, nil)
	}
	frameHistory[genCount] = frameCopy

	// Limit size
	if len(frameHistory) > maxStoredFrames {
		frameHistory = frameHistory[len(frameHistory)-maxStoredFrames:]
	}
}

func loadFrame(frameNum int) {
	if frameNum < 0 || frameNum >= len(frameHistory) || frameHistory[frameNum] == nil {
		return
	}
	// Copy into grid
	copy(grid, frameHistory[frameNum])
	drawGrid()
	updateStatus()
}

func initGrid() {
	rand.Seed(seed)
	size := width * height
	grid = make([]int, size)
	next = make([]int, size)

	for i := 0; i < size; i++ {
		grid[i] = rand.Intn(2)
	}

	stateHistory = make(map[uint64]int)
	frameHistory = nil
	genCount = 0
	currentFrame = 0
	// Store initial frame
	storeFrame()
	drawGrid()
	updateStatus()
}

func updateSimulation() {
	aliveAny := false

	for y := 0; y < height; y++ {
		ym1 := (y - 1 + height) % height
		yp1 := (y + 1) % height

		for x := 0; x < width; x++ {
			xm1 := (x - 1 + width) % width
			xp1 := (x + 1) % width

			count := grid[xm1+ym1*width] +
				grid[x+ym1*width] +
				grid[xp1+ym1*width] +
				grid[xm1+y*width] +
				grid[xp1+y*width] +
				grid[xm1+yp1*width] +
				grid[x+yp1*width] +
				grid[xp1+yp1*width]

			idx := x + y*width
			if grid[idx] == 1 {
				if count == 2 || count == 3 {
					next[idx] = 1
					aliveAny = true
				} else {
					next[idx] = 0
				}
			} else {
				if count == 3 {
					next[idx] = 1
					aliveAny = true
				} else {
					next[idx] = 0
				}
			}
		}
	}

	// Swap
	grid, next = next, grid

	if !aliveAny {
		showMessage(fmt.Sprintf("💀 Extinction at generation %d", genCount+1))
		return
	}

	// Check for cycles
	hash := hashState(grid)
	if firstGen, exists := stateHistory[hash]; exists {
		period := genCount - firstGen
		if period == 1 {
			showMessage(fmt.Sprintf("🎯 Still Life at generation %d", genCount+1))
		} else {
			showMessage(fmt.Sprintf("🔄 Oscillator (period %d) at generation %d", period, genCount+1))
		}
		return
	}

	stateHistory[hash] = genCount
	genCount++

	if genCount >= maxGen {
		showMessage(fmt.Sprintf("⏰ Max generations (%d) reached", maxGen))
		return
	}

	// Store every frame (always, so we can step back)
	storeFrame()
	// When not paused, currentFrame follows genCount
	if !paused {
		currentFrame = genCount
	}
	updateStatus()
}

func hashState(grid []int) uint64 {
	var hash uint64 = 14695981039346656037
	for i := 0; i < len(grid); i++ {
		hash ^= uint64(grid[i])
		hash *= 1099511628211
	}
	return hash
}

func drawGrid() {
	ctx.Set("fillStyle", "black")
	ctx.Call("fillRect", 0, 0, width*cellWidth, height*cellHeight)

	ctx.Set("fillStyle", "white")
	for i := 0; i < len(grid); i++ {
		if grid[i] == 1 {
			x := (i % width) * cellWidth
			y := (i / width) * cellHeight
			ctx.Call("fillRect", x, y, cellWidth-1, cellHeight-1)
		}
	}
}

func updateStatus() {
	if showOverlays && !statusLabel.IsNull() {
		status := "▶"
		if paused {
			status = fmt.Sprintf("⏸ [%d/%d]", currentFrame, genCount)
		}
		text := fmt.Sprintf("Gen: %d | Seed: %d | %s", genCount, seed, status)
		statusLabel.Set("textContent", text)
	}
}

func toggleOverlays() {
	if showOverlays {
		statusLabel.Get("style").Set("display", "block")
		instruction.Get("style").Set("display", "block")
		updateStatus()
	} else {
		statusLabel.Get("style").Set("display", "none")
		instruction.Get("style").Set("display", "none")
	}
}

func showMessage(msg string) {
	if showPopup {
		popup := doc.Call("createElement", "div")
		popup.Get("style").Set("position", "fixed")
		popup.Get("style").Set("top", "50%")
		popup.Get("style").Set("left", "50%")
		popup.Get("style").Set("transform", "translate(-50%, -50%)")
		popup.Get("style").Set("color", "white")
		popup.Get("style").Set("background", "rgba(0,0,0,0.9)")
		popup.Get("style").Set("padding", "20px")
		popup.Get("style").Set("border-radius", "10px")
		popup.Get("style").Set("font-family", "monospace")
		popup.Set("textContent", msg+"\n\nRestarting...")
		doc.Get("body").Call("appendChild", popup)

		time.AfterFunc(2*time.Second, func() {
			popup.Call("remove")
			initGrid()
		})
	} else {
		time.AfterFunc(1*time.Second, func() {
			initGrid()
		})
	}
}

func findParam(params, name string) int {
	search := name + "="
	for i := 0; i <= len(params)-len(search); i++ {
		if params[i] == '?' || params[i] == '&' {
			if i+1 <= len(params)-len(search) && params[i+1:i+1+len(search)] == search {
				return i + 1 + len(search)
			}
		}
	}
	return -1
}

func extractParamValue(params string, start int) string {
	end := start
	for end < len(params) && params[end] != '&' {
		end++
	}
	return params[start:end]
}
