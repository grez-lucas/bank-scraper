function direccion() {
	document.forms[0].action = "https://www.bbvanetcash.pe/DFAUTH85/slod_pe_web/DFServlet";
}
function redire() {
	top.location = 'login_pibee.html';
}

function redireError() {
	top.location = 'logout_pibee_prev_error.html';
}