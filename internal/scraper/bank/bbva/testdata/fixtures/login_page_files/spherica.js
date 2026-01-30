function pintar(objeto) {
    objeto.classList.add("has-value");
}

function despintar(objeto,col) {
    if(objeto.value == "") objeto.classList.remove("has-value");
}

$(document).ready(function() {
    initializeCoronitaIcons();
});

function habilitarTipoDocumento() {
    document.getElementById("numeroDocumento").disabled = false;
    document.getElementById('numeroDocumento').value = "";
    despintar(document.getElementById('numeroDocumento'));
}

function validaCodigoEmpresa(_event) {
    const control = reseteoForm.get('codigoEmpresa');
    const ref = control.getRef();
    const errors = control.getErrors();
}

function _resetTextMessageErrorInLabelConditional() {
    var message = document.getElementById("error-message").style.display = "block";
    if(message.textContent == '')
        message.style.display = "none";
    else
        message.style.display = "block";
}