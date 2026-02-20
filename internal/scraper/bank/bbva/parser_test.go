package bbva

import (
	"strings"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank"
	"github.com/grez-lucas/bank-scraper/internal/scraper/bank/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAccountBalances_ListView(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "accounts_list")

	balances, err := ParseAccountBalances(html)

	assert.NoError(t, err)
	assert.Len(t, balances, 2)

	// PEN account (first table in the list view)
	pen := balances[0]
	assert.Equal(t, bank.CurrencyPEN, pen.Currency)
	assert.Equal(t, "•4607", pen.AccountID)
	assert.Equal(t, int64(857797), pen.AvailableBalance)
	assert.Equal(t, int64(857797), pen.CurrentBalance)
	assert.WithinDuration(t, time.Now(), pen.FetchedAt, 10*time.Second)

	// USD account (second table)
	usd := balances[1]
	assert.Equal(t, bank.CurrencyUSD, usd.Currency)
	assert.Equal(t, "•4615", usd.AccountID)
	assert.Equal(t, int64(1041679), usd.AvailableBalance)
	assert.Equal(t, int64(1041679), usd.CurrentBalance)
	assert.WithinDuration(t, time.Now(), usd.FetchedAt, 10*time.Second)
}

func TestParseAccountBalances_TileView(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "accounts_tile")

	balances, err := ParseAccountBalances(html)

	assert.NoError(t, err)
	assert.Len(t, balances, 2)

	// PEN card
	pen := balances[0]
	assert.Equal(t, bank.CurrencyPEN, pen.Currency)
	assert.Equal(t, "PE001101190100064607", pen.AccountID)
	assert.Equal(t, int64(857797), pen.AvailableBalance)
	assert.Equal(t, int64(0), pen.CurrentBalance) // Tile view only has available balance
	assert.WithinDuration(t, time.Now(), pen.FetchedAt, 10*time.Second)

	// USD card
	usd := balances[1]
	assert.Equal(t, bank.CurrencyUSD, usd.Currency)
	assert.Equal(t, "PE001101190100064615", usd.AccountID)
	assert.Equal(t, int64(1041679), usd.AvailableBalance)
	assert.Equal(t, int64(0), usd.CurrentBalance)
	assert.WithinDuration(t, time.Now(), usd.FetchedAt, 10*time.Second)
}

func TestParseAccountBalances_InvalidHTML(t *testing.T) {
	html := `<html><body>Something unexpected</body></html>`

	_, err := ParseAccountBalances(html)
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
	assert.ErrorContains(t, err, "no account elements found")
}

