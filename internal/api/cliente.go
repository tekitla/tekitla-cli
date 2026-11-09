// Package api es el cliente HTTP de tekitla-gateway (rutas REST de
// issues.v1.IssueService, docs/rutas.md del gateway).
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/tekitla/tekitla-cli/internal/auth"
)

// Tokens entrega el ID token de Firebase. forzar pide uno nuevo (después de
// un 401).
type Tokens interface {
	IDToken(ctx context.Context, forzar bool) (string, error)
}

// ErrorSesion: no hay sesión o ya no sirve. tk sale con código 2.
type ErrorSesion struct{ Causa error }

func (e *ErrorSesion) Error() string {
	return "no hay sesión o venció: corre tk login"
}

func (e *ErrorSesion) Unwrap() error { return e.Causa }

// ErrorAPI es un error de la API con el formato único del gateway:
// {"error": {"code", "message", "request_id"}}. Reason es el reason del
// google.rpc.ErrorInfo cuando el gateway lo propague.
type ErrorAPI struct {
	Status    int    `json:"status"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Reason    string `json:"reason,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func (e *ErrorAPI) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	s := fmt.Sprintf("%s (%s", msg, e.Code)
	if e.Reason != "" {
		s += ", " + e.Reason
	}
	return s + ")"
}

// Cliente habla con tekitla-gateway.
type Cliente struct {
	BaseURL   string
	HTTP      *http.Client
	Tokens    Tokens
	UserAgent string
}

// Nuevo arma un Cliente con timeout razonable.
func Nuevo(baseURL string, tokens Tokens, userAgent string) *Cliente {
	return &Cliente{
		BaseURL:   baseURL,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Tokens:    tokens,
		UserAgent: userAgent,
	}
}

const limitePagina = 100

func (c *Cliente) hacer(ctx context.Context, metodo, ruta string, q url.Values, cuerpo, salida any) error {
	var payload []byte
	if cuerpo != nil {
		var err error
		if payload, err = json.Marshal(cuerpo); err != nil {
			return fmt.Errorf("no pude armar el pedido: %w", err)
		}
	}
	for intento := 0; intento < 2; intento++ {
		tok, err := c.Tokens.IDToken(ctx, intento > 0)
		if err != nil {
			return errorDeToken(err)
		}
		status, body, err := c.enviar(ctx, metodo, ruta, q, payload, tok)
		if err != nil {
			return err
		}
		if status == http.StatusUnauthorized && intento == 0 {
			continue // el token pudo vencer o revocarse: se renueva una vez
		}
		if status == http.StatusUnauthorized {
			return &ErrorSesion{Causa: leerError(status, body)}
		}
		if status < 200 || status > 299 {
			return leerError(status, body)
		}
		if salida == nil {
			return nil
		}
		if err := json.Unmarshal(body, salida); err != nil {
			return fmt.Errorf("respuesta inesperada de la API (%s %s)", metodo, ruta)
		}
		return nil
	}
	return nil
}

func (c *Cliente) enviar(ctx context.Context, metodo, ruta string, q url.Values, payload []byte, tok string) (int, []byte, error) {
	u := c.BaseURL + ruta
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var cuerpo io.Reader
	if payload != nil {
		cuerpo = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, u, cuerpo)
	if err != nil {
		return 0, nil, fmt.Errorf("pedido inválido: %s %s", metodo, ruta)
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		return 0, nil, fmt.Errorf("no hay conexión con %s", req.URL.Host)
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("respuesta cortada de %s", req.URL.Host)
	}
	return res.StatusCode, body, nil
}

func leerError(status int, body []byte) *ErrorAPI {
	var r struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Reason    string `json:"reason"`
			RequestID string `json:"request_id"`
			Details   []struct {
				Type   string `json:"@type"`
				Reason string `json:"reason"`
			} `json:"details"`
		} `json:"error"`
	}
	e := &ErrorAPI{Status: status}
	if json.Unmarshal(body, &r) == nil {
		e.Code, e.Message, e.Reason, e.RequestID = r.Error.Code, r.Error.Message, r.Error.Reason, r.Error.RequestID
		for _, d := range r.Error.Details {
			if e.Reason == "" && d.Reason != "" {
				e.Reason = d.Reason
			}
		}
	}
	if e.Code == "" {
		e.Code = "HTTP_" + strconv.Itoa(status)
	}
	return e
}

func errorDeToken(err error) error {
	if errors.Is(err, auth.ErrSinSesion) || errors.Is(err, auth.ErrSesionInvalida) {
		return &ErrorSesion{Causa: err}
	}
	return err
}

// --- Métodos ----------------------------------------------------------------

// ListarProyectos trae todos los proyectos visibles (todas las páginas).
func (c *Cliente) ListarProyectos(ctx context.Context) ([]Proyecto, error) {
	var todos []Proyecto
	cursor := ""
	for {
		q := url.Values{"page.limit": {strconv.Itoa(limitePagina)}}
		if cursor != "" {
			q.Set("page.cursor", cursor)
		}
		var r struct {
			Projects []Proyecto `json:"projects"`
			Page     pagina     `json:"page"`
		}
		if err := c.hacer(ctx, http.MethodGet, "/v1/projects", q, nil, &r); err != nil {
			return nil, err
		}
		todos = append(todos, r.Projects...)
		if !r.Page.HasNext || r.Page.NextCursor == "" {
			return todos, nil
		}
		cursor = r.Page.NextCursor
	}
}

