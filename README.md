# tekitla-cli

`tk`: la CLI de [Tekitla](https://tekitla.victorrb1015.com). Proyectos e
issues desde la terminal (Go).

## Instalar

Descarga el binario de tu sistema desde
[Releases](https://github.com/tekitla/tekitla-cli/releases) y ponlo en el
`PATH`. Desde el código: `make build` (deja `bin/tk`; en Windows, `bin\tk.exe`).

## Uso

```sh
tk login                 # device flow de Google; --no-browser por SSH
tk proyectos
tk issues -p TK [--estado "En curso"] [--prioridad alta,urgente] [--buscar texto]
tk issue TK-42
tk issue nuevo -p TK "Título" [--prioridad alta] [--descripcion "…"]
tk mover TK-42 "En curso"
tk comentar TK-42 "Texto"
tk agentes [--revocados]
tk agente nuevo "Claude Code PC-01" --herramienta claude-code|codex|cloud|otra
tk agente revocar "Claude Code PC-01"     # y todos sus tokens; no se deshace
tk token nuevo --agente "Claude Code PC-01" --permiso issues:read [--permiso issues:write] [--vence 30d]
tk tokens [--todos]                        # id, agente, permisos y vencimiento; nunca el valor
tk token revocar <id>
tk version                                 # versión y ambiente por defecto
tk logout
```

En todos los comandos:

- `--json`: salida estable para agentes y scripts.
- `--ambiente qa|prod`: prod por defecto (también `TK_AMBIENTE` o
  `ambiente = "qa"` en la configuración). Los comandos que escriben (agente
  nuevo, token nuevo, revocar) dicen dónde lo hicieron: `[qa] Agente "…"
  creado`, o el campo `ambiente` con `--json`. Los comandos que tk sugiere
  repiten `--ambiente` cuando no es prod o vino en el flag, para que copiados
  tal cual vayan al mismo ambiente. `tk version` muestra el **ambiente por
  defecto** (el de un comando sin `--ambiente`).

Códigos de salida: `0` bien, `1` error de uso o de la API, `2` sin sesión
(corre `tk login`).

## Tokens de agente

Un agente (Claude Code, Codex…) se conecta al MCP de Tekitla con un token
`tka_` propio. Se saca con tu sesión de `tk login`:

```sh
tk agente nuevo "Claude Code PC-01" --herramienta claude-code
tk token nuevo --agente "Claude Code PC-01" --permiso issues:read
```

`tk token nuevo` imprime el token **una sola vez**: solo el token por stdout
(el aviso y los datos van a stderr) o, con `--json`, `{"token", "secreto",
"aviso", "ambiente"}`. tk nunca lo escribe en un archivo. Guárdalo como
variable de entorno de usuario `TRACKER_TOKEN` y configura el cliente como
dice la sección MCP del README de `tekitla-gateway`. Vence a los 30 días por
defecto (máximo 90, `--vence 7d` o `--vence 12h`). Para QA, `--ambiente qa`
en los dos comandos (la sugerencia de `tk agente nuevo --ambiente qa` ya lo
trae).

## Configuración

`~/.config/tk/config.toml` (o `$XDG_CONFIG_HOME/tk`, o `$TK_CONFIG_DIR`):

```toml
ambiente = "prod"

# Solo si tu binario no trae el cliente OAuth compilado (builds locales).
[oauth]
client_id = "…"
client_secret = "…"
```

La sesión queda en el llavero del sistema (Credential Manager, Keychain o
Secret Service); si no hay llavero, en `~/.config/tk/sesion.json` con permisos
`0600`.

## Desarrollo

```sh
make test   # sin red: servidor falso en proceso
make lint
make vuln
```

Contexto completo en [CLAUDE.md](CLAUDE.md). Issues: 029 y 030 en
[tekitla-docs](https://github.com/tekitla/tekitla-docs).
