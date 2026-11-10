package api

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// Rutas de issues.v1.ActorService (issue 025, ADR 0009). El gateway las
// acepta solo con la sesión de Firebase: un token tka_ no crea agentes ni
// otros tokens.

// Actor es issues.v1.Actor.
type Actor struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"` // ACTOR_KIND_PERSON | ACTOR_KIND_AGENT
	DisplayName  string     `json:"displayName"`
	OwnerActorID string     `json:"ownerActorId"`
	Tool         string     `json:"tool"` // AGENT_TOOL_*
	CreatedAt    time.Time  `json:"createdAt"`
	RevokedAt    *time.Time `json:"revokedAt"`
}

// Token es issues.v1.Token: nunca trae el secreto.
type Token struct {
	ID         string     `json:"id"`
	ActorID    string     `json:"actorId"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"` // TOKEN_SCOPE_*
	CreatedAt  time.Time  `json:"createdAt"`
	ExpiresAt  time.Time  `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

// NuevoToken es el cuerpo de CreateToken. ExpiresAt nil: 30 días (default
// del servicio).
type NuevoToken struct {
	ActorID   string     `json:"actorId,omitempty"`
	Name      string     `json:"name,omitempty"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// ListarActores es ListActors: la persona (primero) y sus agentes.
func (c *Cliente) ListarActores(ctx context.Context, incluirRevocados bool) ([]Actor, error) {
	q := url.Values{}
	if incluirRevocados {
		q.Set("includeRevoked", "true")
	}
	var r struct {
		Actors []Actor `json:"actors"`
	}
	err := c.hacer(ctx, http.MethodGet, "/v1/actors", q, nil, &r)
	return r.Actors, err
}

// CrearAgente es CreateAgent para la persona de la sesión.
func (c *Cliente) CrearAgente(ctx context.Context, nombre, herramienta string) (Actor, error) {
	var r struct {
		Agent Actor `json:"agent"`
	}
	err := c.hacer(ctx, http.MethodPost, "/v1/agents", nil,
		map[string]string{"displayName": nombre, "tool": herramienta}, &r)
	return r.Agent, err
}

// RevocarActor es RevokeActor (en cascada: revoca sus tokens). No se deshace.
func (c *Cliente) RevocarActor(ctx context.Context, actorID string) (Actor, error) {
	var r struct {
		Actor Actor `json:"actor"`
	}
	err := c.hacer(ctx, http.MethodPost, "/v1/actors/"+url.PathEscape(actorID)+":revoke", nil, map[string]string{}, &r)
	return r.Actor, err
}

// CrearToken es CreateToken. El secreto viene solo en esta respuesta.
func (c *Cliente) CrearToken(ctx context.Context, n NuevoToken) (Token, string, error) {
	var r struct {
		Token  Token  `json:"token"`
		Secret string `json:"secret"`
	}
	err := c.hacer(ctx, http.MethodPost, "/v1/tokens", nil, n, &r)
	return r.Token, r.Secret, err
}

// ListarTokens es ListTokens: los de la persona y sus agentes, sin secreto.
func (c *Cliente) ListarTokens(ctx context.Context, incluirInactivos bool) ([]Token, error) {
	q := url.Values{}
	if incluirInactivos {
		q.Set("includeInactive", "true")
	}
	var r struct {
		Tokens []Token `json:"tokens"`
	}
	err := c.hacer(ctx, http.MethodGet, "/v1/tokens", q, nil, &r)
	return r.Tokens, err
}

// RevocarToken es RevokeToken. No se deshace; el gateway puede seguir
// aceptándolo hasta 60 s (su caché).
func (c *Cliente) RevocarToken(ctx context.Context, tokenID string) (Token, error) {
	var r struct {
		Token Token `json:"token"`
	}
	err := c.hacer(ctx, http.MethodPost, "/v1/tokens/"+url.PathEscape(tokenID)+":revoke", nil, map[string]string{}, &r)
	return r.Token, err
}
