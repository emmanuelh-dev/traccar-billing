# Plan

Estado: `[ ]` pendiente · `[~]` parcial · `[x]` hecho

El orden importa. Las fases 0 y 1 son las que hacen que el sistema cuadre y
que cobrar deje de ser tedioso; automatizar (Fase 2) sobre una base que
descuadra solo multiplica el error.

---

## Fase 0 — Que no descuadre

Bugs de dinero y de operación. Es lo más barato y lo que más duele hoy.

- [x] **Cobro atómico.** `UpsertSubscription` y `RecordPayment` son dos
      llamadas separadas en `api/handlers_accounts.go`; si la segunda falla,
      la cuenta queda al corriente sin pago registrado. Agregar `WithTx` al
      repositorio y envolver el cobro completo.
- [x] **Pagos parciales.** Hoy el pago se registra siempre por el monto de la
      suscripción (`AmountCents: updatedSub.AmountCents`), ignorando lo que
      capturaste. Debe guardarse el monto real y aplicarse contra el saldo.
- [x] **Periodo de gracia.** `grace_days` en la suscripción; `IsOverdue` corta
      un segundo después del vencimiento, sin margen.
- [x] **Bajas en el sync.** `scheduler.syncTenant` solo hace upsert: si borras
      un usuario en Traccar, la cuenta sigue viva y cobrable. Marcar
      `archived_at` cuando el usuario ya no aparece.
- [~] **Zona horaria.** Hecho a medias: variable `TIMEZONE` global (`America/Mexico_City` en producción). Falta que sea por tenant, que es lo que necesita un producto multi-cliente. Antes: `time.Parse("2006-01-02", …)` da medianoche
      UTC; en México corta ~6 h antes de tiempo.
- [x] **Moneda consistente.** Default `MXN` en `handlers_subscription.go:71`
      pero `USD` en `currencyOrDefault` y en el esquema. Una sola fuente:
      moneda por tenant.
- [x] **Aviso de sesión de Traccar vencida.** Hoy solo hay un `Warn` en el log
      y los morosos dejan de cortarse en silencio. Banner rojo en el
      dashboard con botón para reconectar.

> El "recorrido" del ciclo al pagar tarde **no** está en esta lista: es
> intencional. Ver [decisions.md](decisions.md).

---

## Fase 1 — Cobrar bien y que la UI ayude

Lo que hace que registrar un pago deje de ser adivinar.

- [x] **Precio por dispositivo.** `unit_price_cents` configurable por cuenta,
      con `flat_fee_cents` y `min_devices` opcionales. Es el modelo natural
      de un GPS y ya sincronizas `device_count` sin usarlo.
      `total = max(dispositivos, min_devices) × unitario + base`.
- [x] **Modal de cobro.** Al registrar un pago, un `<dialog>` con:
      - dispositivos a cobrar, **precargado con los que tiene la cuenta**, editable
      - precio unitario (de la suscripción, editable para ese cobro)
      - **total calculado en vivo** al cambiar cualquiera de los dos
      - fecha de pago, método (efectivo/transferencia/otro), referencia, nota
      - si hay remisión abierta, precarga sus datos

      El total se recalcula en el servidor al guardar; el JS es comodidad, no
      la fuente de verdad.
- [x] **Modo de ciclo por calendario.** `billing_mode = calendar` con
      `anchor_day` y `due_day` (el caso "cada primero, vence el 5"), además
      del modo `rolling` actual por días corridos. Ver
      [billing-rules.md](billing-rules.md).
- [x] **Sync inmediato al conectar.** Hoy, al iniciar sesión con un servidor
      Traccar nuevo, las cuentas no aparecen hasta el siguiente tick del
      scheduler (hasta 15 min de "lag"). Correr `syncTenant` en el momento del
      login, con timeout, y mostrar el estado mientras corre.
- [~] **Rediseño del dashboard.** Hechos los modales, la fila compacta y el scroll horizontal; faltan el buscador, el filtro por estado, la página de detalle por cuenta y los totales de arriba. El listado actual mete formulario de
      configuración, botón de pago y el historial completo dentro de cada fila
      con `<details>`. Con 50 cuentas es inmanejable. Propuesta:
      - fila compacta: cuenta · dispositivos · monto · estado · vence · acciones
      - buscador y filtro por estado (al corriente / por vencer / vencidos)
      - configurar y cobrar en modal, no expandiendo la fila
      - página de detalle por cuenta con su historial completo
      - totales arriba: por cobrar del mes, cobrado, vencido
