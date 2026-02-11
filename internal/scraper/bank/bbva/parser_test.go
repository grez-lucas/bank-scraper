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

func TestParseTransactions(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "transactions")

	got, err := ParseTransactions(html)

	// Helper to make the test data readable
	dateHelper := func(day int) time.Time {
		return time.Date(2026, 1, day, 0, 0, 0, 0, time.UTC)
	}

	want := []bank.Transaction{
		{
			ID:           "0000001398",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "*C/ HAB4ta   0130007",
			Amount:       999273, // 9,992.73 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "015",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001400",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "*C/PH4OB",
			Amount:       345518, // 3,455.18 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "015",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001396",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "*C/ HAB5ta   0130006",
			Amount:       200000, // 2,000.00 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "015",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001403",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(31), // Value date is 31st
			Description:  "COMISION DE MANTENIMIENTO",
			Amount:       3000, // 30.00 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "638",
				"Office": "0923",
			},
		},
		{
			ID:           "0000001402",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "*/COMIS.TRASPASO OTRO BANCO-",
			Amount:       200, // 2.00 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "015",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001395",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "ITF",
			Amount:       90, // 0.90 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "527",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001399",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "ITF",
			Amount:       45, // 0.45 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "527",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001401",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "ITF",
			Amount:       15, // 0.15 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "527",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001397",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "ITF",
			Amount:       10, // 0.10 (Positive)
			Type:         bank.TransactionDebit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "527",
				"Office": "0437",
			},
		},
		{
			ID:           "0000001394",
			Reference:    "",
			Date:         dateHelper(30),
			ValueDate:    dateHelper(30),
			Description:  "ENTREGA A RENDIR",
			Amount:       1800000, // 18,000.00
			Type:         bank.TransactionCredit,
			BalanceAfter: nil,
			Extra: map[string]string{
				"Codigo": "507",
				"Office": "0437",
			},
		},
	}

	assert.NoError(t, err)
	assert.Len(t, got, 10)

	assert.EqualValues(t, want, got)
}

func TestParseTransactions_InvalidHTML(t *testing.T) {
	html := `<html><body>Something unexpected happened</body></html>`

	txns, err := ParseTransactions(html)

	assert.Error(t, err)
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
	assert.ErrorContains(t, err, "table not found with selector:")
	assert.Nil(t, txns)
}

func TestParseTransactions_EmptyTransactions(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "transactions_empty")

	got, err := ParseTransactions(html)

	// NOTE: We should get no error and an empty transaction slice
	assert.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

func TestParseTransactions_InvalidRow(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "transactions_invalid")

	got, err := ParseTransactions(html)

	assert.Error(t, err)
	assert.Nil(t, got)

	assert.ErrorIs(t, err, bank.ErrParsingFailed)
}

func TestParseBankDate(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Time
		wantErr bool
	}{
		{
			"correct bank date",
			"30-01-2026",
			time.Date(2026, 0o1, 30, 0, 0, 0, 0, time.UTC),
			false,
		},
		{
			"invalid date",
			"01-30-2026",
			time.Time{},
			true,
		},
		{
			"unformatted bank date",
			"30012026",
			time.Time{},
			true,
		},
		{
			"empty string",
			"",
			time.Time{},
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseBankDate(tc.input)

			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestParseSpanishAmount(t *testing.T) {
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
			name:    "small number",
			input:   "45",
			want:    int64(4500),
			wantErr: false,
		},
		{
			name:    "negative number",
			input:   "-45",
			want:    int64(-4500),
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
			got, err := ParseSpanishAmount(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}
