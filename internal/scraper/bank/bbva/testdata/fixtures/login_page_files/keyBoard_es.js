var cantidad = 0;
var tv = false;
var tv_form = null;
var tv_prefijoUri     = '';
var tv_versionTeclado = '1.1';
var tv_idLayerTeclado = 'lteclado';

var tv_tabIndex = 0;
var tv_campoDefault = null;
var tv_campoSeleccionado = null;
var tv_campoSeleccionadoTipo = 'a0';
var tv_campoSeleccionadoMaxLen = 999;

var tv_layerTeclado = null;

var tv_habListaCampos   = true;
var tv_habTecladoNormal = false;
var tv_habNumerosRandom = true;

var tv_tecladoNormalSiempreHabilitado = false;

var tv_margenSup = 0;
var tv_margenDer = 260;

var tv_posIzq = 10;
var tv_posSup = 150;

var tv_camposTab = new Array();

var tv_isMSIE = navigator.userAgent.toLowerCase().indexOf( "msie" ) >= 0;
var tv_isOpera = navigator.userAgent.toLowerCase().indexOf( "opera" ) >= 0;
var tv_browserVersion = navigator.appVersion.substring( 0, 1 );

function tvEventoTeclaPresionada( e ) {
	if (tv_isMSIE) {
		if ( event.keyCode == 13 ) {
			tvEnter();
		} else if (event.keyCode == 9)
			return true;
	} else {
		if (eval('e.which == 13'))
			tvEnter();
		else if(eval('e.which == 9'))
			return true;
	}
	return( tv_habTecladoNormal );
}

function tvDisableRightClick( e ) {return true;
	if (document.layers)
		return(e.which != 3);
	else
		return false;
}

function tvInit() {
	tv_form = document.forms[ 0 ];
	tv_document = document;
	tv_layerTeclado = eval( obj1 + tv_idLayerTeclado + obj2 );

	OGG = eval(obj1 + tv_idLayerTeclado + obj2 + style);

	if (tv_campoDefault == null)
		tvSetCampo(tvObtenerCampoDefault());

	if (tv_isMSIE || document.getElementById) {
		document.oncontextmenu = tvDisableRightClick;
	} else {
		if (!tv_tecladoNormalSiempreHabilitado) {
			window.onkeypress = tvEventoTeclaPresionada;
			window.captureEvents( Event.KEYPRESS );
		}
	}
}

function tvOpen() {
	try {
		if(!havePasswordFields()) return;
	} catch (e) {}
	tvShowHide('visible');
	tv = true;
}

function tvClose() {
	tvShowHide( 'hidden' );
	tv = false;
	var obj = eval( tv_camposTab[ 0 ] );
	try{obj.focus();}catch(e){}
}

function tvObtenerCampoDefault() {
	var c = null;
	if ( tv_camposTab.length > 0)
		c = eval( tv_camposTab[ tv_tabIndex ] );
	else {
		for(var i = 0; i < tv_form.elements.length; i++) {
			var obj = tv_form.elements[i];
			if (obj.type == 'text' || obj.type == 'textarea' || obj.type == 'password') {
				c = obj;
				break;
			}
		}
	}
	return c;
}

function tvSetCampo( c, cml, ct, cid ) {
	if( c != null ) {
		if(c.type == 'text' || c.type == 'textarea' || c.type == 'password') {
			tv_campoSeleccionado = c;
		if(cml != null)
			tv_campoSeleccionadoMaxLen = cml;
		if(ct != null)
			tv_campoSeleccionadoTipo = ct;
		if(cid != null)
			tv_tabIndex = cid;
		if(tv_isMSIE || tv_isOpera || tv_browserVersion != '4')
			c.focus();
		}
	}
}

function tvLimpiaCampos() {
	for( var i = 0; i < tv_camposTab.length; i++ ) {
		var obj = eval(tv_camposTab[i]);
		if(obj) obj.value = '';
	}
}

function tvShowHide(action){
	var ret = tvShowHideGenerico( action, tv_idLayerTeclado );
	tv_habTecladoNormal = !ret;
}

