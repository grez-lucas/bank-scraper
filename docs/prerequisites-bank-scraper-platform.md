# Bank Scraper Development Prerequisites

Author: Lucas Grez
Created time: December 18, 2025 11:59 AM
Category: Strategy doc
Last edited by: Lucas Grez
Last updated time: December 23, 2025 5:05 PM

# Resumen Ejecutivo

Este documento lista todos los pre-requisitos necesarios para comenzar el desarrollo de la plataforma de Bank Scraping sin bloqueantes. Los items están categorizados por prioridad y propiedad. **El bloqueante mas critico es la estrategia de testing—**los desarrolladores no pueden construir un scraper efectivamente sin una manera de probar contra los portales bancarios que no involucre usar nuestros bancos de producción en cada cambio de código. 

---

# 1. Acceso a Cuentas Bancarias y Estrategia de Pruebas

## 🔴 Critico: Infraestructura de Pruebas

El desarrollo de un scraper requiere cientas de iteraciones. Probar tantas veces contra los portales en vivo de cada banco puede llevar a problemas inesperados como:

- Rate-limiting o bloqueos tras demasiados logins
- Riesgo de deteccion de sistemas de fraude
- Feedback loops lentos (30+ segundos por prueba)
- No se puede correr un CI/CD automatizado contra bancos en vivo
- Riesgo de exponer credenciales bancarias en entornos de prueba

## Estrategia Sugerida: Record & Play

![image.png](Bank%20Scraper%20Development%20Prerequisites/image.png)

Esta estrategia consiste en priorizar la mayoría de las pruebas en paginas pre-generadas y sesiones grabadas. De esta manera evitamos consumir las plataformas en vivo de cada banco durante el desarrollo. Para esto, necesitamos lo siguiente:

### 1. Requerimiento: Cuentas de Prueba Dedicadas por Banco

| Banco | Requerimiento | Proposito | Responsable |
| --- | --- | --- | --- |
| BBVA | 1 cuenta de pruebas (PEN) | Desarrollo y grabado de sesiones | Finanzas |
| BBVA | 1 cuenta de pruebas (USD) | Pruebas de variacion segun moneda | Finanzas |

**Notas importantes:**

- Estas deben ser **cuentas reales** pero con **fondos minimos**
- Estas cuentas deben ser del **mismo tipo que la cuenta principal de AyniFX.** (e.g., si esta es empresa, o persona).
- Deben tener **mucha actividad en sus movimientos** (aunque sean transferencias pequenhas). Deben ser muchas, para asegurarnos de cubrir casos de paginacion.
- Las credenciales serán ocupadas para **grabar sesiones** que serán replicadas durante desarrollo
- El acceso en vivo sera **restringido** al equipo designado

### 2. Requerimiento: Proceso de Grabado de Sesiones

Dado que el requerimiento 1 fue cumplido, los siguientes flujos deben ser grabados para poder ser replicados por el desarrollador sin la necesidad de acceder a estas credenciales.

| Grabacion Requerida | Descripcion | Frecuencia | Encargado |
| --- | --- | --- | --- |
| Login flow | Toda la secuencia de autenticado | Una vez por banco + si cambia el portal | Lucas |
| Pagina de saldos | Pagina de resumen de cuenta | Una vez por banco + si cambia el portal | Lucas |
| Historial de transacciones | Multiples paginas de movimientos | Semanal (para capturar nuevos formatos) | Lucas |
| Estados de error | Login invalido, timeout, mantenimiento | Cuando sean encontrados | Lucas |
| Casos borde | Cuentas vacias, transacciones pendientes | Cuando sean encontrados | Lucas |

**Entregable:** Un archivo grabado HAR o trace de Playwright para cada banco, para que los desarrolladores puedan replicar localmente.

# 2. Team Roles

Para poder llevar a cabo este proyecto, se requiere de un equipo que cubra los siguientes roles. Cabe notar que un miembro puede cubrir mas de un rol.

| Rol | Responsabilidad |
| --- | --- |
| Tech Lead | Decisiones de arquitectura, code review |
| Backend Developer | API Gateware, core services |
| Scraper Developer | Implementaciones especificar por banco |
| Security Engineer | Credential Manager, encriptacion |
| DevOps Engineer | Infraestructura, configuracion de CI/CD, deployment |