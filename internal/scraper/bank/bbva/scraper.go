// Package bbva defines the scraper and parsing logic to process the BBVA
// portal.
package bbva

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

type BBVAScraper struct {
	browser *rod.Browser
}

func NewBBVAScraper() *BBVAScraper {
	// Launch w/ stealth mode
	url := launcher.New().Headless(true).MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	return &BBVAScraper{browser: browser}
}

// 1. Gotta inject BBVA credentials somehow
// 2. Gotta make the Login stuff follow the interface
