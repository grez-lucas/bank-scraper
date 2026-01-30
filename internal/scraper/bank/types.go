package bank

import "time"

type Session struct {
	ID        string
	BankCode  BankCode
	ExpiresAt time.Time
	// Internal: browser page reference (not exported)
	page any
}
