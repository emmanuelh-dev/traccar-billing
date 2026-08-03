# Reglas de cobro

Todo lo que está en este documento vive (o debe vivir) en `internal/billing`,
como funciones puras que reciben `now` como parámetro. Nada de I/O aquí.

## Estados de una suscripción

| Estado | Significado | Acceso a Traccar |
|---|---|---|
| `active` | Al corriente. | Habilitado |
| `overdue` | Pasó el vencimiento (más la gracia). | Deshabilitado |
| `suspended` | Cortado a mano por el operador. | Deshabilitado |
| `canceled` | Ya no se cobra. Nunca vuelve a vencer. | Sin tocar |

`IsOverdue` ignora las canceladas a propósito.

## Modos de ciclo

Hoy solo existe uno. El objetivo son dos, elegibles por cuenta con un campo
`billing_mode` en la suscripción.

### 1. `rolling` — por días corridos (el actual)

`period_days` días a partir del pago. Al registrar un pago:

```
next_due_at = fecha_de_pago + period_days
```

Esto significa que quien paga tarde recorre su ciclo hacia adelante y no
pierde días de servicio. **Es intencional**, ver [decisions.md](decisions.md).

Sirve para clientes que entran a mitad de mes o que pagan cuando pueden.

### 2. `calendar` — anclado al mes (a implementar)

El periodo es el mes natural. Dos parámetros:

- `anchor_day` — día en que se **genera** la remisión del periodo (ej. 1).
- `due_day` — día en que se **vence** (ej. 5).

Ejemplo con `anchor_day = 1`, `due_day = 5`:

```
1 de marzo   → se genera la remisión de marzo (1 mar – 31 mar)
5 de marzo   → vence
5 + gracia   → si no pagó, se corta
```

Pagar tarde **no** recorre nada: la remisión de abril se genera el 1 de abril
igual. Este es el modo que corresponde al "todo es cada primero al 5".

Los días de un mes varían, así que el cálculo usa `AddDate(0, 1, 0)` sobre el
inicio del periodo, no sumas de días. Cuando `anchor_day` es 29, 30 o 31 y el
mes no lo tiene, se usa el último día del mes.

## Cómo se calcula el monto

Hoy: un `amount_cents` fijo por suscripción. El `device_count` se sincroniza
pero no se usa para cobrar.

Objetivo — el monto se deriva de los dispositivos:

```
subtotal = max(dispositivos, min_devices) × unit_price_cents + flat_fee_cents
```

- `unit_price_cents` — precio por dispositivo, se configura en la cuenta.
- `flat_fee_cents` — cargo base opcional (0 por default).
- `min_devices` — mínimo facturable (0 = sin mínimo).

Compatibilidad: si `unit_price_cents = 0`, se cobra `amount_cents` como
precio fijo, que es el comportamiento de hoy. Ninguna cuenta existente
cambia de monto al migrar.

**El conteo de dispositivos se congela al generar la remisión.** Si el
cliente agrega un GPS a media semana, eso afecta la remisión del **siguiente**
periodo, no la que ya está emitida. Sin esta regla el monto adeudado cambiaría
solo y sería imposible aclarar una disputa.

## Remisiones

Una remisión es el documento que dice *qué* está pagando el cliente: periodo,
cuántos dispositivos, a qué precio, cuánto suma, para cuándo. Hoy no existe —
solo hay un `next_due_at` suelto y una lista de pagos que no está ligada a
ningún periodo.

Ciclo de vida:

```
open ──(pago parcial)──> partial ──(completa)──> paid
 │
 ├──(pasa due_at + grace_days)──> overdue ──(pago)──> paid
 └──(error de captura)──────────> void
```

Reglas:

- **Idempotencia:** `UNIQUE(account_id, period_start)`. El generador puede
  correr cien veces al día sin duplicar nada. Esto no es negociable: el job
  va a reintentar.
- Un pago se aplica contra la remisión abierta más vieja. Si sobra, queda
  como saldo a favor de la cuenta.
- Una remisión no se edita: se cancela (`void`) y se emite otra. Es lo que
  permite auditar.
- Cancelar (`void`) requiere motivo y queda registrado quién y cuándo.

## Morosidad y corte

```
due_at  →  [grace_days]  →  overdue  →  se deshabilita el usuario en Traccar
```

Hoy `grace_days` no existe: `IsOverdue` corta un segundo después del
vencimiento y el scheduler suspende en el mismo tick. Hace falta el campo,
con default configurable por tenant.

Antes de cortar hay que avisar. Ver la Fase 3 del roadmap (cobranza).

**Nunca se corta al usuario administrador del tenant.** Está protegido en
`scheduler.pauseTraccarUser` y en `api.syncTraccarAccess`; si agregas otra
ruta de corte, replica la protección.

## Zonas horarias

Todo se guarda en UTC. Pero un vencimiento es una fecha de calendario, no un
instante: "vence el 5" significa fin del día 5 **en la zona del tenant**.
Hoy `time.Parse("2006-01-02", ...)` produce medianoche UTC, que en México
adelanta el corte unas 6 horas. Hace falta `tenants.timezone` y calcular los
límites del día ahí.

## Dinero

Siempre enteros de centavos (`int64`), nunca `float`. `parseAmountCents`
convierte desde la entrada del formulario con `math.Round`; es el único lugar
donde aparece un flotante y así debe seguir.

No se mezclan monedas: los totales de un tenant asumen una sola moneda.
Si algún día se necesitan varias, hay que agrupar por moneda en los reportes,
no convertir.
