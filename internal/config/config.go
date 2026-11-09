// Package config resuelve el ambiente, la URL de la API y el cliente OAuth de
// tk a partir de los flags, las variables de entorno y ~/.config/tk/config.toml.
//
// Prioridad (de mayor a menor): flag > variable de entorno > config.toml >
// valor compilado (ldflags) > default.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Ambientes conocidos y la URL de su API.
const (
	AmbienteProd = "prod"
	AmbienteQA   = "qa"
)

var urlsAPI = map[string]string{
	AmbienteProd: "https://api.tekitla.victorrb1015.com",
	AmbienteQA:   "https://api-qa.tekitla.victorrb1015.com",
}

// Variables de entorno.
const (
	EnvAmbiente     = "TK_AMBIENTE"
	EnvConfigDir    = "TK_CONFIG_DIR"
	EnvAPIURL       = "TK_API_URL" // solo para desarrollo: pisa la URL del ambiente
	EnvClientID     = "TK_OAUTH_CLIENT_ID"
	EnvClientSecret = "TK_OAUTH_CLIENT_SECRET" //nolint:gosec // G101: nombre de la variable, no su valor
)

// Compilado con -ldflags (GoReleaser lo llena desde variables del CI). Vacío
// en un `go build` local: se configura en config.toml o con las variables de
// entorno.
var (
	OAuthClientID     = ""
	OAuthClientSecret = ""
)

// Archivo es el contenido de config.toml.
type Archivo struct {
	Ambiente string     `toml:"ambiente,omitempty"`
	OAuth    OAuthTOML  `toml:"oauth,omitempty"`
	API      APIArchivo `toml:"api,omitempty"`
}

// OAuthTOML es la sección [oauth]: el cliente OAuth de Google de tipo «TVs y
// dispositivos de entrada limitada». Su secret no es un secreto real (ADR
// 0005), pero igual no se commitea: va en ldflags o en este archivo local.
type OAuthTOML struct {
	ClientID     string `toml:"client_id,omitempty"`
	ClientSecret string `toml:"client_secret,omitempty"`
}

// APIArchivo es la sección [api].
type APIArchivo struct {
	// URL pisa la del ambiente (desarrollo local).
	URL string `toml:"url,omitempty"`
}

// Config es la configuración resuelta.
type Config struct {
	Dir               string
	Ambiente          string
	APIURL            string
	OAuthClientID     string
	OAuthClientSecret string
}

// Dir devuelve la carpeta de configuración: $TK_CONFIG_DIR, o
// $XDG_CONFIG_HOME/tk, o ~/.config/tk (también en Windows y macOS, como dice
// el plan del 002).
func Dir() (string, error) {
	if d := os.Getenv(EnvConfigDir); d != "" {
		return d, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "tk"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("no encuentro la carpeta del usuario: %w", err)
	}
	return filepath.Join(home, ".config", "tk"), nil
}

// Leer lee config.toml de dir. Si no existe devuelve un Archivo vacío.
func Leer(dir string) (Archivo, error) {
	var a Archivo
	b, err := os.ReadFile(filepath.Join(dir, "config.toml")) //nolint:gosec // G304: la carpeta de config la elige el usuario
	if errors.Is(err, fs.ErrNotExist) {
		return a, nil
	}
	if err != nil {
		return a, fmt.Errorf("leyendo config.toml: %w", err)
	}
	if err := toml.Unmarshal(b, &a); err != nil {
		return a, fmt.Errorf("config.toml inválido: %w", err)
	}
	return a, nil
}

// Resolver arma la Config. flagAmbiente vacío significa «no vino el flag».
func Resolver(flagAmbiente string) (Config, error) {
	dir, err := Dir()
	if err != nil {
		return Config{}, err
	}
	a, err := Leer(dir)
	if err != nil {
		return Config{}, err
	}

	amb := primero(flagAmbiente, os.Getenv(EnvAmbiente), a.Ambiente, AmbienteProd)
	amb = strings.ToLower(strings.TrimSpace(amb))
	url, ok := urlsAPI[amb]
	if !ok {
		return Config{}, fmt.Errorf("ambiente %q desconocido: usa qa o prod", amb)
	}
	url = primero(os.Getenv(EnvAPIURL), a.API.URL, url)

	return Config{
		Dir:               dir,
		Ambiente:          amb,
		APIURL:            strings.TrimRight(url, "/"),
		OAuthClientID:     primero(os.Getenv(EnvClientID), a.OAuth.ClientID, OAuthClientID),
		OAuthClientSecret: primero(os.Getenv(EnvClientSecret), a.OAuth.ClientSecret, OAuthClientSecret),
	}, nil
}

// URLAPI devuelve la URL de la API de un ambiente conocido.
func URLAPI(ambiente string) (string, bool) {
	u, ok := urlsAPI[ambiente]
	return u, ok
}

func primero(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