function tvShowHideGenerico( action, layerName ) {
  OGG = eval( obj1 + layerName + obj2 + style );
  OGG.visibility = action;
  return( OGG.visibility == 'visible' || OGG.visibility == 'show' );
}

function tvOcultarTecla( c ) {
  if( !document.layers ) {
      var obj = tblGetTD(eval( "document.getElementById( 'tecla_" + c + "' )" ));
      if( obj ) obj.style.backgroundColor = "#F5F5F5";
      if( obj ) obj.bgColor = "#F5F5F5";
  }
  return;
}

function tvMostrarTecla( c ) {
  if( !document.layers ) {
      var obj = tblGetTD(eval( "document.getElementById( 'tecla_" + c + "' )" ));
      if( obj ) obj.style.backgroundColor = "#AFCBE2";
      if( obj ) obj.bgColor = "#AFCBE2";
  }
  return;
}

function tblGetTD(oElem) {
	while (oElem){
		if (oElem.tagName.toLowerCase() == "td" && oElem.parentElement.tagName.toLowerCase() == "tr") 
			return oElem;
		if (oElem.tagName.toLowerCase() == "table") return null;
		oElem = oElem.parentElement;
	}
}

function tvTipear(tecla) {
  var c = ( tv_campoSeleccionado != null ? tv_campoSeleccionado : tv_campoDefault );
  var sel = document.selection; 
  sel.clear(); 

   if( !document.layers ) {
      var obj = tblGetTD(eval( "document.getElementById( 'tecla_" + tecla + "' )" ));
      if( obj ) obj.style.backgroundColor = "#DD5500";
      if( obj ) obj.bgColor = "#DD5500";
  }
  
  if( tecla == '_ENTER' ){
      tvEnter();
  }else if( tecla == 'OK' ) {
           tv_tabIndex++;
    	   if( tv_tabIndex == tv_camposTab.length ) tv_tabIndex = 0;
           var cc = eval( tv_camposTab[ tv_tabIndex ] );
           if( cc ) tvSetCampo( cc );
  }else if (tecla == 'KO'){
  	 c.value = "";
     c.focus();
  }else if (tecla == 'SP'){
  	c.value = c.value + " ";
  	c.focus();
  }else if (tecla == 'Mm'){
  	
  	if (c.value == c.value.toUpperCase()){
  		cantidad=1;
  	}else if (c.value == c.value.toLowerCase()){
  		cantidad=0;
  	}	
  		
  	if (cantidad%2==0){
  			c.value = c.value.toUpperCase();
  	}else{
  			c.value = c.value.toLowerCase();
  	}
  	cantidad++;
  	c.focus();	
  }else {
      if( c != null ) {
          if( tecla == '_CLEAR' ) {
              c.value = "";
              c.focus();
          }
          else if( tecla == 'BACKSPACE' ) {
              if( c.value.length > 0 )
                  c.value = c.value.substring( 0, c.value.length - 1 );
              c.focus();
          }
          else {
              if( c.value.length < tv_campoSeleccionadoMaxLen ) {
                  var tv_selCharList = eval( 'tv_charlist_' + tv_campoSeleccionadoTipo );
                  if( tv_selCharList.indexOf( tecla ) >= 0 ) {
                      c.value += tecla;
                      c.focus();
                  }
                  else c.focus();
              }
          }
      }
      else alert( "por favor, haga click primero en el campo a completar" );
  }

  window.status = ' ';
  return;
}

function Cambia_Imagen(identificador, imagen)
{
	alert("identificador:: "+identificador+" imagen:: "+imagen);
	identificador.src = imagen;
}

function tvEscribirTeclaHtml( c) {
  s = new String( c );
 	document.write( ' <td colspan=2 height="24" class="key">' + ( s == '' ? '&nbsp;' : '<a href="javascript:void(0)" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'' + s + '\' ); return false;"' : '' ) + ' onclick="tvTipear( \'' + s + '\' ); return false;" name="tecla_' + s + '" id="tecla_' + s + '" onfocus="if( this.blur ) this.blur(); return false;" onMouseOver="tvOcultarTecla( \'' + s + '\'); return false;" onMouseOut="tvMostrarTecla( \'' + s + '\'); return false;" nowrap>&nbsp;' + s + '</a>' ) + '</td>' ) ;
}

