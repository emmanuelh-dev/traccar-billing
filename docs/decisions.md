# Decisiones

Cosas que se ven como bugs pero se hicieron a propósito. Si vas a cambiar
alguna, que sea a sabiendas.

## 1. Pagar tarde recorre el ciclo (no se "pierden" días)

`ApplyPayment` calcula `next_due_at = fecha_de_pago + period_days`, no
`vencimiento_anterior + period_days`. Un cliente que vencía el 1 y paga el 10
tiene su siguiente vencimiento el 10, no el 1.

**Es intencional.** En el modo `rolling`, el cliente compra N días de
servicio y los recibe completos aunque pague tarde. La alternativa (anclar al
calendario) existe como el modo `calendar` — ver
[billing-rules.md](billing-rules.md). No lo "arregles" en `rolling`.

## 2. Nunca se guarda la contraseña de Traccar

Solo la cookie de sesión que devuelve el login. Si vence, el operador vuelve
a meter la contraseña. **No hay ni habrá** una variable de entorno para
saltarse esto.

Costo aceptado: si la sesión vence, el scheduler deja de poder cortar
morosos hasta que alguien entre. La mitigación correcta es avisar de forma
visible, no guardar la contraseña.

## 3. El `TraccarClient` no tiene estado

Cada método recibe `baseURL` y `Session` como parámetros. Un solo valor del
cliente sirve a todos los tenants en paralelo, sin locks ni estado por
tenant. No le agregues campos de sesión.

## 4. Los dispositivos se cuentan por usuario, no en bloque

`FetchDevicesForUser` hace una petición por usuario, lo cual es más lento que
un `FetchDevices` global. Es necesario: los dispositivos de Traccar no traen
campo de dueño, así que la única forma de saber de quién es cada uno es
preguntar por usuario.

## 5. El corte se reintenta cada tick

`checkOverdue` llama a `pauseTraccarUser` en cada pasada, no solo cuando la
suscripción cambia a `overdue`. Así, si Traccar estaba caído en el primer
intento, se corrige solo en la siguiente vuelta. Es idempotente a propósito.

## 6. Las plantillas nunca reciben modelos del dominio

Cada handler arma su propia *view struct* con los valores ya formateados
(`AmountDisplay`, `DaysLeftLabel`). Evita meter lógica y formato en el HTML.

## 7. Sin dependencias de frontend

HTML plano renderizado en el servidor, CSS propio, cero JavaScript de
terceros. El binario es autocontenido. Los modales de la Fase 1 se hacen con
`<dialog>` nativo y un puñado de JS embebido, no con un framework.
