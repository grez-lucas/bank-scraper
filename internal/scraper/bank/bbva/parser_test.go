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

func int64Ptr(v int64) *int64 { return &v }

func TestParseTransactions(t *testing.T) {
	html := testutil.LoadFixture(t, "bbva", "transactions")

	got, err := ParseTransactions(html)

	require.NoError(t, err)
	require.Len(t, got, 50)

	// Row 0: first row — debit, Feb 2026
	row0 := got[0]
	assert.Equal(t, "1411", row0.ID)
	assert.Equal(t, "", row0.Reference)
	assert.Equal(t, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), row0.Date)
	assert.Equal(t, time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC), row0.ValueDate)
	assert.Equal(t, "PAGO FACTURA | SUNAT DETRACCIONES", row0.Description)
	assert.Equal(t, int64(350), row0.Amount)
	assert.Equal(t, bank.TransactionDebit, row0.Type)
	require.NotNil(t, row0.BalanceAfter)
	assert.Equal(t, int64(857797), *row0.BalanceAfter)
	assert.Equal(t, "151", row0.Extra["Codigo"])
	assert.Equal(t, "*Mp: 20607818054S Com Sunat Detraccione@", row0.Extra["Beneficiary"])

	// Row 8: debit with different op/value dates (30 Ene vs 31 Ene)
	row8 := got[8]
	assert.Equal(t, "1403", row8.ID)
	assert.Equal(t, "", row8.Reference)
	assert.Equal(t, time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC), row8.Date)
	assert.Equal(t, time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC), row8.ValueDate)
	assert.Equal(t, "COMISION DE MANTENIMIENTO", row8.Description)
	assert.Equal(t, int64(3000), row8.Amount)
	assert.Equal(t, bank.TransactionDebit, row8.Type)
	require.NotNil(t, row8.BalanceAfter)
	assert.Equal(t, int64(1107432), *row8.BalanceAfter)
	assert.Equal(t, "638", row8.Extra["Codigo"])
	assert.Equal(t, "Comision De Mantenimiento", row8.Extra["Beneficiary"])

	// Row 17: credit (positive amount)
	row17 := got[17]
	assert.Equal(t, "1394", row17.ID)
	assert.Equal(t, "", row17.Reference)
	assert.Equal(t, time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC), row17.Date)
	assert.Equal(t, time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC), row17.ValueDate)
	assert.Equal(t, "ABONO POR TRASPASO", row17.Description)
	assert.Equal(t, int64(1800000), row17.Amount)
	assert.Equal(t, bank.TransactionCredit, row17.Type)
	require.NotNil(t, row17.BalanceAfter)
	assert.Equal(t, int64(2655583), *row17.BalanceAfter)
	assert.Equal(t, "507", row17.Extra["Codigo"])
	assert.Equal(t, "Entrega A Rendir                       @", row17.Extra["Beneficiary"])

	// Row 49: last row — debit, Nov 2025
	row49 := got[49]
	assert.Equal(t, "1362", row49.ID)
	assert.Equal(t, "", row49.Reference)
	assert.Equal(t, time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC), row49.Date)
	assert.Equal(t, time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC), row49.ValueDate)
	assert.Equal(t, "NOTA DE CARGO", row49.Description)
	assert.Equal(t, int64(170000), row49.Amount)
	assert.Equal(t, bank.TransactionDebit, row49.Type)
	require.NotNil(t, row49.BalanceAfter)
	assert.Equal(t, int64(1177375), *row49.BalanceAfter)
	assert.Equal(t, "015", row49.Extra["Codigo"])
	assert.Equal(t, "*C/Ph4Ob", row49.Extra["Beneficiary"])
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
	// Inline HTML with empty date attr to trigger parse error
	html := `<html><body>
		<bbva-btge-accounts-solution-table id="moviments-table" state="loaded" total-items="1">
			<table><tbody>
				<tr class="row" data-actionable>
					<td><bbva-table-body-date class="operationDate" date="" year="2026"></bbva-table-body-date></td>
					<td><bbva-table-body-date class="valueDate" date="10 Feb" year="2026"></bbva-table-body-date></td>
					<td><bbva-table-body-text class="code" text="151"></bbva-table-body-text></td>
					<td><bbva-table-body-text class="numberMovement" text="1411"></bbva-table-body-text></td>
					<td><bbva-table-body-text class="concept" text="TEST" description="test"></bbva-table-body-text></td>
					<td><bbva-table-body-amount class="transactionAmount" amount="-3.5" secondary-amount="100.00"></bbva-table-body-amount></td>
				</tr>
			</tbody></table>
		</bbva-btge-accounts-solution-table>
	</body></html>`

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
	assert.Equal(t, "EAI0000", loginErr.Code)
	assert.Equal(t, 200, loginErr.HTTPStatus)
	assert.Equal(t, "No pudimos iniciar tu sesión", loginErr.Message)
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

