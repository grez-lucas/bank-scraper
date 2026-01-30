	// config to validate
	const _emailRegex = /^[A-Za-z0-9._-]{6,}@[A-Za-z0-9-]+\.[A-Za-z]{2,3}(\.[A-Za-z]{2,3})?$/,
	_cantMaxElementosConsecIguales = 3,
	_minLengthPwd = 8,
	_maxLengthPwd = 36,
	_pwdLevel = {
		veryLow: {
			text: "muy bajo",
			class: "very-low"
		},
		low: {
			text: "bajo",
			class: "low"
		},
		medium: {
			text: "medio",
			class: "medium"
		},
		high: {
			text: "alto",
			class: "high"
		},
		veryHigh: {
			text: "muy Alto",
			class: "very-high"
		}
	},
	// URL Have I Been Pwned API to check if the value hash appears in any data breaches
	_PWNED_RANGE_API_URL_PWD = 'https://api.pwnedpasswords.com/range/',
	_ERROR_MESSAGE_PWD = {
		FORMAT: "Tu contrase\u00f1a debe cumplir con las 3 indicaciones mostradas. Revisa y vuelve a intentarlo.",
		SERIES: "No incluyas m\u00e1s de " + _cantMaxElementosConsecIguales + " caracteres consecutivos o iguales. Revisa y vuelve a intentarlo.",
		DOCUMENT: "Tu nueva contrase\u00f1a no puede ser el n\u00famero de tu documento de identidad.",
		PWD_NOT_EQUAL: "Las contrase\u00f1as no coinciden.",
		EQUAL_TO_OLD: "Es necesario que tu nueva contrase\u00f1a sea distinta de la temporal.",
		EMAIL_FORMAT: "Necesitas ingresar un correo v\u00E1lido.",
	};
	
	function initializeCoronitaIcons() {
		onClickPasswordIconVisibility();
		onClickClearIcon();
		toogleIconsOnEvent();
		$(document).trigger('capgemini.wizard.resize');
	}

	function onClickPasswordIconVisibility() {
		const toggleButtons = document.querySelectorAll(".icon-password");
		toggleButtons.forEach((toggleButton) => {
			toggleButton.addEventListener("mousedown", (event) => {
				event.preventDefault();
				const passwordField = toggleButton.parentNode.querySelector("input");
				const type = passwordField.getAttribute("type") === "password" ? "text" : "password";
				passwordField.setAttribute("type", type);
				toggleButton.classList.toggle("close-eye");
				if(toggleButton.classList.contains("open-eye-red")||toggleButton.classList.contains("close-eye-red")){
					toggleButton.classList.toggle("close-eye-red");
					toggleButton.classList.toggle("open-eye-red");
				}
			});
		});
	}

	function onClickClearIcon() {
		const clearButtons = document.querySelectorAll(".icon-limpiar-input, .icon-limpiar-input-alone");
		clearButtons.forEach((clearButton) => {
			clearButton.addEventListener("mouseup", () => {
				const input = clearButton.parentNode.querySelector(".coronita-input");
				input.value = "";
				input.focus();
				toogleCleanIcon(input);
				hideErrorMessage(input);
				const rulesContainer = clearButton.parentNode.querySelector(".marks-container");
				if(rulesContainer!=null){
					cleanMarkedRules();
					resetPwdLevel();
					$(document).trigger('capgemini.wizard.resize');
				}
			});
			clearButton.addEventListener("mousedown", (event) => {
				event.preventDefault();
			});
		});
	}

	function toogleIconsOnEvent() {
		const inputFields = document.querySelectorAll(".coronita-input");
		inputFields.forEach((input) => {
			
			input.addEventListener("keyup", () => {
				// input.value = input.value.trim();
				toogleCleanIcon(input);
			});
			
			input.addEventListener("focus", () => {
				const iconEye = input.parentNode.querySelector(".icon-password");
				if (iconEye !== null) iconEye.style.display = 'block';
			});
			
			input.addEventListener("focusout", () => {
				toogleEyeIcon(input);
				toogleCleanIcon(input);
			});
			
			// input.addEventListener("keydown", (e) => {
			// 	if(e.keyCode === 32 || e.keyCode === 229 || e.key === " "){
			// 		e.preventDefault();
			// 		return false;
			// 	}
			// });
			
		});
	}

	function toogleCleanIcon(inputSelected) {
		const icons = inputSelected?.parentNode.querySelectorAll(".icon-limpiar-input, .icon-limpiar-input-alone");
		icons?.forEach((icon) => {
			icon.style.display = inputSelected.value.length <= 0 ? 'none' : 'block';;
		});
		reglaPass();
	}
	
	function toogleEyeIcon(inputSelected) {
		const icons = inputSelected?.parentNode.querySelectorAll(".icon-password");
		icons?.forEach((icon) => {
			icon.style.display = inputSelected.value.length <= 0 ? 'none' : 'block';;
		});
	}

	function initializePasswordRules() {
		const pwdInput = getElementBySelector(".clave-internet-input");
	
		pwdInput?.addEventListener("keyup", () => {
			const pwdInputValue = getValueFromElement(pwdInput);
			pwdInputValue.length > 0 ? markRules(pwdInput.value) : cleanMarkedRules();
		});

		pwdInput?.addEventListener("focusout", () => {
			const pwdInputValue = getValueFromElement(pwdInput);
			if (pwdInputValue.length > 0) {
				const error = getBasicError(pwdInputValue);
				if (error.hasError) {
					$(".marks-container").hide();
					showErrorMessage(pwdInput, error.message);
					resetPwdLevel();
				}else{
					analyzePwdLevel(pwdInputValue);
					pwdInput.classList.add("color-changed"); /** */
				}
				$(document).trigger('capgemini.wizard.resize');
			} else {
				pwdInput.classList.remove("color-changed"); /** */
			}
		});

		pwdInput?.addEventListener("focus", () => {
			const pwdInputValue = getValueFromElement(pwdInput);
			hideErrorMessage(pwdInput);
			pwdInput.classList.add("color-changed"); /** */
			$(".marks-container").show();
			if(pwdInputValue.length <= 0){
				cleanMarkedRules();
				resetPwdLevel();
			}
		});
	}


	function checkPWdConfirmation() {
		const pwdConfirmationInput = obtenerElementoHtmlPorId("claveInternetConfirmada");
		pwdConfirmationInput?.addEventListener("focusout", () => {
			if (pwdConfirmationInput.value.length > 0) {
				let pwdInput = document.querySelector("#claveInternet");
				if (pwdConfirmationInput.value !== pwdInput.value) {
					showErrorMessage(pwdConfirmationInput, _ERROR_MESSAGE_PWD.PWD_NOT_EQUAL)
				}else{
					hideErrorMessage(pwdConfirmationInput);
				};
			} else {
				pwdConfirmationInput.classList.remove("color-changed"); /** */
			}
		});
		
		pwdConfirmationInput?.addEventListener("focus", () => {
			pwdConfirmationInput.classList.add("color-changed"); /** */
			if (pwdConfirmationInput.value.length > 0) {
					hideErrorMessage(pwdConfirmationInput);
			}
		});
	}
	
	function emailFormatInitializer() {
		const emailFields = document.querySelectorAll(".email");
		emailFields.forEach((emailInput) => {
			emailInput?.addEventListener("focusout", () => {
				if (emailInput.value.length > 0) {
					if (!checkEmailFormat(emailInput.value)) {
						showErrorMessage(emailInput, _ERROR_MESSAGE_PWD.EMAIL_FORMAT)
					}else{
						hideErrorMessage(emailInput);
					};
				}
			});
			
			emailInput?.addEventListener("focus", () => {
				if (emailInput.value.length > 0) {
						hideErrorMessage(emailInput);
				}
			});
		});
	}
	
	function obtenerElementoHtmlPorId(id) {
		return document.getElementById(id);
	}
	
	function getValueFromElement(element){
		return element?.value;
	}
	
	function getElementBySelector(selector){
		return document.querySelector(selector);
	}
	
	function hideErrorMessage(element){
		element.classList.remove("validation-invalid");
		const labelError = element?.parentNode.querySelector(".validation-error");
		if (labelError !== null){
			labelError.remove();
		}
		cleanOverLayErrorOnIcon(element);
	}

	function resetPwdLevel() {
		$("#nivelClave").text(_pwdLevel.veryLow)
		$("#nivelContra").removeClass().addClass("area-nivel-seguridad flag-" + _pwdLevel.veryLow.class).hide();
		$("#pwdWarning").hide();
	}

	function setPwdLevel(level) {
		$("#nivelClave").text(level.text)
		$("#nivelContra").removeClass().addClass("area-nivel-seguridad flag-" + level.class).show();
	}

	function almenosUnaLetraMinuscula(dato) {
		return /[a-z]+/.test(dato);
	}

	function almenosUnaLetraMayuscula(dato) {
		return /[A-Z]+/.test(dato);
	}

	function almenosUnNumero(dato) {
		return /\d+/.test(dato);
	}
	
	function hasCorrectFormat(dato){
		return /^[a-zA-Z0-9!@#$%^&*()_+\-\~=\[\]{};`':"\\|,.<>\/?]{8,36}$/.test(dato);
	}

	function chekRepetitiveCaracteres(str) {
		
		// Validar letras repetidas y numeros repetidos
		const letrasRepetidas = str.match(/([a-zA-Z0-9])\1+/g);
		if (letrasRepetidas && letrasRepetidas.some(repeticion => repeticion.length > _cantMaxElementosConsecIguales)) {
			return true;
		}

		return false;
	}

	function checkConsecutiveCaracteres(str) {
		
		 const regex = /[a-zA-Z0-9]/;
		 let count = 1;

		  for (let i = 0; i < str.length - 1; i++) {
		   
		    const currChar = str[i];
		    const nextChar = str[i + 1];
		
		    const isCurrValid = regex.test(currChar);
		    const isNextValid = regex.test(nextChar);
		
		    if (isCurrValid && isNextValid && Math.abs(currChar.charCodeAt(0) - nextChar.charCodeAt(0)) === 1) {
		      count++;
		      if (count > _cantMaxElementosConsecIguales) {
		        return true;
		      }
		    } else {
		      count = 1;
		    }
		  }

 		return false;
	}

	function cleanMarkedRules() {
		$(".mark").removeClass("marked-check").addClass("unmarked");
	}

	function checkRule(ruleIndex) {
		$("#check-" + ruleIndex).removeClass("unmarked").addClass("marked-check");
	}

	function unCheckRule(ruleIndex) {
		$("#check-" + ruleIndex).removeClass("marked-check").addClass("unmarked");
	}

	function markRules(value) {
		// Debe incluir Almenos una letra minuscula y una minuscula
		almenosUnaLetraMinuscula(value) && almenosUnaLetraMayuscula(value) ? checkRule(1) : unCheckRule(1);

		// Debe incluir Almenos un Numero
		almenosUnNumero(value) ? checkRule(2) : unCheckRule(2);

		// Debe tener el minimo y maximo en longitud
		checkMinMaxLengthPwd(value) ? checkRule(3) : unCheckRule(3);
	}

	function checkMinMaxLengthPwd(pwdText) {
		return pwdText.length >= _minLengthPwd && pwdText.length <= _maxLengthPwd;
	}

	function existeConsecutividadOigualdad(value) {
		return checkConsecutiveCaracteres(value) || chekRepetitiveCaracteres(value);
	}
	
	// Retorna un objeto de tipo error y el mensaje correspondiente
	function getBasicError(value) {
		
		const oldPWD = document.querySelector(".old-pwd");
		
		//only if exist oldPWd
		if(oldPWD?.value === value){
			return {
				hasError: true,
				message: _ERROR_MESSAGE_PWD.EQUAL_TO_OLD
			};
		}
		else if(!hasCorrectFormat(value)){
			return {
				hasError: true,
				message: _ERROR_MESSAGE_PWD.FORMAT
			};
		}
		else if (!(almenosUnaLetraMinuscula(value) &&
				 	almenosUnaLetraMayuscula(value) && 
				 		almenosUnNumero(value) && 
				 			checkMinMaxLengthPwd(value))) {
			return {
				hasError: true,
				message: _ERROR_MESSAGE_PWD.FORMAT
			};
		}
		else if (existeConsecutividadOigualdad(value)) {
			return {
				hasError: true,
				message: _ERROR_MESSAGE_PWD.SERIES
			};
		}
		else if (existeAliasEnPwd(value)) {
			return {
				hasError: true,
				message: _ERROR_MESSAGE_PWD.DOCUMENT
			};
		} else {
			return {
				hasError: false,
				message: ""
			};
		}
	}

	function existeAliasEnPwd(pwd) {
		let alias = $("#alias").val();
		return alias?.length > 0 ? pwd.search(alias.substring(1)) !== -1: false;
	}

	function checkEmailConfirmation() {
		let emailA = $('#email').val().toLowerCase();
		let emailB = $('#emailConfirmado').val().toLowerCase();
		return emailA == emailB;
	}

	function showErrorMessage(inputElement, errorMessage) {
		const hasError = inputElement?.parentNode.querySelector(".validation-error");
		if(hasError === null){
			inputElement.classList.remove("color-changed"); /** */
			inputElement.classList.add("validation-invalid");
			let divElement = document.createElement('div');
			divElement.classList.add("validation-error");
			divElement.textContent = errorMessage;
			inputElement.after(divElement);
			overLayErroOnIcon(inputElement);
		}

	}
	
	function overLayErroOnIcon(input){
		const icons = input?.parentNode.querySelectorAll(".icon-limpiar-input, .icon-limpiar-input-alone, .icon-password");
		icons?.forEach((icon) => {
			icon.classList.add("filter-error-svg");
		});
		input?.classList.remove("color-changed"); /** */
		input?.classList.add("validation-invalid");/** */
	}
	
	function cleanOverLayErrorOnIcon(input){
		const icons = input?.parentNode.querySelectorAll(".icon-limpiar-input, .icon-limpiar-input-alone, .icon-password");
		icons?.forEach((icon) => {
			icon.classList.remove("filter-error-svg");
		});
	}

	function checkEmailFormat(email) {
		return _emailRegex.test(email);
	}
	
	async function sha1(message) {
		const msgBuffer = new TextEncoder().encode(message);
		const hashBuffer = await crypto.subtle.digest('SHA-1', msgBuffer);
		const hashArray = Array.from(new Uint8Array(hashBuffer));
		const hashHex = hashArray.map(b => b.toString(16).padStart(2, '0')).join('');
		return hashHex.toUpperCase();
	}

	async function hasBeenPwned(password, timeout = 1000) {
		// Hash the value using SHA-1 algorithm
		const sha1Password = await sha1(password);

		// Split the hash into prefix and suffix
		const prefix = sha1Password.substring(0, 5);
		const suffix = sha1Password.substring(5);
		
		// Wait for the API response or timeout, whichever comes first
		const response = await fetchWithTimeout(_PWNED_RANGE_API_URL_PWD + prefix, {}, timeout);

		// Check the response for the suffix of the value hash
		const text = await response.text();
		for (let line of text.split('\n')) {
			let parts = line.split(":");
			if (parts[0] === suffix) {
				// If the value hash suffix is found, return true
				return true;
			}
		}
		// If the value hash suffix is not found, return false
		return false;
	}

	function fetchWithTimeout(url, options, timeout) {
		return new Promise((resolve, reject) => {
			fetch(url, options).then(resolve, reject);
			if (timeout) {
				const e = new Error("Connection timed out");
				setTimeout(reject, timeout, e);
			}
		});
	}

	async function analyzePwdLevel(password) {
		try {
			const isCompromised = await hasBeenPwned(password);
			if (isCompromised) {
				setPwdLevel(_pwdLevel.veryLow);
				$("#pwdWarning").show();
				document.getElementById('pwdWarning').scrollIntoView();
			} else {
				$("#pwdWarning").hide();
				setPwdLevel(getLevelusingZXCVBN(password));
			}
		} catch (error) {
			console.log("Error determinando el nivel de seguridad", error);
		}
	}
	
	function getLevelusingZXCVBN(pwd) {
		const result = zxcvbn(pwd);
		const score = result.score;
		switch (true) {
  			case score <=  1 : return _pwdLevel.low;
  			case score === 2 : return _pwdLevel.medium;
  			case score >=  3 : return _pwdLevel.high;
    		default: return _pwdLevel.veryLow;
		}
	}
	
	function checkCountDigitsCards() {
		let numCard = $('#numeroTarjeta').val();
		return numCard.length == 16;
	}
	
	function checkCountDigitsCajero() {
		let numCajero = $('#claveCajero').val();
		return numCajero.length == 4;
	}
	
	function initializeInputClaveTemporal() {
		const pwdInputTemp = getElementBySelector(".old-pwd");

		pwdInputTemp?.addEventListener("focusout", () => {
			const pwdInputValue = getValueFromElement(pwdInputTemp);
			if (pwdInputValue.length > 0) {
				pwdInputTemp.classList.add("color-changed"); /** */
			} else {
				pwdInputTemp.classList.remove("color-changed"); /** */
			}
		});

		pwdInputTemp?.addEventListener("focus", () => {
			pwdInputTemp.classList.add("color-changed"); /** */
		});
	}