// ListarIssues trae los issues de un proyecto con filtros (todas las páginas).
func (c *Cliente) ListarIssues(ctx context.Context, proyectoID string, f FiltrosIssues) ([]Issue, error) {
	var todos []Issue
	cursor := ""
	for {
		q := url.Values{"page.limit": {strconv.Itoa(limitePagina)}}
		for _, s := range f.StatusIDs {
			q.Add("statusIds", s)
		}
		for _, p := range f.Priorities {
			q.Add("priorities", p)
		}
		if f.Query != "" {
			q.Set("query", f.Query)
		}
		if cursor != "" {
			q.Set("page.cursor", cursor)
		}
		var r struct {
			Issues []Issue `json:"issues"`
			Page   pagina  `json:"page"`
		}
		if err := c.hacer(ctx, http.MethodGet, rutaProyecto(proyectoID)+"/issues", q, nil, &r); err != nil {
			return nil, err
		}
		todos = append(todos, r.Issues...)
		if !r.Page.HasNext || r.Page.NextCursor == "" {
			return todos, nil
		}
		cursor = r.Page.NextCursor
	}
}

// ObtenerIssue es GetIssue.
func (c *Cliente) ObtenerIssue(ctx context.Context, proyectoID string, numero int) (Issue, error) {
	var r struct {
		Issue Issue `json:"issue"`
	}
	err := c.hacer(ctx, http.MethodGet, rutaIssue(proyectoID, numero), nil, nil, &r)
	return r.Issue, err
}

// CrearIssue es CreateIssue.
func (c *Cliente) CrearIssue(ctx context.Context, proyectoID string, n NuevoIssue) (Issue, error) {
	var r struct {
		Issue Issue `json:"issue"`
	}
	err := c.hacer(ctx, http.MethodPost, rutaProyecto(proyectoID)+"/issues", nil, n, &r)
	return r.Issue, err
}

// MoverIssue es MoveIssue a un estado concreto.
func (c *Cliente) MoverIssue(ctx context.Context, proyectoID string, numero int, estadoID string) (Issue, error) {
	var r struct {
		Issue Issue `json:"issue"`
	}
	err := c.hacer(ctx, http.MethodPost, rutaIssue(proyectoID, numero)+":move", nil,
		map[string]string{"statusId": estadoID}, &r)
	return r.Issue, err
}

// Comentar es CreateComment.
func (c *Cliente) Comentar(ctx context.Context, proyectoID string, numero int, texto string) (Comentario, error) {
	var r struct {
		Comment Comentario `json:"comment"`
	}
	err := c.hacer(ctx, http.MethodPost, rutaIssue(proyectoID, numero)+"/comments", nil,
		map[string]string{"body": texto}, &r)
	return r.Comment, err
}

// ListarEstados es ListStatuses (sin paginar, máximo 20).
func (c *Cliente) ListarEstados(ctx context.Context, proyectoID string) ([]Estado, error) {
	var r struct {
		Statuses []Estado `json:"statuses"`
	}
	err := c.hacer(ctx, http.MethodGet, rutaProyecto(proyectoID)+"/statuses", nil, nil, &r)
	return r.Statuses, err
}

// ListarEtiquetas es ListLabels (sin paginar, máximo 100).
func (c *Cliente) ListarEtiquetas(ctx context.Context, proyectoID string) ([]Etiqueta, error) {
	var r struct {
		Labels []Etiqueta `json:"labels"`
	}
	err := c.hacer(ctx, http.MethodGet, rutaProyecto(proyectoID)+"/labels", nil, nil, &r)
	return r.Labels, err
}

// ListarEventos trae el historial completo de un issue, del más viejo al más
// nuevo.
func (c *Cliente) ListarEventos(ctx context.Context, proyectoID string, numero int) ([]Evento, error) {
	var todos []Evento
	cursor := ""
	for {
		q := url.Values{"page.limit": {strconv.Itoa(limitePagina)}}
		if cursor != "" {
			q.Set("page.cursor", cursor)
		}
		var r struct {
			Events []Evento `json:"events"`
			Page   pagina   `json:"page"`
		}
		if err := c.hacer(ctx, http.MethodGet, rutaIssue(proyectoID, numero)+"/events", q, nil, &r); err != nil {
			return nil, err
		}
		todos = append(todos, r.Events...)
		if !r.Page.HasNext || r.Page.NextCursor == "" {
			return todos, nil
		}
		cursor = r.Page.NextCursor
	}
}

func rutaProyecto(id string) string { return "/v1/projects/" + url.PathEscape(id) }

func rutaIssue(id string, numero int) string {
	return rutaProyecto(id) + "/issues/" + strconv.Itoa(numero)
}
