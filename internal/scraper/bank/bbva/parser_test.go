package bbva

import (
	"testing"
	"time"

	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/testutil"
	"github.com/stretchr/testify/assert"
)

func TestParseBalance_PEN(t *testing.T) {
	// Load fixture
	html := testutil.LoadFixture(t, "bbva", "balance_pen")

	// Test parsing
	balance, err := ParseBalancePEN(html)

	assert.NoError(t, err)
	assert.NotNil(t, balance)
	if balance != nil {
		assert.Equal(t, bank.CurrencyPEN, balance.Currency)
		assert.Equal(t, "XXXX-XXXX-XX-XXXXXXXX", balance.AccountID)
		assert.Equal(t, int64(11_074_32), balance.CurrentBalance)
		assert.Equal(t, int64(11_074_30), balance.AvailableBalance)
		assert.WithinDuration(t, time.Now(), balance.FetchedAt, 10*time.Second)
	}
}

func TestParseBalance_USD(t *testing.T) {
	// Load fixture
	html := testutil.LoadFixture(t, "bbva", "balance_usd")

	balance, err := ParseBalanceUSD(html)
	assert.NoError(t, err)
	assert.NotNil(t, balance)

	if balance != nil {
		assert.Equal(t, bank.CurrencyUSD, balance.Currency)
		assert.Equal(t, "XXXX-XXXX-XX-XXXXXXXX", balance.AccountID)
		assert.Equal(t, int64(11_616_79), balance.CurrentBalance)
		assert.Equal(t, int64(11_616_70), balance.AvailableBalance)
		assert.WithinDuration(t, time.Now(), balance.FetchedAt, 10*time.Second)
	}
}

func TestParseBalance_InvalidHTML(t *testing.T) {
	html := `<html><body>Something unexcepted</body></html>`

	_, err := ParseBalanceUSD(html)
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
}

func TestParseBalance_InvalidAmount(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "balance_invalid")

	_, err := ParseBalancePEN(html)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "failed to convert current balance")
}

func TestParseBalance_NoAccountsTableButOnlyJuridicTable(t *testing.T) {
	// Load fixture
	html := testutil.LoadFixture(t, "bbva", "balance_empty")

	_, err := ParseBalanceUSD(html)
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
	assert.ErrorContains(t, err, string(bank.CurrencyUSD))
}

func TestToCents(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:    "normal number",
			input:   "12,345.67",
			want:    int64(12_345_67),
			wantErr: false,
		},
		{
			name:    "empty string",
			input:   "",
			want:    0,
			wantErr: true,
		},
		{
			name:    "string with only removable chars",
			input:   ".,  .",
			want:    0,
			wantErr: true,
		},
		{
			name:    "number without decimal point",
			input:   "123",
			want:    int64(12300),
			wantErr: false,
		},
		{
			name:    "number with one decimal point",
			input:   "123.1",
			want:    int64(12310),
			wantErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ToCents(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
