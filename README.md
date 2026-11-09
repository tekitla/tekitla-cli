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
tk version
tk logout
```

En todos los comandos:

- `--json`: salida estable para agentes y scripts.
- `--ambiente qa|prod`: prod por defecto (también `TK_AMBIENTE` o
  `ambiente = "qa"` en la configuración).

Códigos de salida: `0` bien, `1` error de uso o de la API, `2` sin sesión
(corre `tk login`).

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

Contexto completo en [CLAUDE.md](CLAUDE.md). Issue: 029 en
[tekitla-docs](https://github.com/tekitla/tekitla-docs).
