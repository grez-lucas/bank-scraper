# Bank Scraper Platform PRD

Author: Lucas Grez
Created time: December 17, 2025 10:45 AM
Last edited by: Lucas Grez
Last updated time: February 3, 2026 5:23 PM

# 1. Introducción

## 1.1. Planteamiento del Problema

La plataforma de cambio de divisas AyniFX necesita acceder de manera programatica informacion de cuentas bancarias corporativas (saldos y movimientos) de multiples bancos peruanos (BBVA, Interbank, BCP) para facilitar operaciones. Actualmente, la unica manera unificada para consultar esta información son soluciones como PrometeoAPI o GMoney. Consultar esta informacion manualmente en cada portal bancario y luego actualizarla en el sistema es ineficiente y propenso a errores.

## 1.2 Solución

La solución consiste en desarrollar una plataforma interna que consiga esta información mediante scraping y pueda abstraer la complejidad de interactuar con cada una de las entidades bancarias mediante una única y consistente interfaz. La plataforma debe ser un sistema seguro y modular, separando la lógica de scraping, almacenamiento de credenciales e interfaz externa. 

La plataforma debe consistir en tres módulos principales:

| Modulo | Proposito | Nivel de Acceso |
| --- | --- | --- |
| API Gateway | Expone endpoints REST para el consumo del sistema AyniFX | Interno (solo via VPN) |
| Scraping Engine | Maneja automatizacion de navegador y extraccion de datos | Solo a nivel del sistema |
| Credential Manager | Almacenar y gestionar cuentas bancarias de manera segura  | Solo ejecutivos de nivel C (2FA requerido) |

## 1.3 Pilares de la Plataforma

- **Seguridad primero:** Las credenciales bancarias son los activos mas sensibles de la empresa y deben ser protegidos con multiples capas de seguridad.
- **Abstracción:** Los consumidores de la plataforma no necesitan saber que banco están consultando. La API proveerá una interfaz unificada.
- **Extensibilidad:** Agregar nuevos bancos no debería requerir cambios a implementaciones existentes.
- **Auditabilidad:** Cada acción realizada debe ser registrada por cumplimiento y análisis forense.

# 2. Objetivos

## 2.1 Objetivos Principales

| ID | Meta | Resultado medible |
| --- | --- | --- |
| G1 | Proveer una API unificada para acceder a datos de cuentas bancarias | Un unico endpoint retorna datos de cualquier banco configurado |
| G2 | Gestion de credenciales segura con 2FA | Cero acceso no-autorizado a credenciales |
| G3 | Soporte para multiples cuentas por banco | El sistema puede manejar N cuentas para M bancos  |
| G4 | Registro de auditoria completo | 100% de las acciones son registradas con contexto completo |
| G5 | Scraping resiliente  | Retry automatico + alertas cuando falla |

## 2.2 Objetivos Secundarios

| ID | Meta | Resultado medible |
| --- | --- | --- |
| G6 | Respuesta de API < 30 segundos | Tiempo de respuesta del percentil 95 cae bajo el umbral |
| G7 | Manejo de sesion para minimizar re-autenticacion | Las sesiones son re-utilizadas hasta que expiran |
| G8 | Facil adicion de nuevos bancos | Agregar un nuevo banco al sistema toma menos de 4 semanas de esfuerzo |

# 3. User Stories

## 3.1 API Consumer User Stories

| ID | User Story | Acceptance Criteria |
| --- | --- | --- |
| US-001: Consultar Balances de Cuentas | Como la plataforma AyniFX
Quiero consultar el balance actual de un banco en especifico
Para poder mostrar los saldos disponibles para operaciones de tipo de cambio. | - API retorna el balance de la cuenta en su moneda nativa (USD o PEN)
- La respuesta incluye un timestamp de cuando la data fue traida
- El tiempo de respuesta es menor a 30 segundos
- Identificadores de cuentas invalidas retornan codigos de error apropiados |
| US-002: Consultar Historial de Movimientos | Como la plataforma AyniFX
Quiero consultar movimientos recientes para una cuenta bancaria especifica
Para poder reconciliar pagos y seguir movimientos | - Transacciones son retornadas en orden cronologico inverso (mas recientes a mas antiguas)
- El rango por defecto es 7 dias
- El rango de fecha es configurable por request parameters.
- Cada transaccion incluye: date, description, amount, type (credit/debit), balance after |
| US-003: Listas Cuentas Disponibles | Como la plataforma AyniFX
Quiero poder listar todas las cuentas bancarias configuradas
Para poder descubrir que cuentas estan disponibles para consultar | - Retorna un listado de identificadores de cuentas con nombre de banco y moneda 
- NO expone credenciales ni informacion sensible 
- Las cuentas pueden ser filtradas por banco o por moneda |
| US-004: Health Check | Como la plataforma AyniFX
Quiero poder revisar la salud de las conecciones a cada banco
Para saber si una integracion a un banco en particular esta operacional | - Retorna un estado por banco (healthy, degraded, unavailable)
- Incluye un timestamp de la ultima conexion exitosa
- No gatilla operaciones de scraping |