function tvWriteLayerStyle() {
  document.write( '<style type="text/css">' );
  document.write( '#' + tv_idLayerTeclado + ' td.key {' );
  document.write( '    background-image:url(\'' + tv_prefijoUri + 'img/key_normal_es.gif\');' );
  document.write( '    background-repeat:no-repeat;' );
  document.write( '    background-position:center;' );
  document.write( '    height:35px;' );
  document.write( '    align:center;' );
  document.write( '    text-align:center;' );
  document.write( '    text-decoration:none;' );
  document.write( '    vertical-align:middle;' );
  document.write( '    background-color: #AFCBE2;' );
  document.write( '    font-weight:bold;' );
  document.write( '    font-family: Arial; }' );
  document.write( '#' + tv_idLayerTeclado + ' td.filler {' );
  document.write( '    height:35px;' );
  document.write( '    align:center;' );
  document.write( '    text-align:center;' );
  document.write( '    text-decoration:none;' );
  document.write( '    vertical-align:middle;' );
  document.write( '    background-color: #EBEBEB; }' );
  document.write( '#' + tv_idLayerTeclado + ' td.keyback {' );
  document.write( '    background-repeat:no-repeat;' );
  document.write( '    background-position:center;' );
  document.write( '    height:35px;' );
  document.write( '    text-align:center;' );
  document.write( '    text-decoration:none;' );
  document.write( '    vertical-align:middle;' );
  document.write( '    align:center;' );
  document.write( '    background-color: #AFCBE2;' );
  document.write( '    font-weight:bold; }' );
  document.write( '#' + tv_idLayerTeclado + ' a,a.hover,a.visited { text-decoration:none;color:#939393;font-size:16px; }' );
  document.write( '</style>' );
}

