package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// Parse command line arguments
	help := flag.Bool("h", false, "Show help")
	helpLong := flag.Bool("help", false, "Show help")
	gen := flag.Int("gen", 1000, "Number of generations")
	sleep := flag.Float64("sleep", 0.03, "Sleep between generations (seconds)")
	seed := flag.Int64("seed", 0, "Random seed (0 for random)")
	
	flag.Parse()
	
	// Show help
	if *help || *helpLong {
		showHelp()
		return
	}
	
	// Use full terminal size
	width := getTerminalWidth() / 2
	height := getTerminalHeight() - 1
	
	// Initialize random seed
	if *seed == 0 {
		*seed = rand.Int63()
	}
	rand.Seed(*seed)
	
	// Set up signal handling for Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	
	// Hide cursor and clear screen (noscroll mode by default)
	hideCursor()
	defer showCursor()
	clearScreen()
	
	// Handle Ctrl+C gracefully
	go func() {
		<-sigChan
		// Restore cursor
		showCursor()
		// Move cursor below the game area
		fmt.Printf("\033[%d;0H", height+2)
		fmt.Printf("\nInterrupted by user\n")
		fmt.Printf("Seed: %d\n", *seed)
		os.Exit(0)
	}()
	
	// Print seed at exit
	defer func() {
		// Move cursor to bottom before printing seed
		fmt.Printf("\033[%d;0H", height+2)
		fmt.Printf("\nSeed: %d\n", *seed)
	}()
	
	// Create grid
	size := width * height
	grid := make([]int, size)
	next := make([]int, size)
	
	// Store previous states for stabilization detection
	// Using map with string keys for state history
	stateHistory := make(map[string]int)
	
	// Random initialization
	for i := 0; i < size; i++ {
		grid[i] = rand.Intn(2)
	}
	
	// Main game loop
	for g := 0; g < *gen; g++ {
		aliveAny := false
		
		// Calculate next generation
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				count := 0
				
				// Check all 8 neighbors
				for dy := -1; dy <= 1; dy++ {
					for dx := -1; dx <= 1; dx++ {
						if dx == 0 && dy == 0 {
							continue
						}
						
						nx := (x + dx + width) % width
						ny := (y + dy + height) % height
						count += grid[nx+ny*width]
					}
				}
				
				// Apply Game of Life rules
				idx := x + y*width
				if grid[idx] == 1 {
					if count == 2 || count == 3 {
						next[idx] = 1
					} else {
						next[idx] = 0
					}
				} else {
					if count == 3 {
						next[idx] = 1
					} else {
						next[idx] = 0
					}
				}
				
				if next[idx] == 1 {
					aliveAny = true
				}
			}
		}
		
		// Exit if no alive cells
		if !aliveAny {
			moveCursorToBottom(height)
			fmt.Printf("Extinction: All cells died at generation %d\n", g+1)
			break
		}
		
		// Check for stabilization before swapping
		currentState := stateToString(grid, width, height)
		if firstGen, exists := stateHistory[currentState]; exists {
			period := g - firstGen
			moveCursorToBottom(height)
			if period == 1 {
				fmt.Printf("Stabilized: Still life detected at generation %d\n", g+1)
			} else {
				fmt.Printf("Stabilized: Oscillator detected with period %d (first seen at gen %d, repeats at gen %d)\n", 
					period, firstGen+1, g+1)
			}
			break
		}
		
		// Store current state in history
		stateHistory[currentState] = g
		
		// Swap grids
		grid, next = next, grid
		
		// Draw frame (in-place update)
		moveCursorToTop()
		draw(grid, width, height)
		
		// Sleep between generations
		time.Sleep(time.Duration(*sleep * float64(time.Second)))
	}
}

func draw(grid []int, width, height int) {
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if grid[x+y*width] == 1 {
				fmt.Print("⬜")
			} else {
				fmt.Print("  ")
			}
		}
		fmt.Println()
	}
}

// Convert grid state to string for history tracking
func stateToString(grid []int, width, height int) string {
	// Simple but effective string representation
	state := make([]byte, len(grid))
	for i, val := range grid {
		if val == 1 {
			state[i] = '1'
		} else {
			state[i] = '0'
		}
	}
	return string(state)
}

func moveCursorToBottom(height int) {
	fmt.Printf("\033[%d;0H", height+3)
}

func showHelp() {
	fmt.Print(`Game of Life - Fullscreen Terminal Version

Usage:
  gameOfLife [options]

Options:
  -gen int        Number of generations (default 1000)
  -seed int       Random seed (0 for random, seed will be printed at exit)
  -sleep float    Sleep between generations in seconds (default 0.03)
  -h, -help       Show this help

Exit Conditions:
  - Extinction: All cells die
  - Still life: Pattern stops changing (period 1)
  - Oscillator: Pattern repeats with period > 1
  - Max generations: Reached -gen limit
  - Ctrl+C: Manual interrupt

Examples:
  gameOfLife                     # Run with defaults
  gameOfLife -gen=500            # Run for 500 generations
  gameOfLife -seed=123           # Deterministic pattern
  gameOfLife -sleep=0.1          # Slower animation

Note: The random seed used is always printed at exit for reproducibility.
Press Ctrl+C to interrupt (cursor will be restored)
`)
}

func getTerminalWidth() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 80
	}
	
	// Parse "height width"
	var height, width int
	fmt.Sscanf(string(out), "%d %d", &height, &width)
	return width
}

func getTerminalHeight() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err := cmd.Output()
	if err != nil {
		return 24
	}
	
	var height, width int
	fmt.Sscanf(string(out), "%d %d", &height, &width)
	return height
}

func hideCursor() {
	fmt.Print("\033[?25l")
}

func showCursor() {
	fmt.Print("\033[?25h")
}

func clearScreen() {
	fmt.Print("\033[2J")
}

func moveCursorToTop() {
	fmt.Print("\033[H")
}
