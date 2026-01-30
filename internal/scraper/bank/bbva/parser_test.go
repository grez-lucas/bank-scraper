package bbva

import (
	"testing"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/testutil"
	"github.com/stretchr/testify/assert"
)

func TestParseBalance_PEN(t *testing.T) {
	// Load fixture
	html := testutil.LoadFixture(t, "bbva", "balance_pen")

	// Test parsing
	balance, err := ParseBalance(html)

	assert.NoError(t, err)
	assert.Equal(t, balance, 8_555.83)
}
