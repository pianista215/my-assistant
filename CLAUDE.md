# CLAUDE.md

Guía para Claude Code al trabajar en este repositorio.

## Qué es esto

Servicio REST en Go que decide y sirve qué debe mostrar una pantalla e-ink conectada a un ESP32. El ESP32 consultará el endpoint cada hora; el contenido cambiará con la hora del día y, en iteraciones futuras, vendrá de Google Calendar y Google Sheets. Corre de forma autónoma en un VPS. Es el primer proyecto en Go del usuario — prioriza código idiomático y explicable sobre atajos.

Ver [README.md](README.md) para el detalle de uso, endpoint y formato binario.

## Hardware objetivo

Seeed reTerminal E1001 (panel GDEY075T7, controlador UC8179): **800×480 px, 4 niveles de gris (2 bits/píxel)**. No hay estándar de imagen ligero que valga la pena adoptar para esto, de ahí el formato binario propio en `internal/display/codec.go` y la CLI `cmd/preview` para inspeccionarlo visualmente.

## Comandos

```bash
go build ./...
go vet ./...
go test ./...
go run ./cmd/server
go run ./cmd/preview --file buffer.bin
```

A diferencia de otros proyectos de este usuario, aquí **sí se pueden ejecutar los tests directamente** (`go test ./...`) durante la implementación — es un proyecto Go nuevo, pequeño y aislado, sin la política de "no ejecutes tests, dame el comando" que aplica a otros repos (esa política es de otro proyecto no relacionado).

## Convenciones de este proyecto

- **Sin paquetes de un solo uso**: no crear un paquete `internal/auth` solo para el middleware de token — vive en `internal/server/middleware.go`, junto al resto del servidor HTTP. Si en el futuro hay varios middlewares reutilizados entre distintos servidores, ahí sí se justifica extraer `internal/middleware` (patrón `mid` de Ardan Labs Service).
- **`internal/display`, no `internal/eink`**: el paquete representa *qué se va a mostrar* (imagen + codec), no el driver/firmware del panel. Consistente con el endpoint `/api/v1/display`.
- **Token de autenticación**: se carga desde variable de entorno (`AUTH_TOKEN`), con soporte de `.env` en desarrollo vía `github.com/joho/godotenv`. En producción el VPS define variables de entorno reales (systemd `EnvironmentFile=`), no hay `.env` en el servidor. La comparación del token usa `crypto/subtle.ConstantTimeCompare`.
- **Formato de imagen propio**: `internal/display/codec.go` empaqueta a 2 bits/píxel sin estándar externo, pensado para minimizar memoria en el ESP32. Cualquier cambio de formato debe mantener el roundtrip `Encode`/`Decode` y su test.

## Alcance por iteraciones

- **Iteración 1 (actual)**: estructura del REST, middleware de auth por token, endpoint `/api/v1/display` con placeholder "Hello World" + hora actual, CLI de visualización, tests. **Sin** integración con Google todavía — ni siquiera paquetes stub, se añadirán cuando se diseñe esa fase.
- **Próximas iteraciones**: integración con Google Calendar/Sheets como fuente real de contenido, lógica de variación según la hora del día, firmware del ESP32, despliegue en VPS.