## 3.2 Credential Manager User Stories

| ID | User Story | Acceptance Criteria |
| --- | --- | --- |
| US-101: Agregar Credenciales Bancarias | Como un ejecutivo de nivel C
Quiero poder agregar credenciales bancarias de una nueva cuenta de manera segura
Para permitir que el scraper acceda al banco en nombre de la compania | - Requiere autenticacion 2FA (TOTP) exitosa antes de acceso
- Las credenciales son encriptadas en descanso
- La accion es auditada con la identidad del usuario y el timestamp
- Se requiere de una confirmacion antes de guardar |
| US-102: Ver Cuentas Configuradas | Como un ejecutivo de nivel C
Quiero ver que cuentas bancarias estan configuradas en el sistema
Para poder auditar el setup actual | - Requiere autenticacion 2FA (TOTP) exitosa antes de acceso
- Muestra el nombre del banco, identificador de cuenta (masked), moneda, status 
- NO muestra contrasehas en texto plano
- Acceso es registrado |
| US-103: Actualizar Credenciales Bancarias | Como un ejecutivo de nivel C
Quiero actualizar credenciales para una cuenta bancaria existente
Para poder rotar contrasehas o solucionar problemas de autenticacion | - Requiere autenticacion 2FA (TOTP) exitosa antes de acceso
- Las credenciales antiguas jamas son mostradas 
- El cambio es registrado con metadatos de antes/despues (no las credenciales antiguas/nuevas)
- Existe la opcion de probar credenciales antes de guardar |
| US-104: Borrar Credenciales Bancarias | Como un ejecutivo de nivel C
Quiero borrar credenciales bancarias del sistema
Para desmantelar cuentas que ya no seran ocupadas  | - Requiere autenticacion 2FA (TOTP) exitosa antes de acceso
- Una confirmacion es requerida con el identificador de cuenta explicito
- Se realiza un soft-delete con tiempo de retencion (desvincular antes de borrar por completo) para auditoria
- Hard delete solo despues del tiempo de retencion  |
| US-105: Ver logs de auditoria | Com un ejecutivo de nivel C
Quiero ver registros de auditoria exhaustivos
Para poder monitorear la actividad en el sistema e investigar incidentes | - Requiere autenticacion 2FA (TOTP) exitosa antes de acceso
- Registros (logs) son filtrables por fecha, usuario y tipo de accion
- Se puede exportar a un CSV/JSON por cumplimiento
- Incluye todas las llamadas de API, acceso a credenciales y eventos del sistema |

---

# 4. Requerimientos Funcionales

## 4.1 Modulo de API Gateway

### 4.1.1. Autenticación y Autorización

| ID | Requerimiento |
| --- | --- |
| FR-101 | La API DEBE autenticar todo request usando API Keys |
| FR-102 | Las API Keys DEBEN ser transmitidas mediante el header HTTP `X-API-Key`  |
| FR-103 | API Keys invalidas o faltantes deben retornar `HTTP 401 Unauthorized` |
| FR-104 | API Keys DEBEN ser almacenadas como valores hasheados (no texto plano) |
| FR-105 | El sistema DEBE soportar rotacion de API Keys sin downtime |
| FR-106 | Cada API Key DEBE estar asociado con un identificador de cliente por fines de auditoria |

### 4.1.2 Account Balance Endpoint

| ID | Requerimiento |
| --- | --- |
| FR-201 | La API DEBE exponer `GET /accounts/{account_id}/balance` |
| FR-202 | Response debe incluir: `account_id`, `bank_code`, `currency`, `available_balance` , `current_balance` , `fetched_at` |
| FR-203 | El campo `fetched_at` DEBE ser un timestamp en formato ISO 8601 |
| FR-204 | Si una cuenta no es encontrada, debe retornar un `HTTP 404` con detalles del error |
| FR-205 | Si el scraping falla despues de 3 reintentos, se retorna un `HTTP 503 Service Unavailable` con detalles del error |

**Response Schema**

```json
{
	"account_id": "string",
	"bank_code": "string (BBVA|INTERBANK|BCP)",
	"currency": "string (PEN|USD)",
	"available_balance": "number",
	"current_balance": "number",
	"fetched_at": "string (ISO 8601)"
}
```

### 4.1.3 Transaction History Endpoint

