# my-assistant

Servicio REST en Go que sirve a un ESP32 con pantalla e-ink qué debe mostrar. El ESP32 consulta el endpoint cada hora; el contenido irá cambiando con la hora (y, en iteraciones futuras, vendrá de Google Calendar y Google Sheets). Pensado para correr de forma autónoma en un VPS.

**Estado actual (primera iteración)**: solo la estructura base — un endpoint protegido por token que devuelve una imagen placeholder ("Hello World" + hora actual). Todavía no hay integración con Google.

## Hardware objetivo

Pantalla e-ink Seeed Studio reTerminal E1001 (familia "E10xx"), panel GDEY075T7, controlador UC8179:

- Resolución: **800 × 480 px**
- **4 niveles de gris** (negro, gris oscuro, gris claro, blanco) = 2 bits por píxel

No existe un formato estándar ligero para 4 niveles de gris que valga la pena adoptar, así que el servidor usa un **formato binario propio** pensado para minimizar el uso de memoria en el ESP32 (ver [`internal/display/codec.go`](internal/display/codec.go)):

```
offset  tamaño  campo
0       4       magic "EINK"
4       1       versión de formato
5       2       ancho  (uint16 big-endian)
7       2       alto   (uint16 big-endian)
9       1       bits por píxel
10      ...     datos de píxel empaquetados a 2 bits/píxel (4 píxeles por byte)
```

## Requisitos

- Go 1.18+

## Configuración

```bash
cp .env.example .env
# edita .env y pon un AUTH_TOKEN aleatorio, ej: openssl rand -hex 32
```

En producción (VPS) no se usa `.env`: las variables de entorno reales se definen en el propio servicio (por ejemplo `EnvironmentFile=` en la unidad systemd).

## Arrancar el servidor

```bash
go run ./cmd/server
```

Por defecto escucha en `:8080` (configurable con `PORT`).

## Endpoint

`GET /api/v1/display`

Requiere cabecera `Authorization: Bearer <AUTH_TOKEN>`. El mismo token debe estar fijado en el firmware del ESP32.

```bash
curl -H "Authorization: Bearer $AUTH_TOKEN" http://localhost:8080/api/v1/display -o buffer.bin
```

- Sin token o con token incorrecto → `401 Unauthorized`.
- Con token correcto → `200 OK`, `Content-Type: application/octet-stream`, cuerpo = imagen en el formato binario descrito arriba.

## Herramienta de visualización (`cmd/preview`)

Como no se usa un formato de imagen estándar, `cmd/preview` permite inspeccionar en la propia terminal qué se le está enviando al ESP32, sin necesidad de tener el panel físico. Pinta la imagen usando caracteres de bloque Unicode y colores ANSI de escala de grises (232-255), aprovechando semi-bloques (`▀`) para duplicar la resolución vertical aparente.

```bash
# contra un buffer ya descargado
go run ./cmd/preview --file buffer.bin

# o directamente contra el servidor
go run ./cmd/preview --url http://localhost:8080/api/v1/display --token "$AUTH_TOKEN"

# --cols controla el ancho de salida en columnas de terminal (por defecto 120)
go run ./cmd/preview --file buffer.bin --cols 160
```

Para imágenes con contenido fino (como texto), la herramienta reduce cada bloque de píxeles quedándose con el más oscuro, así el contenido delgado no desaparece al hacer submuestreo.

## Tests

```bash
go test ./...
```

Cubren: validación del token (auth middleware), codificación/decodificación round-trip del formato binario propio, y el handler del endpoint vía `httptest`.

## Estructura del proyecto

```
cmd/
  server/     # entrypoint del servidor HTTP
  preview/    # CLI de visualización del buffer en terminal
internal/
  config/     # carga de configuración (token, puerto) desde entorno/.env
  display/    # generación de la imagen a mostrar + codec del formato binario propio
  server/     # router, middleware de auth y handlers HTTP
```

## Roadmap

- Integración con Google Calendar y Google Sheets como fuente real del contenido a mostrar (sustituirá al placeholder "Hello World").
- Lógica de variación por hora: qué se muestra y con qué formato según el momento del día.
- Firmware del ESP32 que consulta este endpoint cada hora y pinta el buffer recibido en el panel e-ink.
- Despliegue en VPS (systemd, variables de entorno reales).
