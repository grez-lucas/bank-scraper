package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatAmount(t *testing.T) {
	tests := []struct {
		cents int64
		want  string
	}{
		{123456, "1234.56"},
		{100, "1.00"},
		{5, "0.05"},
		{0, "0.00"},
		{-50, "-0.50"},
		{-123456, "-1234.56"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, FormatAmount(tt.cents), "FormatAmount(%d)", tt.cents)
	}
}

func TestMaskAccountNumber(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"PE001101190100064607", "XXXXXXXXXXXXXXXX4607"},
		{"12345678", "XXXX5678"},
		{"1234", "1234"},
		{"123", "123"},
		{"", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, MaskAccountNumber(tt.input), "MaskAccountNumber(%q)", tt.input)
	}
}
