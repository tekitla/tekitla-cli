package config

// Configuración pública del proyecto de Firebase tekitla-app: la misma que usa
// el panel (tekitla-web/src/environments). La apiKey web de Firebase NO es un
// secreto: viaja en el bundle del panel y la protegen las restricciones de la
// clave y la allowlist del gateway. .gitleaks.toml la permite solo en este
// archivo.
const (
	FirebaseProjectID = "tekitla-app"
	FirebaseAPIKey    = "AIzaSyBkK6lhxg2dU0aX8Jqr-2VEfKGlx7319E8" //nolint:gosec // G101: pública por diseño (ver arriba)
)
