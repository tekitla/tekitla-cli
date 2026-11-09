# tekitla-cli — contexto para Claude

## Qué es

`tk`, la CLI de Tekitla (issue 029, rebanada 2 del 002): proyectos e issues
desde la terminal y desde las sesiones de Claude. Go
([ADR 0002](https://github.com/tekitla/tekitla-docs/blob/main/contenido/adr/0002-cli-del-tracker-en-go.md)).

- **Habla con** `tekitla-gateway` por REST (rutas de `issues.v1.IssueService`,
  `docs/rutas.md` del gateway), con el ID token de Firebase en
  `Authorization: Bearer`. JSON de protojson: camelCase, enums como string.
  **No cambia** el contrato ni el backend: si falta algo, va primero a
  `tekitla-protos` → `tekitla-issues` → gateway.
- **Login** ([ADR 0005](https://github.com/tekitla/tekitla-docs/blob/main/contenido/adr/0005-login-de-la-cli-y-membresias-de-tequio-via-la-plataforma.md)):
  device flow de Google (scopes `openid email`) → `accounts:signInWithIdp` de
  Firebase (proyecto `tekitla-app`). El refresh token va al llavero del
  sistema (o a `~/.config/tk/sesion.json` con 0600) y el ID token se renueva
  solo (5 min antes de vencer, y una vez más si la API responde 401).

## Estructura

| Paquete | Qué hace |
|---|---|
| `cmd/tk` | `main`: señales y código de salida |
| `internal/cli` | Comandos (cobra), salida `--json`/tablas y códigos de salida |
| `internal/api` | Cliente HTTP del gateway: tipos, paginación, errores |
| `internal/auth` | Device flow, Firebase, almacén de la sesión, renovación |
| `internal/config` | Ambiente, URL de la API, `config.toml`, cliente OAuth, config pública de Firebase |

## Reglas

- **Códigos de salida:** `0` bien; `1` error de uso o de la API; `2` sin
  sesión o vencida (pide `tk login`). Los errores van a stderr (con `--json`,
  como `{"error": {"code", "message", …}}`); stdout queda para el resultado.
- **Tokens:** el ID token, el refresh token y el código de dispositivo nunca
  se imprimen ni aparecen en errores (los errores de red muestran solo el
  host: la URL de Firebase lleva la apiKey). `TestNoSeFiltranTokens` lo cubre.
- **`--json` es un contrato** con los agentes: nombres en español y
  snake_case; solo se agregan campos, nunca se renombran ni se quitan.
- **Credenciales:** el cliente OAuth («TVs y dispositivos de entrada
  limitada») entra por `-ldflags` en el release (variables del CI), por
  `[oauth]` en `config.toml` o por `TK_OAUTH_CLIENT_ID`/`TK_OAUTH_CLIENT_SECRET`.
  Nunca en el repo. La apiKey web de Firebase es pública y vive solo en
  `internal/config/publico.go` (allowlist de gitleaks).
- **Tests sin red:** `internal/cli/falso_test.go` es un `RoundTripper` que
  simula Google, Firebase y el gateway. Todo comando nuevo lleva su test ahí.

## Comandos de desarrollo

```sh
make build   # bin/tk (bin\tk.exe en Windows)
make test
make lint    # gofmt + vet + golangci-lint (v2.15.0)
make vuln    # govulncheck
```

Contra un gateway local: `TK_API_URL=http://localhost:8080 bin/tk proyectos`
(en PowerShell: `$env:TK_API_URL = "http://localhost:8080"; .\bin\tk.exe proyectos`).

## Usar tk desde una sesión de Claude

Las sesiones llaman `tk … --json` por Bash y leen el código de salida:

```sh
tk proyectos --json
tk issues -p TK --estado "En curso" --json
tk issue TK-42 --json                      # detalle + últimos 10 eventos
tk issue nuevo -p TK "Título" --prioridad alta --descripcion "…" --json
tk mover TK-42 "En curso" --json           # por nombre de estado (sin acentos ni mayúsculas da igual)
tk comentar TK-42 "Hecho en tekitla-cli#1" --json
```

- Código `2`: no hay sesión. **No intentes `tk login`** desde la sesión
  (necesita que una persona apruebe en el navegador): avisa a Victor.
- Código `1` con `"code": "ESTADO_DESCONOCIDO"`: el error trae
  `estados_validos`; elige uno de esos.
- `--ambiente qa` (o `TK_AMBIENTE=qa`) para probar contra QA; prod por defecto.

## Fuera de la v0.1

`tk start`, `tk board`, tokens `tka_` (`tk login --token`, issue 025) y
sesiones por dispositivo (issue 019).
