package bbva

// CSS Selectors for BBVA Bank Web Portal
const (
	// Login page
	SelectorCompanyInput  = "input#empresa"
	SelectorUserInput     = "input#usuario"
	SelectorPasswordInput = "input#clave_acceso_ux"

	SelectorLoginButton = "button#enviarSenda"

	// Login error page
	SelectorLoginErrorCode    = "div.error-code.error-title"
	SelectorLoginErrorMessage = "h1.title"
	SelectorLoginErrorSpan    = "span#error-message.coronita-small-icon-warning.icon-info-svg-warning.span-error"

	// Dashboard Page
	SelectorDashboard = "table#kyop-boby-table.kyop-boby-table"

	// Balance Page
	SelectorAccountsTableRows = "#tabla-contenedor0_1 tbody tr:not(.tb_column_header)"

	// Transactions Page
	SelectorTransactionsTable     = "table#tabladatos"
	SelectorTransactionsTableRows = "#tabladatos tbody tr:not(.tb_column_header):not(.tb_total)"
	SelectorNoMovementsError      = "div.msj_ico.msj_err"
)
