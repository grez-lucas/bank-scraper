package bbva

// CSS Selectors for BBVA Bank Web Portal
const (
	// Login page
	SelectorCompanyInput  = "input#empresa"
	SelectorUserInput     = "input#usuario"
	SelectorPasswordInput = "input#clave_acceso_ux"
	// Use legacy button to bypass micro-frontend postMessage flow.
	// The #enviarSenda button sends credentials to an iframe via postMessage,
	// which doesn't work in HAR replay mode.
	SelectorLoginButton = "button#aceptar"

	// Login error page
	SelectorLoginErrorCode    = "div.error-code.error-title"
	SelectorLoginErrorMessage = "h1.title"
	SelectorLoginErrorSpan    = "span#error-message.coronita-small-icon-warning.icon-info-svg-warning.span-error"

	// Dashboard Page
	SelectorDashboard = "table#kyop-boby-table.kyop-boby-table"

	// Balance Page (pre-2026)
	SelectorAccountsTableRows = "#tabla-contenedor0_1 tbody tr:not(.tb_column_header)"

	// Accounts Page (2026 redesign)
	// View toggle
	SelectorViewToggleTile = `bbva-button-group-item[value="TiledView"]`
	SelectorViewToggleList = `bbva-button-group-item[value="ListView"]`

	// List view
	SelectorAccountAccordion   = `bbva-expandable-accordion.entity-accordion`
	SelectorAccountTable       = `bbva-btge-accounts-solution-table.accountsTable`
	SelectorAccountRow         = `tr.row`
	SelectorAccountDescription = `bbva-table-body-text.accountDescription`
	SelectorAvailableBalance   = `bbva-table-body-amount.availableBalance`
	SelectorAccountedBalance   = `bbva-table-body-amount.accountedBalance`
	SelectorExpandButton       = `button.header-accordion`

	// Tile view
	SelectorAccountCard = `bbva-btge-card-product-select`

	// Transactions Page (pre-2026)
	SelectorTransactionsTable     = "table#tabladatos"
	SelectorTransactionsTableRows = "#tabladatos tbody tr:not(.tb_column_header):not(.tb_total)"
	SelectorNoMovementsError      = "div.msj_ico.msj_err"
)
