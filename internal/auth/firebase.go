package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Firebase canjea el ID token de Google por una sesión de Firebase y la
// renueva.
type Firebase struct {
	HTTP      *http.Client
	Endpoints Endpoints
	APIKey    string
	Ahora     func() time.Time
}

// ErrSesionInvalida: el refresh token ya no sirve (revocado, vencido o
// usuario deshabilitado). Hace falta tk login.
var ErrSesionInvalida = errors.New("la sesión venció o fue revocada")

func (f *Firebase) ahora() time.Time {
	if f.Ahora != nil {
		return f.Ahora()
	}
	return time.Now()
}

// Canjear llama a accounts:signInWithIdp con el ID token de Google.
func (f *Firebase) Canjear(ctx context.Context, googleIDToken string) (Sesion, error) {
	payload, _ := json.Marshal(map[string]any{
		"postBody":          url.Values{"id_token": {googleIDToken}, "providerId": {"google.com"}}.Encode(),
		"requestUri":        "http://localhost",
		"returnSecureToken": true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		f.Endpoints.SignInWithIdp+"?key="+url.QueryEscape(f.APIKey), bytes.NewReader(payload))
	if err != nil {
		return Sesion{}, errors.New("no pude armar el canje en firebase")
	}
	req.Header.Set("Content-Type", "application/json")
	status, body, err := hacer(f.HTTP, req)
	if err != nil {
		return Sesion{}, err
	}
	if status != http.StatusOK {
		return Sesion{}, fmt.Errorf("firebase rechazó el login: %s", mensajeFirebase(body, status))
	}
	var r struct {
		IDToken      string `json:"idToken"`
		RefreshToken string `json:"refreshToken"`
		ExpiresIn    string `json:"expiresIn"`
		Email        string `json:"email"`
		LocalID      string `json:"localId"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.IDToken == "" || r.RefreshToken == "" {
		return Sesion{}, errors.New("respuesta inesperada de firebase al canjear")
	}
	return Sesion{
		IDToken:      r.IDToken,
		RefreshToken: r.RefreshToken,
		Expira:       f.ahora().Add(segundos(r.ExpiresIn)),
		Email:        r.Email,
		UID:          r.LocalID,
	}, nil
}

// Renovar pide un ID token nuevo con el refresh token. Devuelve
// ErrSesionInvalida si el refresh token ya no sirve.
func (f *Firebase) Renovar(ctx context.Context, s Sesion) (Sesion, error) {
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {s.RefreshToken}}
	status, body, err := postForm(ctx, f.HTTP, f.Endpoints.SecureToken+"?key="+url.QueryEscape(f.APIKey), form)
	if err != nil {
		return s, err
	}
	if status != http.StatusOK {
		if status == http.StatusBadRequest || status == http.StatusUnauthorized || status == http.StatusForbidden {
			switch mensajeFirebase(body, status) {
			case "TOKEN_EXPIRED", "INVALID_REFRESH_TOKEN", "USER_DISABLED", "USER_NOT_FOUND", "INVALID_GRANT_TYPE", "MISSING_REFRESH_TOKEN":
				return s, ErrSesionInvalida
			}
		}
		return s, fmt.Errorf("firebase no renovó la sesión: %s", mensajeFirebase(body, status))
	}
	var r struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
		UserID       string `json:"user_id"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.IDToken == "" {
		return s, errors.New("respuesta inesperada de firebase al renovar")
	}
	s.IDToken = r.IDToken
	if r.RefreshToken != "" {
		s.RefreshToken = r.RefreshToken
	}
	if r.UserID != "" {
		s.UID = r.UserID
	}
	s.Expira = f.ahora().Add(segundos(r.ExpiresIn))
	return s, nil
}

// mensajeFirebase saca el código de error ({"error":{"message":"..."}}).
// Los mensajes de Firebase son códigos (INVALID_IDP_RESPONSE, …), sin datos
// del pedido.
func mensajeFirebase(body []byte, status int) string {
	var r struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &r) == nil && r.Error.Message != "" {
		// "TOKEN_EXPIRED : detalle" → "TOKEN_EXPIRED"
		m := r.Error.Message
		for i := 0; i < len(m); i++ {
			if m[i] == ' ' || m[i] == ':' {
				return m[:i]
			}
		}
		return m
	}
	return fmt.Sprintf("HTTP %d", status)
}

func segundos(s string) time.Duration {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		n = 3600
	}
	return time.Duration(n) * time.Second
}
