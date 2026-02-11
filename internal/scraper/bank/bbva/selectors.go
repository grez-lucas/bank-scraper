package bbva

// CSS Selectors for BBVA Bank Web Portal
const (
	// Login page
	SelectorCompanyInput  = "#empresa"
	SelectorUserInput     = "#usuario"
	SelectorPasswordInput = "#clave_acceso_ux"

	// Balance Page
	SelectorAccountsTableRows = "#tabla-contenedor0_1 tbody tr:not(.tb_column_header)"

	// Transactions Page
	SelectorTransactionsTable     = "table#tabladatos"
	SelectorTransactionsTableRows = "#tabladatos tbody tr:not(.tb_column_header):not(.tb_total)"
	SelectorNoMovementsError      = "div.msj_ico.msj_err"
)
