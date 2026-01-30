function getCookie (nombreCookie) {
	var start = document.cookie.indexOf(nombreCookie+"=");
	var len = start+nombreCookie.length+1;

	if ((!start) && (nombreCookie != document.cookie.substring(0,nombreCookie.length))) 
		return null;
	if (start == -1) 
		return null;

	var end = document.cookie.indexOf(";",len);
	if (end == -1) 
		end = document.cookie.length;
	return unescape(document.cookie.substring(len,end));
}

function setCookie (nombreCookie, valorCookie) {
	var expdate = new Date();
	expdate.setTime (expdate.getTime() + (1000 * 60 * 60 * 24 * 1000));
	document.cookie = nombreCookie + "=" + escape (valorCookie) + "; expires=" + expdate.toGMTString() +  "; path=/"; 
}

function DeleteCookie (nombreCookie) {
	var exp = new Date();
	exp.setTime (exp.getTime() - 1);
	var cval = getCookie (nombreCookie);
	if (cval != null)
		document.cookie = nombreCookie + "=" + cval + "; expires=" + exp.toGMTString();
}