func TestParseBankDate2026(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		year    string
		want    time.Time
		wantErr bool
	}{
		{
			"Feb date",
			"10 Feb",
			"2026",
			time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC),
			false,
		},
		{
			"Ene (January in Spanish)",
			"30 Ene",
			"2026",
			time.Date(2026, 1, 30, 0, 0, 0, 0, time.UTC),
			false,
		},
		{
			"Dic (December in Spanish)",
			"31 Dic",
			"2025",
			time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC),
			false,
		},
		{
			"Nov date",
			"28 Nov",
			"2025",
			time.Date(2025, 11, 28, 0, 0, 0, 0, time.UTC),
			false,
		},
		{
			"empty date",
			"",
			"2026",
			time.Time{},
			true,
		},
		{
			"empty year",
			"10 Feb",
			"",
			time.Time{},
			true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseBankDate2026(tc.date, tc.year)
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

func TestHasMoreTransactions(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    bool
	}{
		{"transactions with Ver más footer", "transactions", true},
		{"transactions_load_more with Ver más footer", "transactions_load_more", true},
		{"empty transactions page", "transactions_empty", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := testutil.LoadFixture(t, "bbva", tc.fixture)
			got := HasMoreTransactions(html)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestHasMoreTransactions_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			"no table at all",
			`<html><body></body></html>`,
			false,
		},
		{
			"table without footer",
			`<html><body>
				<bbva-btge-accounts-solution-table id="moviments-table" state="" total-items="5">
					<table><tbody></tbody></table>
				</bbva-btge-accounts-solution-table>
			</body></html>`,
			false,
		},
		{
			"footer with different class (table-footer, not footer-link-text)",
			`<html><body>
				<bbva-table-footer class="table-footer" loading="" loading-text="Cargando"></bbva-table-footer>
			</body></html>`,
			false,
		},
		{
			"footer-link-text present",
			`<html><body>
				<bbva-table-footer class="footer-link-text" variant="footer">Ver más</bbva-table-footer>
			</body></html>`,
			true,
		},
		{
			"empty HTML",
			``,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := HasMoreTransactions(tc.html)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetectAnnouncementModal(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    bool
	}{
		{"login_popup has opened modal", "login_popup", true},
		{"dashboard_news_popup has opened modal", "dashboard_news_popup", true},
		{"accounts_news_popup has opened modal", "accounts_news_popup", true},
		{"dashboard without popup", "dashboard", false},
		{"accounts_list without popup", "accounts_list", false},
		{"transactions without popup", "transactions", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := testutil.LoadFixture(t, "bbva", tc.fixture)
			got := DetectAnnouncementModal(html)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestDetectAnnouncementModal_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			"no modal element",
			`<html><body><div>no modal here</div></body></html>`,
			false,
		},
		{
			"modal without opened attr",
			`<html><body><bbva-btge-microfrontend-modal></bbva-btge-microfrontend-modal></body></html>`,
			false,
		},
		{
			"modal with opened attr",
			`<html><body><bbva-btge-microfrontend-modal opened=""></bbva-btge-microfrontend-modal></body></html>`,
			true,
		},
		{
			"empty HTML",
			``,
			false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectAnnouncementModal(tc.html)
			assert.Equal(t, tc.want, got)
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
