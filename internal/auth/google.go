// Package auth implementa el login de tk (ADR 0005): device flow de Google
// OAuth canjeado por una sesión de Firebase, el almacén de la sesión y la
// renovación del ID token.
//
// Ningún error de este paquete incluye tokens, códigos de dispositivo ni URLs
// con la apiKey: solo códigos de error de Google o Firebase.
package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoints de Google y Firebase. Los tests los apuntan a un servidor falso.
type Endpoints struct {
	DeviceCode    string // POST: pide el código de dispositivo
	Token         string // POST: canjea el código de dispositivo
	SignInWithIdp string // POST: accounts:signInWithIdp (sin ?key=)
	SecureToken   string // POST: renueva el ID token (sin ?key=)
}

// EndpointsReales son los de producción de Google.
var EndpointsReales = Endpoints{ //nolint:gosec // G101: URLs públicas, no credenciales
	DeviceCode:    "https://oauth2.googleapis.com/device/code",
	Token:         "https://oauth2.googleapis.com/token",
	SignInWithIdp: "https://identitytoolkit.googleapis.com/v1/accounts:signInWithIdp",
	SecureToken:   "https://securetoken.googleapis.com/v1/token",
}

// ErrSinClienteOAuth: falta el cliente OAuth de tipo dispositivo.
var ErrSinClienteOAuth = errors.New("falta el cliente OAuth de Google para el device flow: " +
	"configúralo en [oauth] de config.toml o con TK_OAUTH_CLIENT_ID y TK_OAUTH_CLIENT_SECRET")

// CodigoDispositivo es la respuesta de Google al pedir un código.
type CodigoDispositivo struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// Google es el cliente del device flow.
type Google struct {
	HTTP         *http.Client
	Endpoints    Endpoints
	ClientID     string
	ClientSecret string
	// Dormir espera entre sondeos; los tests la reemplazan.
	Dormir func(ctx context.Context, d time.Duration) error
}

// PedirCodigo inicia el device flow (scopes openid email).
func (g *Google) PedirCodigo(ctx context.Context) (CodigoDispositivo, error) {
	var c CodigoDispositivo
	if g.ClientID == "" || g.ClientSecret == "" {
		return c, ErrSinClienteOAuth
	}
	form := url.Values{"client_id": {g.ClientID}, "scope": {"openid email"}}
	status, body, err := postForm(ctx, g.HTTP, g.Endpoints.DeviceCode, form)
	if err != nil {
		return c, err
	}
	if status != http.StatusOK {
		return c, fmt.Errorf("google rechazó el pedido de código: %s", errorOAuth(body, status))
	}
	if err := json.Unmarshal(body, &c); err != nil || c.DeviceCode == "" || c.UserCode == "" {
		return c, errors.New("respuesta inesperada de google al pedir el código")
	}
	if c.Interval <= 0 {
		c.Interval = 5
	}
	return c, nil
}

// EsperarAprobacion sondea hasta que el usuario aprueba el código y devuelve
// el ID token de Google.
func (g *Google) EsperarAprobacion(ctx context.Context, c CodigoDispositivo) (string, error) {
	dormir := g.Dormir
	if dormir == nil {
		dormir = dormirCtx
	}
	intervalo := time.Duration(c.Interval) * time.Second
	limite := time.Now().Add(time.Duration(c.ExpiresIn) * time.Second)
	form := url.Values{
		"client_id":     {g.ClientID},
		"client_secret": {g.ClientSecret},
		"device_code":   {c.DeviceCode},
		"grant_type":    {"urn:ietf:params:oauth:grant-type:device_code"},
	}
	for {
		if c.ExpiresIn > 0 && time.Now().After(limite) {
			return "", errors.New("el código venció antes de aprobarse: corre tk login otra vez")
		}
		if err := dormir(ctx, intervalo); err != nil {
			return "", err
		}
		status, body, err := postForm(ctx, g.HTTP, g.Endpoints.Token, form)
		if err != nil {
			return "", err
		}
		if status == http.StatusOK {
			var r struct {
				IDToken string `json:"id_token"`
			}
			if err := json.Unmarshal(body, &r); err != nil || r.IDToken == "" {
				return "", errors.New("google aprobó pero no devolvió un id_token")
			}
			return r.IDToken, nil
		}
		switch codigo := codigoOAuth(body); codigo {
		case "authorization_pending":
		case "slow_down":
			intervalo += 5 * time.Second
		case "access_denied":
			return "", errors.New("se rechazó el acceso en google")
		case "expired_token":
			return "", errors.New("el código venció antes de aprobarse: corre tk login otra vez")
		default:
			return "", fmt.Errorf("google rechazó el login: %s", errorOAuth(body, status))
		}
	}
}

func dormirCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func postForm(ctx context.Context, c *http.Client, endpoint string, form url.Values) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return 0, nil, errors.New("no pude armar el pedido de login")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return hacer(c, req)
}

func hacer(c *http.Client, req *http.Request) (int, []byte, error) {
	if c == nil {
		c = http.DefaultClient
	}
	res, err := c.Do(req)
	if err != nil {
		// El error de net/http incluye la URL (con ?key=): solo el host.
		return 0, nil, fmt.Errorf("no hay conexión con %s", req.URL.Host)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("respuesta cortada de %s", req.URL.Host)
	}
	return res.StatusCode, body, nil
}

func codigoOAuth(body []byte) string {
	var r struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Error
}

func errorOAuth(body []byte, status int) string {
	if c := codigoOAuth(body); c != "" {
		return c
	}
	return fmt.Sprintf("HTTP %d", status)
}
