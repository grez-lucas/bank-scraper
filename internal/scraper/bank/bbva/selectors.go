package bbva

// CSS Selectors for BBVA Bank Web Portal
const (
	// Login page
	SelectorCompanyInput  = "input#empresa"
	SelectorUserInput     = "input#usuario"
	SelectorPasswordInput = "input#clave_acceso_ux"
	// Senda flow: clicks #enviarSenda which sends credentials via postMessage
	// to iframe#microfrontend, which calls the Senda API (grantingTicket/V02).
	SelectorLoginButton = "button#enviarSenda"

	// Senda login error display element
	SelectorLoginErrorSpan = `span#error-message`

	// SendaAPIURL is the grantingTicket endpoint used by the Senda login flow.
	// In replay mode, the scraper probes this URL directly via fetch() to bypass
	// the broken iframe postMessage chain.
	SendaAPIURL = "https://asosenda.bbva.pe/TechArchitecture/pe/grantingTicket/V02"

	// Login error page (legacy DFServlet flow — kept for reference)
	SelectorLoginErrorCode    = "div.error-code.error-title"
	SelectorLoginErrorMessage = "h1.title"

	// PortalPath is the URL path the browser redirects to on successful Senda login.
	PortalPath = "/nextgenempresas/portal/"

	// Announcement modal (post-login news popup)
	SelectorAnnouncementModal    = `bbva-btge-microfrontend-modal[opened]`
	SelectorAnnouncementCloseBtn = `bbva-btge-microfrontend-modal[opened] button.close-btn`

	// Dashboard Page
	SelectorDashboard = "bbva-btge-dashboard-solution-home-page#cells-template-bbva-btge-dashboard-solution-home"

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

	// DashboardRoute is the SPA hash fragment set after successful login.
	// The 2026 portal updates the URL to include this after the
	// "Validando tus credenciales" splash transitions to the dashboard.
	DashboardRoute = "bbva-btge-dashboard-solution"

	// Transactions Page (post-2026)
	SelectorTransactionsTable = `bbva-btge-accounts-solution-table#moviments-table`
	SelectorTransactionRow    = `tr.row[data-actionable]`
	SelectorTxOperationDate   = `bbva-table-body-date.operationDate`
	SelectorTxValueDate       = `bbva-table-body-date.valueDate`
	SelectorTxCode            = `bbva-table-body-text.code`
	SelectorTxMovementNumber  = `bbva-table-body-text.numberMovement`
	SelectorTxConcept         = `bbva-table-body-text.concept`
	SelectorTxAmount          = `bbva-table-body-amount.transactionAmount`

	// Pagination — "Ver más" link in the table footer
	SelectorLoadMoreButton = `bbva-table-footer.footer-link-text`
)
