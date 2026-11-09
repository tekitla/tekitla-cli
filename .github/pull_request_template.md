## Qué cambia

<!-- Una o dos líneas. El título va con el issue: feat(issue-<n>): … (chore(deps) sin issue). -->

Issue: issue-N  <!-- carpeta contenido/issues/N-…/ del repo de docs; obligatorio salvo chore(deps) -->

## Cómo se probó

- [ ] `make lint`
- [ ] `make test`
- [ ] `make vuln`
- [ ] Probado a mano con `make build` + `bin/tk …` (`bin\tk.exe` en Windows) (comandos tocados) / no aplica

## Checklist

- [ ] Sin secretos ni datos personales en el diff (gitleaks corre en el CI igual)
- [ ] Ningún token (ID token, refresh token) se imprime ni se escribe en logs
- [ ] La salida `--json` solo agrega campos (no renombra ni quita) / no aplica
- [ ] Docs del repo actualizadas (README, CLAUDE.md) / no aplica
- [ ] Ficha o ADR en el repo de docs actualizados / no aplica
- [ ] Avance del issue registrado en el repo de docs (PR: …) / no aplica
- [ ] Si el cambio sirve a otros proyectos, PR en dev-plantillas (link) / no aplica

## Notas para el review

<!-- Decisiones, deuda que queda, PR relacionados en otros repos. -->
