// Package browser provides utilities for browser automation with Rod.
package browser

import (
	"fmt"

	"github.com/go-rod/rod"
)

// WaitForIFrames recursively waits for DOM stability on all visible iframes.
// This ensures that iframe content is fully loaded before interaction.
func WaitForIFrames(page *rod.Page) {
	page.MustWaitDOMStable()

	iframes, err := page.Elements("iframe")
	if err != nil {
		return
	}

	for _, iframe := range iframes {
		visible, _ := iframe.Visible()
		if !visible {
			continue
		}

		frame, err := iframe.Frame()
		if err != nil {
			continue
		}

		WaitForIFrames(frame)
	}
}

// GetDeepestVisibleFrame recursively navigates into the deepest visible iframe.
// Returns the innermost page/frame context for element interaction.
// If no visible iframe is found, returns the original page.
func GetDeepestVisibleFrame(page *rod.Page) *rod.Page {
	page.MustWaitDOMStable()

	iframes, err := page.Elements("iframe")
	if err != nil {
		return page
	}

	for _, iframe := range iframes {
		if visible, _ := iframe.Visible(); visible {
			child := iframe.MustFrame()
			return GetDeepestVisibleFrame(child)
		}
	}

	return page
}

// GetIFrameBySelector returns the frame context for a specific iframe selector.
// The returned *rod.Page can be used to interact with elements inside the iframe.
func GetIFrameBySelector(page *rod.Page, selector string) (*rod.Page, error) {
	iframeEl, err := page.Element(selector)
	if err != nil {
		return nil, fmt.Errorf("iframe element not found: %w", err)
	}

	frame, err := iframeEl.Frame()
	if err != nil {
		return nil, fmt.Errorf("failed to get frame context: %w", err)
	}

	return frame, nil
}