func TestCurrencyFromSymbol(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     bank.Currency
		wantErr  bool
		errMatch string
	}{
		{"PEN symbol", CurrencySymbolPEN, bank.CurrencyPEN, false, ""},
		{"USD symbol", CurrencySymbolUSD, bank.CurrencyUSD, false, ""},
		{"empty string", "", "", true, "unknown currency symbol"},
		{"euro symbol", "€", "", true, "unknown currency symbol"},
		{"full currency name", "PEN", "", true, "unknown currency symbol"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := currencyFromSymbol(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.errMatch)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestCurrencyFromCode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     bank.Currency
		wantErr  bool
		errMatch string
	}{
		{"PEN code", CurrencyCodePEN, bank.CurrencyPEN, false, ""},
		{"USD code", CurrencyCodeUSD, bank.CurrencyUSD, false, ""},
		{"empty string", "", "", true, "unknown currency code"},
		{"lowercase pen", "pen", "", true, "unknown currency code"},
		{"symbol instead of code", "S/", "", true, "unknown currency code"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := currencyFromCode(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				assert.ErrorContains(t, err, tc.errMatch)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestParseAccountBalances_ListViewMissingCurrency(t *testing.T) {
	html := `<html><body>
		<bbva-btge-accounts-solution-table class="accountsTable" list-group-entity="BBVA">
			<table><tbody>
				<tr class="row">
					<td><bbva-table-body-text class="accountDescription" text="•1234"></bbva-table-body-text></td>
					<td><bbva-table-body-amount class="availableBalance" amount="100.00" currency="S/"></bbva-table-body-amount></td>
					<td><bbva-table-body-amount class="accountedBalance" amount="100.00" currency="S/"></bbva-table-body-amount></td>
				</tr>
			</tbody></table>
		</bbva-btge-accounts-solution-table>
	</body></html>`

	_, err := ParseAccountBalances(html)
	_ = err
	assert.Error(t, err)
	assert.ErrorContains(t, err, "missing list-group-currency")
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
}

func TestParseAccountBalances_ListViewMissingAvailableBalance(t *testing.T) {
	html := `<html><body>
		<bbva-btge-accounts-solution-table class="accountsTable" list-group-currency="PEN">
			<table><tbody>
				<tr class="row">
					<td><bbva-table-body-text class="accountDescription" text="•1234"></bbva-table-body-text></td>
					<td><bbva-table-body-amount class="availableBalance" currency="S/"></bbva-table-body-amount></td>
					<td><bbva-table-body-amount class="accountedBalance" amount="100.00" currency="S/"></bbva-table-body-amount></td>
				</tr>
			</tbody></table>
		</bbva-btge-accounts-solution-table>
	</body></html>`

	_, err := ParseAccountBalances(html)
	_ = err
	assert.Error(t, err)
	assert.ErrorContains(t, err, "missing available balance amount")
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
}

func TestParseAccountBalances_ListViewMissingAccountedBalance(t *testing.T) {
	html := `<html><body>
		<bbva-btge-accounts-solution-table class="accountsTable" list-group-currency="PEN">
			<table><tbody>
				<tr class="row">
					<td><bbva-table-body-text class="accountDescription" text="•1234"></bbva-table-body-text></td>
					<td><bbva-table-body-amount class="availableBalance" amount="100.00" currency="S/"></bbva-table-body-amount></td>
					<td><bbva-table-body-amount class="accountedBalance" currency="S/"></bbva-table-body-amount></td>
				</tr>
			</tbody></table>
		</bbva-btge-accounts-solution-table>
	</body></html>`

	_, err := ParseAccountBalances(html)
	_ = err
}

func TestParseAccountBalances_TileViewUnknownCurrencySymbol(t *testing.T) {
	html := `<html><body>
		<bbva-btge-card-product-select id="XX001" product-amount="500.00" product-amount-currency="€">
		</bbva-btge-card-product-select>
	</body></html>`

	_, err := ParseAccountBalances(html)
	_ = err
	assert.Error(t, err)
	assert.ErrorContains(t, err, "unknown currency symbol: \"€\"")
	assert.ErrorIs(t, err, bank.ErrParsingFailed)
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

func TestDetectLoginError_404(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "login_error_404")

	gotErr := DetectLoginError(html, 404)

	wantCode := "EAI0000"
	wantMsg := "No pudimos iniciar tu sesión"
	wantStatus := 404

	assert.NotNil(t, gotErr)
	assert.Error(t, gotErr)

	var loginErr *LoginErrorInfo
	assert.ErrorAs(t, gotErr, &loginErr)

	assert.Equal(t, wantCode, loginErr.Code)
	assert.Equal(t, wantMsg, loginErr.Message)
	assert.Equal(t, wantStatus, loginErr.HTTPStatus)
}

func TestDetectLoginError_InvalidCredentials(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "login_error")

	gotErr := DetectLoginError(html, 200)

	assert.NotNil(t, gotErr)
	assert.Error(t, gotErr)

	var loginErr *LoginErrorInfo
	assert.ErrorAs(t, gotErr, &loginErr)
	assert.Equal(t, "", loginErr.Code)
	assert.Equal(t, 200, loginErr.HTTPStatus)
	assert.Equal(t, "Es necesario que corrijas los datos que ingresaste para poder continuar.", loginErr.Message)
}

func TestDetectLoginError_HTTPErrors(t *testing.T) {
	tests := []struct {
		name        string
		hasFixture  bool
		fixtureName string
		statusCode  int
		wantErr     bool
		wantErrMsg  string
	}{
		{
			name:       "503 Service Unavailable returns LoginErrorInfo",
			hasFixture: false,
			statusCode: 503,
			wantErr:    true,
			wantErrMsg: "Bank service temporarily unavailable or rate limited",
		},
		{
			name:       "429 Rate Limited returns LoginErrorInfo",
			hasFixture: false,
			statusCode: 429,
			wantErr:    true,
			wantErrMsg: "Bank service temporarily unavailable or rate limited",
		},
		{
			name:       "403 Forbidden returns LoginErrorInfo",
			hasFixture: false,
			statusCode: 403,
			wantErr:    true,
			wantErrMsg: "Access forbidden - possible bot detection",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var html string
			if tc.hasFixture {
				html = testutil.LoadFixture(t, "bbva", tc.fixtureName)
			}

			gotErr := DetectLoginError(html, tc.statusCode)

			if !tc.wantErr {
				assert.NoError(t, gotErr)
			} else {
				var loginErr *LoginErrorInfo
				assert.ErrorAs(t, gotErr, &loginErr)

				assert.Equal(t, tc.wantErrMsg, loginErr.Message)
				assert.Equal(t, tc.statusCode, loginErr.HTTPStatus)
			}
		})
	}
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

func Test_hasNoMovements(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			"no table present",
			"<html><body></body></html>",
			false,
		},
		{
			"table with state noresults",
			`<html><body><bbva-btge-accounts-solution-table id="moviments-table" state="noresults"></bbva-btge-accounts-solution-table></body></html>`,
			true,
		},
		{
			"table with empty state",
			`<html><body><bbva-btge-accounts-solution-table id="moviments-table" state=""></bbva-btge-accounts-solution-table></body></html>`,
			false,
		},
		{
			"table without state attribute",
			`<html><body><bbva-btge-accounts-solution-table id="moviments-table"></bbva-btge-accounts-solution-table></body></html>`,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)
			got := hasNoMovements(doc)
			assert.Equal(t, tt.want, got)
		})
	}
}