| ID | Requerimiento |
| --- | --- |
| FR-301 | La API DEBE exponer `GET /accounts/{account_id}/transactions`  |
| FR-302 | El endpoint DEBE aceptar query parameters opcionales: `from_date` y `to_date` |
| FR-303 | El rango por defecto debe ser los ultimos 7 dias al no especificar parametros |
| FR-304 | El maximo rango DEBE ser 90 dias |
| FR-305 | Las transacciones deben ser listadas en orden cronologico inverso |
| FR-306 | El Response DEBE incluir metadata de paginacion |

**Response Schema**

```json
{
  "account_id": "string",
  "bank_code": "string",
  "currency": "string",
  "from_date": "string (YYYY-MM-DD)",
  "to_date": "string (YYYY-MM-DD)",
  "transactions": [
    {
      "id": "string",
      "reference": "string", // OPTIONAL
      "date": "string (ISO 8601)",
      "description": "string",
      "amount": "number",
      "type": "string (CREDIT|DEBIT)",
      "balance_after": "number", // OPTIONAL
      "bank_code": "string", // OPTIONAL
      "extra": {} // OPTIONAL
    }
  ],
  "pagination": {
    "total_count": "number",
    "page": "number",
    "page_size": "number",
    "has_more": "boolean"
  },
  "fetched_at": "string (ISO 8601)"
}
```

### 4.1.4 Account Listing Endpoint

| ID | Requerimiento |
| --- | --- |
| FR-401 | La API DEBE exponer `GET /accounts` |
| FR-402 | El endpoint DEBE incluir query parameters opcionales: `bank_code`, `currency` |
| FR-403 | El Response NO DEBE incluir credenciales |
| FR-404 | Cada cuenta DEBE incluir: `account_id`, `bank_code` , `currency` , `account_type`, `status` |

**Response Schema:**

```json
{
	"accounts": [
		{
			"account_id": "string",
			"bank_code": "string",
			"currency": "string",
			"account_type": "string (CHECKING|SAVINGS)",
			"status": "string (ACTIVE|INACTIVE|ERROR)",
			"last_sync": "string (ISO 8601, nullable)"
		}
	]
}
```

### 4.1.5 Health Check Endpoint

 

| ID | Requerimiento |
| --- | --- |
| FR-501 | La API DEBE exponer `GET /health`  |
| FR-502 | El Response DEBE contener un status general del sistema y un status por banco |
| FR-503 | El health check NO DEBE gatillar operaciones de scrapping |
| FR-504 | Los posibles valores de Status deben ser: `healthy`, `degraded`, `unavailable` |

**Response Schema:**

```json
{
	"status": "string (healthy|degraded|unavailable)",
  "timestamp": "string (ISO 8601)",
  "banks": {
    "BBVA": {
      "status": "string",
      "last_successful_connection": "string (ISO 8601, nullable)",
      "error_message": "string (nullable)"
    },
    "INTERBANK": { ... },
    "BCP": { ... }
  }
}
```

### 4.1.6 Manejo de Errores

Todo tipo de errores en la API deben seguir un formato estándar.

**Response Schema:**

```json
{
	"status": "string",
	"message": "string"
}
```

## 4.2 Modulo de Motor de Scraping

### 4.2.1 Automatización de Browser

| ID | Requerimiento |
| --- | --- |
| FR-601 | El scraper DEBE usar automatizacion de browser headless (e.g.,  Playwright, Puppeteer) |
| FR-602 | Cada banco DEBE tener una implementacion de scraper independiente |
| FR-603 | Los scrapers DEBEN implementar una interfaz o contrato comun |
| FR-604 | Los scrapers DEBEN tener timeouts configurables |
| FR-605 | Los scrapers DEBEN poder manejar contenido dinamico de paginas y renderizado con JavaScript |

### 4.2.2 Manejo de Sesión

| ID | Requerimiento |
| --- | --- |
| FR-701 | El sistema DEBE mantener sesiones activas por cuenta bancaria |
| FR-702 | Las sesiones DEBEN ser reutilizadas hasta que expiren o tornen invalidas |
| FR-703 | El estado de una sesion DEBE ser almacenada de manera segura (encriptada en descanso) |
| FR-704 | El sistema DEBE detectar expiracion de sesiones y gatillar una re-autenticacion |
| FR-705 | El maximo tiempo de vida de una sesion debe ser configurable por banco |

### 4.2.3 Manejo de Errores y Resiliencia

