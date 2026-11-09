package config

import (
	"os"
	"path/filepath"
	"testing"
)

func limpiar(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(EnvConfigDir, dir)
	for _, e := range []string{EnvAmbiente, EnvAPIURL, EnvClientID, EnvClientSecret} {
		t.Setenv(e, "")
	}
	return dir
}

func TestPrioridad(t *testing.T) {
	dir := limpiar(t)
	toml := "ambiente = \"qa\"\n[oauth]\nclient_id = \"id-toml\"\nclient_secret = \"s-toml\"\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(toml), 0o600); err != nil {
		t.Fatal(err)
	}
	OAuthClientID = "id-ldflags"
	t.Cleanup(func() { OAuthClientID = "" })

	c, err := Resolver("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Ambiente != "qa" || c.APIURL != "https://api-qa.tekitla.victorrb1015.com" {
		t.Errorf("config.toml: %+v", c)
	}
	if c.OAuthClientID != "id-toml" || c.OAuthClientSecret != "s-toml" {
		t.Errorf("oauth: %+v", c)
	}

	t.Setenv(EnvAmbiente, "prod")
	t.Setenv(EnvClientID, "id-env")
	if c, _ = Resolver(""); c.Ambiente != "prod" || c.OAuthClientID != "id-env" {
		t.Errorf("env: %+v", c)
	}
	if c, _ = Resolver("QA"); c.Ambiente != "qa" {
		t.Errorf("flag: %+v", c)
	}
	t.Setenv(EnvAPIURL, "http://localhost:8080/")
	if c, _ = Resolver("qa"); c.APIURL != "http://localhost:8080" {
		t.Errorf("TK_API_URL: %+v", c)
	}
}

func TestLdflagsSinConfig(t *testing.T) {
	limpiar(t)
	OAuthClientID, OAuthClientSecret = "id-ldflags", "s-ldflags"
	t.Cleanup(func() { OAuthClientID, OAuthClientSecret = "", "" })
	c, err := Resolver("")
	if err != nil {
		t.Fatal(err)
	}
	if c.Ambiente != "prod" || c.OAuthClientID != "id-ldflags" || c.OAuthClientSecret != "s-ldflags" {
		t.Errorf("%+v", c)
	}
}

func TestConfigInvalida(t *testing.T) {
	dir := limpiar(t)
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("ambiente = \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Resolver(""); err == nil {
		t.Error("quiero error con un config.toml inválido")
	}
}