- [~] **Mejorar `/payments`.** Hechos el total cobrado, editar y anular; faltan el filtro por rango de fechas y por cuenta, y el export CSV. Filtro por rango de fechas y por cuenta,
      total del periodo, y exportar CSV.
- [x] **Anular pagos.** Un pago mal capturado hoy es permanente. `voided_at`
      + motivo, y el saldo se recalcula.

---

## Fase 2 — Remisiones automáticas

El corazón de lo que falta: que el sistema genere solo lo que hay que cobrar.

- [ ] **Entidad `statements`** (ver [data-model.md](data-model.md)), con
      folio consecutivo por tenant.
- [ ] **Generador automático.** Job diario en el scheduler (separado del sync,
      que corre cada 15 min): para cada cuenta activa en modo `calendar`
      cuyo `anchor_day` sea hoy, emite la remisión del periodo con el conteo
      de dispositivos **congelado**. Idempotente vía
      `UNIQUE(account_id, period_start)`.
- [ ] **Vista de remisiones** `/statements`: filtro por estado y periodo,
      generar a mano una faltante, cancelar con motivo.
- [ ] **Remisión imprimible / PDF** con folio, periodo, desglose de
      dispositivos y total. Es lo que el cliente pide como comprobante.
- [ ] **Morosidad basada en remisión**, no en `next_due_at` suelto: el corte
      se dispara por remisión vencida + gracia.

---

## Fase 3 — Cobranza

Lo que más baja la morosidad, más que cualquier otra función de esta lista.

- [ ] Envío por correo (SMTP) de la remisión al emitirse.
- [ ] Recordatorios: 3 días antes de vencer, el día del vencimiento, y aviso
      antes de suspender.
- [ ] Plantillas de mensaje editables por tenant, en español e inglés.
- [ ] Bitácora de envíos (qué se mandó, a quién, cuándo) para poder decir
      "sí te avisamos".
- [ ] WhatsApp como canal opcional.

---

## Vendedores

- [x] **Alta de vendedores** con nombre, correo, teléfono, comisión y activo,
      en su propia pestaña `/sellers`.
- [x] **Asignar un vendedor a cada cuenta** desde el dashboard, con columna
      propia y desasignación.
- [x] **Totales por vendedor**: cuentas, dispositivos, cobro mensual y
      comisión calculada.
- [ ] Filtrar el dashboard y los pagos por vendedor.
- [ ] Liquidación de comisiones: hoy el porcentaje solo se muestra como
      referencia, no genera un pago a liquidar ni lleva historial.

## Fase 4 — Producto para vender

Necesario porque el destino es que otros administradores de Traccar lo usen,
no solo tú.

- [ ] **Usuarios operadores locales.** Hoy la sesión del navegador es la del
      tenant y quien tenga las credenciales de Traccar entra. Hacen falta
      usuarios propios con contraseña, y roles: dueño / cobrador (solo
      registra pagos) / lectura.
- [ ] **Bitácora de auditoría** (`audit_log`): quién cobró, quién canceló,
      quién suspendió.
- [ ] **Respaldos.** Copia programada de la base y comando de restauración.
      Es lo primero que pregunta quien va a confiarte su cobranza.
- [ ] **Instalación de una línea.** Imagen publicada, `docker compose` con
      valores por default sensatos, y comprobación de salud real
      (`/health` hoy no verifica base de datos ni sesión de Traccar).
- [ ] **Reportes:** antigüedad de saldos, ingreso por mes, morosidad,
      ingreso recurrente, altas y bajas.
- [ ] **Cobro en línea.** Link de pago (Stripe / Mercado Pago) y webhook que
      registra el pago y reactiva al usuario solo. Va aquí y no antes: sin
      remisiones y sin cobro atómico, automatizar el ingreso es prematuro.

---

## Fase 5 — Fiscal (México)

Los datos se capturan desde la Fase 0/1 (migración 7); el timbrado llega aquí.

- [ ] Captura y validación de RFC, régimen fiscal y uso de CFDI por cuenta.
- [ ] Timbrado CFDI 4.0 vía PAC (Facturama o SW Sapien).
- [ ] Cancelación de CFDI con motivo SAT.
- [ ] Descarga de XML y PDF, y complemento de pago (PPD) si aplica.

---

## Pendientes sin fase asignada

- **Portal de clientes.** Que cada cliente entre a ver su estado de cuenta,
  sus remisiones y sus pagos. Requiere autenticación de clientes (separada de
  la de operadores) y, para que valga la pena, pago en línea. Anotado también
  en el README.
- Notificaciones al operador cuando el sync falla varias veces seguidas.
- Multi-moneda real (hoy se asume una por tenant).
