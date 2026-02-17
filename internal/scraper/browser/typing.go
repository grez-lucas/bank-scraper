// Package browser provides utilities for browser automation with Rod.
package browser

import (
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
)

// TypeHuman types text into an element with human-like timing.
// It uses Element.Type() which properly triggers keyboard events (keydown/keyup).
// Small random delays (50-150ms) between keystrokes simulate human typing.
func TypeHuman(el *rod.Element, text string) error {
	for _, char := range text {
		if err := el.Type(input.Key(char)); err != nil {
			return err
		}
		// Small random delay to simulate human typing
		time.Sleep(time.Duration(50+rand.Intn(100)) * time.Millisecond)
	}
	return nil
}

// TypeFast types text quickly without delays.
// Useful for tests and replay mode where speed matters more than human simulation.
// Still triggers proper keyboard events (keydown/keyup) for each character.
func TypeFast(el *rod.Element, text string) error {
	keys := make([]input.Key, len(text))
	for i, char := range text {
		keys[i] = input.Key(char)
	}
	return el.Type(keys...)
}