| ID | Requerimiento |
| --- | --- |
| FR-801 | Intentos de scraping fallidos DEBEN reintentar con un backoff exponencial |
| FR-802 | La configuracion de reintentos DEBE ser:
- 3 intentos
- delay inicial 1s
- delay maximo 30s |
| FR-803 | Despues de gastar todos los reintentos, el sistema DEBE alertar al equipo de operaciones por mensaje de Slack |
| FR-804 | Cada banco DEBE seguir el patron circuit breaker, y tener una implementacion independiente |
| FR-805 | Los circuit breakers DEBEN activarse despues de 5 fallas consecutivas |
| FR-806 | Los circuit breakers DEBEN intentar resetearse despues de 5 minutos |
| FR-807 | Cuando un circuit esta abierto, los requests DEBEN fallar rapido con un error apropiado |
| FR-808 | Las alertas DEBEN enviarse mediante canales configurables (correo, webhook). |

### 4.2.3 Implementaciones Especificas de Cada Banco

| ID | Requerimiento |
| --- | --- |
| FR-901 | El sistema DEBE incluir scraping del portal web BBVA Peru |
| FR-902 | El sistema DEBE incluir scraping del portal web Interbank Peru |
| FR-903 | El sistema DEBE incluir scraping del portal web BCP Peru |
| FR-904 | Cada implementacion DEBE extraer: saldo de cuenta, saldo de cuenta disponible, lista de movimientos |
| FR-905 | Cada implementacion DEBE manejar formatos de fecha especificos de cada banco |
| FR-906 | Nuevas implementaciones de bancos DEBEN ser agregables sin modificar codigo existente |

# 4.3 Modulo Credential Manager

### 4.3.1 Autenticacion

| ID | Requerimiento |
| --- | --- |
| FR-1001 | Acceso DEBE requerir username/password + TOTP 2FA |
| FR-1002 | Maximo 3-4 usuarios (nivel-C solamente) |
| FR-1003 | Intentos fallidos de login DEBEN ser registrados |
| FR-1004 | Cuentas son bloqueadas despues de 5 intentos fallidos |
| FR-1005 | Secretos TOTP DEBEN estar encriptados en descanso |
| FR-1006 | Sesiones deben cerrarse tras 15 minutos de inactividad |

## 4.3.2 Almacenamiento de Credenciales

 

| ID | Requerimiento |
| --- | --- |
| FR-1101 | Todas las credenciales DEBEN estar encriptadas en descanso usando el estandar AES-256-GCM |
| FR-1102 | Las llaves de encriptacion DEBEN ser almacenadas separadas de la data encriptada |
| FR-1103 | La llave maestra de encriptacion DEBE usar envelope encryption |
| FR-1104 | Las credenciales JAMAS DEBEN ser registradas en texto plano |
| FR-1105 | Las credenciales JAMAS DEBEN ser retornadas en responses API |
| FR-1106 | Backups de base de datos DEBEN mantener encriptacion |

## 4.3.3 Operaciones de Credenciales

 

| ID | Requerimiento |
| --- | --- |
| FR-1201 | Admin DEBE poder agregar nuevas credenciales bancarias  |
| FR-1202 | Admin DEBE poder actualizar credenciales existentes |
| FR-1203 | Admin DEBE poder hacer un soft-delete de credenciales existentes |
| FR-1204 | Admin DEBE poder probar credenciales antes de guardar |
| FR-1205 | Toda operacion DEBE requerir autenticacion para acciones sensibles |
| FR-1206 | Cambios en credenciales DEBEN crear una nueva version (versioning) |

## 4.3.4 Registro de Auditoria

| ID | Requerimiento |
| --- | --- |
| FR-1301 | Toda accion DEBE ser registrada con: `timestamp`, `user_id`, `action`, `target` , `ip_address`, `user_agent` |
| FR-1302 | Los registros DEBEN incluir tanto operaciones exitosas como no exitosas |
| FR-1303 | Los registros DEBEN ser immutables |
| FR-1304 | Los registros DEBEN ser retenidos por al menos 2 anhos |
| FR-1305 | Los registros deben ser exportables a CSV y JSON  |
| FR-1306 | Las entradas de registros NO DEBEN contener data sensible (contrasenhas, credenciales) |
| FR-1307 | El acceso a los registros en si debe ser registrado |

# 5. Out of Scope

Los siguientes puntos NO ESTÁN dentro del alcance para esta version:

| ID | No-Meta | Razon |
| --- | --- | --- |
| NG-01 | Exposicion externa de la API | Solo la plataforma AyniFX consumira la API (on-prem) |
| NG-02 | Autenticacion OAuth 2.0/JWT  | API Keys cubren nuestro caso para solo un consumidor |
| NG-03 | Iniciacion de transferencias | Solo cubriremos a nivel de lectura |
| NG-04 | Aplicacion movil | El gestor de credenciales se accedera solo mediante web |
| NG-05 | Manejo de OTP/2FA para loguear a bancos  | Los bancos iniciales no lo requieren |
| NG-06 | Rate limiting | No necesario con un unico consumidor inicial |