function tvWriteLayer() {
  tvWriteLayerStyle();
  document.write( '<div  name="' + tv_idLayerTeclado + '" id="' + tv_idLayerTeclado + '" onselectstart="return false" style="layer-background-color: #E3E3EF; position:absolute; left:' + tv_posIzq + '; top:' + tv_posSup + '; width:450px; height:140px; visibility:hidden; overflow:visible; z-index:300;">' );
  document.write( '  <CENTER>' );
  
		  document.write( '  <table border=0 class="xkBoard" style="TABLE-LAYOUT:fixed;" cols=28 cellpadding=0 cellspacing=0 ' +  '>' );
		  document.write( '  <COLGROUP>' );
		  document.write( '  <COL ALIGN=CENTER width="10px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="20px">' );
		  document.write( '  <COL ALIGN=CENTER width="10px">' );
		  document.write( '  </COLGROUP>' );

		  document.write( '  <THEAD>' );
		  document.write( '  </THEAD>' );
		  document.write( '  <TBODY BGCOLOR="#EBEBEB">' );
		  document.write( '    <tr height="30px">' );
		  document.write( '    	 <td colspan=1 BGCOLOR="#EBEBEB">&nbsp;</TD>' );
		  document.write( '      <td colspan=2 BGCOLOR="#EBEBEB" ALIGN="LEFT"><a href="#" onClick="return false" onMouseDown="engager( event, \'' + tv_idLayerTeclado + '\' );return false;"><img src=' + tv_prefijoUri + 'img/move_es.gif border=0 alt="Desplazar"></a></td>' );
		  document.write( '      <td colspan=23 BGCOLOR="#EBEBEB" ALIGN="RIGHT">&nbsp;</td>' );
		  document.write( '    </tr>' );
		  document.write( '    <tr>' );
		  document.write( '      <td colspan=26 BGCOLOR="#EBEBEB"><img src="' + tv_prefijoUri + 'img/spacer_es.gif" height=4></td>' );
		  document.write( '    </tr>' );

		  document.write( '    <tr> ' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  tvEscribirTeclaHtml( '1' );
		  tvEscribirTeclaHtml( '2' );
		  tvEscribirTeclaHtml( '3' );
		  tvEscribirTeclaHtml( '4' );
		  tvEscribirTeclaHtml( '5' );
		  tvEscribirTeclaHtml( '6' );
		  tvEscribirTeclaHtml( '7' );
		  tvEscribirTeclaHtml( '8' );
		  tvEscribirTeclaHtml( '9' );
		  tvEscribirTeclaHtml( '0' );
		  document.write( '      <td colspan=4 class="keyback" BGCOLOR="#AFCBE2"><a name="tecla_BACKSPACE" onMouseOver="tvOcultarTecla( \'BACKSPACE\'); return true;" onMouseOut="tvMostrarTecla( \'BACKSPACE\'); return false;" href="javascript:tvTipear( \'BACKSPACE\');" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'BACKSPACE\');"' : '' ) + '><img src=' + tv_prefijoUri + 'img/key_back_es.gif border=0></a></td>' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  document.write( '    </tr>' );

		  document.write( '    <tr>' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  tvEscribirTeclaHtml( 'Q' );
		  tvEscribirTeclaHtml( 'W' );
		  tvEscribirTeclaHtml( 'E' );
		  tvEscribirTeclaHtml( 'R' );
		  tvEscribirTeclaHtml( 'T' );
		  tvEscribirTeclaHtml( 'Y' );
		  tvEscribirTeclaHtml( 'U' );
		  tvEscribirTeclaHtml( 'I' );
		  tvEscribirTeclaHtml( 'O' );
		  tvEscribirTeclaHtml( 'P' );
			
			document.write( '      <td colspan=4 BGCOLOR="#AFCBE2"><a name="tecla_KO" onMouseOver="tvOcultarTecla( \'KO\'); return true;" onMouseOut="tvMostrarTecla( \'KO\'); return false;" href="javascript:tvTipear( \'KO\');" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'KO\');"' : '' ) + '><img src=' + tv_prefijoUri + 'img/key_ko_es.gif border=0></a></TD>' );		 
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  document.write( '     </tr>' );

		  document.write( '     <tr>' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  tvEscribirTeclaHtml( 'A' );
		  tvEscribirTeclaHtml( 'S' );
		  tvEscribirTeclaHtml( 'D' );
		  tvEscribirTeclaHtml( 'F' );
		  tvEscribirTeclaHtml( 'G' );
		  tvEscribirTeclaHtml( 'H' );
		  tvEscribirTeclaHtml( 'J' );
		  tvEscribirTeclaHtml( 'K' );
		  tvEscribirTeclaHtml( 'L' );
		  tvEscribirTeclaHtml( '�' );
		  document.write( '      <td colspan=4 BGCOLOR="#AFCBE2"><a name="tecla_OK" onMouseOver="tvOcultarTecla( \'OK\'); return true;" onMouseOut="tvMostrarTecla( \'OK\'); return false;" href="javascript:tvTipear( \'OK\');" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'OK\');"' : '' ) + '><img src=' + tv_prefijoUri + 'img/key_ok_es.gif border=0></a></TD>' );		 
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  document.write( '     </tr>' );

		  document.write( '     <tr>' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  document.write( '      <td colspan=2 BGCOLOR="#AFCBE2"><a name="tecla_Mm" onMouseOver="tvOcultarTecla( \'Mm\'); return true;" onMouseOut="tvMostrarTecla( \'Mm\'); return false;" href="javascript:tvTipear( \'Mm\' );" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'Mm\' );"' : '' ) + '><img src=' + tv_prefijoUri + 'img/key_block_es.gif border=0></a></TD>' );
		  tvEscribirTeclaHtml( 'Z' );
		  tvEscribirTeclaHtml( 'X' );
		  tvEscribirTeclaHtml( 'C' );
		  tvEscribirTeclaHtml( 'V' );
		  tvEscribirTeclaHtml( 'B' );
		  tvEscribirTeclaHtml( 'N' );
		  tvEscribirTeclaHtml( 'M' );
		  document.write( '      <td colspan=8 BGCOLOR="#AFCBE2"><a name="tecla_SP" onMouseOver="tvOcultarTecla( \'SP\'); return true;" onMouseOut="tvMostrarTecla( \'SP\'); return false;" href="javascript:tvTipear( \'SP\' );" ' + ( ( tv_isMSIE && !tv_isOpera ) ? 'ondblclick="tvTipear( \'SP\');"' : '' ) + '><img src=' + tv_prefijoUri + 'img/key_big_es.gif border=0></a></TD>' );
		  document.write( '      <td colspan=1 class="filler">&nbsp;</TD>' );
		  document.write( '    </tr>' );
			document.write( '     <tr height="5px">' );
		  document.write( '      <td colspan=26 BGCOLOR="#EBEBEB">&nbsp;</TD>' );
		  document.write( '    </tr>' );
		  document.write( '  </TBODY>' );
		  document.write( '  </table>' );

  
  document.write( '  </div>' );
}

// -----------------------------------------------------------------
var engaged = false;
var obj1, obj2, style, eX, eY, offsetX, offsetY;
var currentOffsetX, currentOffsetY;
var OGG, OGGhlp;
var engagedZindex = 300;
var differL, differT;

if( document.getElementById ) {
    obj1 = "document.getElementById('";
    obj2 = "')";
    style = ".style";
    eX = ( navigator.appName.indexOf( "Internet Explorer" ) == -1 ) ? "e.clientX" : "event.clientX";
    eY = ( navigator.appName.indexOf( "Internet Explorer" ) == -1 ) ? "e.clientY" : "event.clientY";

    offsetX = ( navigator.appName.indexOf( "Internet Explorer" ) == -1 ) ? "pageXOffset" : "document.body.scrollLeft";
    offsetY = ( navigator.appName.indexOf( "Internet Explorer" ) == -1 ) ? "pageYOffset" : "document.body.scrollTop";
} else if( document.all ) {
    obj1 = "document.all['";
    obj2 = "']";
    style = ".style";
    eX = "event.clientX";
    eY = "event.clientY";

    offsetX = "document.body.scrollLeft";
    offsetY = "document.body.scrollTop";
}
else if( document.layers ) {

    obj1 = "document.layers['";
    obj2 = "']";
    style = "";
    eX = "e.pageX";
    eY = "e.pageY";

    offsetX = "pageXOffset";
    offsetY = "pageYOffset";
    document.captureEvents( Event.MOUSEMOVE );
}

function engager( e, layername ) {
  engaged = ( !engaged ) ? layername : false;
  if( engaged ) {
      OGG = eval( obj1 + engaged + obj2 + style );
      currentOffsetX = ( document.layers ) ? 0 : eval( offsetX );
      currentOffsetY = ( document.layers ) ? 0 : eval( offsetY );
      engagedZindex = OGG.zIndex;
      OGG.zIndex = 300;
      var eXin = eval( eX );
      var eYin = eval( eY );
      differL = ( eXin + currentOffsetX ) - parseFloat( OGG.left );
      differT = ( eYin + currentOffsetY ) - parseFloat( OGG.top );
      document.onmousemove = dragLayerByCorner;
      return;
  }

  OGG.zIndex = engagedZindex;
  document.onmousemove = null;
}

function dragLayerByCorner( e ) {
		window.status=OGG.top + ' ' + OGG.left;

  if( !engaged ) { return true; }

  var eXin = eval( eX );
  var eYin = eval( eY );

  var tmpTop  = ( eYin + currentOffsetY ) - differT;
  var tmpLeft = ( eXin + currentOffsetX ) - differL;

  if( !document.layers || ( document.layers && tmpTop > tv_margenSup ) ) {
      if( tmpTop > 0 ) {
          OGG.top = tmpTop;
	  }
  }

  if( !document.layers || ( document.layers && tmpLeft <= tv_margenDer ) ) {
       if( tmpLeft > 0 ) {
           OGG.left = tmpLeft;
	   }
  }
}
// ------------------------------------------------------