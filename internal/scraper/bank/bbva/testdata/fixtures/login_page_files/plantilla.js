/* para completar 8 digitos de referencia y usuario */
function completar(){
	if (parseInt(document.logon.cod_emp.value.length) == 6) {
		empresa = "00" + document.logon.cod_emp.value;
		document.logon.cod_emp.value = empresa;
	}
	if (parseInt(document.logon.cod_usu.value.length) == 4) {
		usuario = document.logon.cod_usu.value;
		document.logon.cod_usu.value = usuario;
	}
}

function validarFormulario() {
	idioma = getCookie('COD_IDIOMISO');

	if (idioma == null) {
		setCookie('COD_IDIOMISO','es');
	}
	
	idioma = getCookie('COD_IDIOMISO');
	completar();
	var f = document.logon;
	f.cod_emp.value = f.cod_emp.value.toUpperCase();
	f.cod_usu.value = f.cod_usu.value.toUpperCase();
	f.eai_URLDestino.value = "/SPEKYOP/kyop_mult_web_pub/index.html";
	
	canal = "0026";
	bancoInt = "0081";
	
	f.idioma.value = traducirIdiomaSlod(idioma);
	
	if (document.getElementById("checkRecData").checked == true) {
	
		var activateEncryptLastEmpresa= document.getElementById("activateEncryptLastEmpresa").value;
		var activateEncryptLastUsuario= document.getElementById("activateEncryptLastUsuario").value;
		
		var auxEmp= "";
		var auxUsu= "";
		
		auxEmp = document.getElementById("empresa").value;
		auxUsu = document.getElementById("usuario").value;
			
		if (activateEncryptLastEmpresa == "Y") {
			auxEmp = document.getElementById("empresaTemp").value;
		}
		
		if (activateEncryptLastUsuario == "Y") {
			auxUsu= document.getElementById("usuarioTemp").value;
		}

		var enmAuxEmp;
		var enmAuxUsu;

		enmAuxEmp = leftpad((auxEmp.substring(auxEmp.length - 4)), 8, "*");
		enmAuxUsu = leftpad((auxUsu.substring(auxUsu.length - 4)), 8, "*");

		v_PAN = auxEmp + '-' + auxUsu + '-';
		v_alias_pad = enmAuxEmp + '-' + enmAuxUsu;

		keyCifrado = getKeyServlet(v_PAN, v_alias_pad, '', 0);
		setCookie("CRE", "");
		if (keyCifrado.length > 1) {
			setCookie("CRE", keyCifrado);
		}
	} else {
		setCookie("CRE", "");
	}
	
	var activateEncryptLastEmpresa = document.getElementById("activateEncryptLastEmpresa").value;
	var activateEncryptLastUsuario = document.getElementById("activateEncryptLastUsuario").value;
	
	if (activateEncryptLastEmpresa == "Y") {
		document.getElementById("empresa").style.display = "none";
		document.getElementById("empresa_aux").style.display = "block";
		document.getElementById("empresa_aux").value = document.getElementById("empresa").value;
		document.getElementById("empresa").value = document.getElementById("empresaTemp").value;
	}
	
	if (activateEncryptLastUsuario == "Y") {
		document.getElementById("usuario").style.display = "none";
		document.getElementById("usuario_aux").style.display = "block";
		document.getElementById("usuario_aux").value = document.getElementById("usuario").value;
		document.getElementById("usuario").value =  document.getElementById("usuarioTemp").value;			
	}
	
	document.getElementById("empresa_aux").value = "";
	document.getElementById("usuario_aux").value = "";
	
	f.eai_user.value = canal + bancoInt + f.cod_emp.value + f.cod_usu.value;
	
	return true;
}

function traducirIdiomaSlod(idioma) {
	if (idioma == "es") {
		return "CAS";
	} else if (idioma == "en") {
		return "ING";
	}
}

function accesoSeguro(campo) {
	if (campo.checked)
		tvOpen();
	else
		tvClose();
}
  
function trim(str) {
	str = str.replace( /^\s+/g, "" );  // espacios a la derecha 
	return str.replace( /\s+$/g, "" ); // espacios a la isquierda
}	
	
function getKeyServlet(PAN, Alias, Cryptogram, Function) {
	var resultado = '';  
	var servletLocal = '/DFAUTH85/nNigmaservletAO/nNigmaServlet';
	var AliasTmp = Alias;
	var ueCrypto = unescape(unescape(Cryptogram));
	var urlserv = servletLocal+"?PAN="+encodeURIComponent(PAN) + "&Alias=" + encodeURIComponent(AliasTmp) + "&Cryptogram=" + encodeURIComponent(ueCrypto) + "&Function=" + Function;	
	$.ajax({
		type: 'get',
		url: urlserv,
		cache : false,
		success: function (data) {
			try{
				var cifrado;
				cifrado = data.substring(9);
				resultado = escape(cifrado);
				
				if (resultado.length > 3 && resultado.indexOf("%0A") == (resultado.length - 3)) {
					resultado = resultado.substring(0,resultado.length-3);
				}
			} catch (error) {
				resultado = '';
			}
		}, error: function(requestObject, error, errorThrown){
			resultado = '';
		},
		async:false
	});
	return resultado;		  
}

function pad(str, max) { //completa el tamanio a enviar a nnigma
	str = str.toString();
	return str.length < max ? pad(str+"*" , max) : str;
}

function leftpad(str, len, ch) {
	str = String(str);

	var i = -1;

	if (!ch && ch !== 0)
		ch = ' ';

	len = len - str.length;
	while (++i < len) {
		str = ch + str;
	}
	return str;
}

function modificarDatosIngreso(idCampo) {

	var activateEncryptLastEmpresa = document.getElementById("activateEncryptLastEmpresa").value;
	var activateEncryptLastUsuario = document.getElementById("activateEncryptLastUsuario").value;
	
	if (idCampo == 'empresa') {
		document.getElementById("activateEncryptLastEmpresa").value = "N";
		if (activateEncryptLastEmpresa == "Y") {
			document.getElementById("empresa").value = "";
		}
	}
	
	if (idCampo == 'usuario') {
		document.getElementById("activateEncryptLastUsuario").value = "N";
		if(activateEncryptLastUsuario == "Y") {
			document.getElementById("usuario").value = "";
		}
	}
}
