package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tekitla/tekitla-cli/internal/auth"
	"github.com/tekitla/tekitla-cli/internal/config"
)

// Tokens falsos: todos contienen SECRETO para poder buscar fugas (CU-029-7).
const (
	tokGoogle    = "google-id-token-SECRETO"
	tokRefresh   = "fb-refresh-SECRETO"
	tokDevice    = "device-code-SECRETO"
	clientSecret = "client-secret-SECRETO"
)

// pedido es un request que llegó al falso.
type pedido struct {
	Metodo string
	Host   string
	Ruta   string
	Query  url.Values
	Cuerpo string
}

// falso simula Google (device flow), Firebase y tekitla-gateway en proceso:
// es el Transport del http.Client de tk, así ningún test sale a la red.
type falso struct {
	t  *testing.T
	mu sync.Mutex

	pedidos []pedido

	// Google
	pendientes int // respuestas authorization_pending antes de aprobar
	// Firebase
	idVigente       string // el único ID token que la API acepta
	generacion      int
	renovaciones    int
	refreshInvalido bool
	// API
	proyectos []map[string]any
	estados   []map[string]any
	etiquetas []map[string]any
	issues    map[int]map[string]any
	eventos   []map[string]any
	actores   []map[string]any
	tokens    []map[string]any
	// errorAPI fuerza una respuesta de error en la API.
	errorAPI *errorFalso
	// rechazar401 responde 401 a la próxima llamada de la API.
	rechazar401 int
}

type errorFalso struct {
	status int
	code   string
	msg    string
}

func nuevoFalso(t *testing.T) *falso {
	f := &falso{t: t, idVigente: "fb-id-token-SECRETO-0"}
	f.proyectos = []map[string]any{
		{"id": "p-dp", "key": "DP", "name": "Dos Delicias", "description": "", "workspaceId": "w1"},
		{"id": "p-tk", "key": "TK", "name": "Tekitla", "description": "El tracker", "workspaceId": "w1"},
	}
	f.estados = []map[string]any{
		{"id": "s1", "name": "Por hacer", "category": "ISSUE_STATE_TODO", "hidden": false},
		{"id": "s2", "name": "En curso", "category": "ISSUE_STATE_IN_PROGRESS", "hidden": false},
		{"id": "s3", "name": "Hecho", "category": "ISSUE_STATE_DONE", "hidden": false},
		{"id": "s4", "name": "Descartado", "category": "ISSUE_STATE_CANCELED", "hidden": true},
	}
	f.etiquetas = []map[string]any{{"id": "l1", "name": "backend", "color": "LABEL_COLOR_INFO"}}
	f.issues = map[int]map[string]any{}
	for n, titulo := range []string{"", "Pan de muerto", "Rosca", "Conchas"} {
		if n == 0 {
			continue
		}
		f.issues[n] = issueFalso(n, titulo, "s1", "ISSUE_STATE_TODO", "PRIORITY_MEDIUM")
	}
	f.issues[3]["priority"] = "PRIORITY_HIGH"
	f.issues[3]["labelIds"] = []string{"l1"}
	f.issues[3]["description"] = "Con azúcar"
	f.eventos = []map[string]any{
		{"id": "e1", "actorId": "u1", "agentId": "", "createdAt": "2026-11-01T10:00:00Z", "created": map[string]any{}},
		{"id": "e2", "actorId": "u1", "agentId": "", "createdAt": "2026-11-02T10:00:00Z",
			"commented": map[string]any{"id": "c1", "body": "Primera línea\nsegunda", "authorId": "u1"}},
	}
	f.actores = []map[string]any{
		{"id": "a-persona", "kind": "ACTOR_KIND_PERSON", "displayName": "", "userId": "u1", "createdAt": "2026-11-01T10:00:00Z"},
		{"id": "a-codex", "kind": "ACTOR_KIND_AGENT", "displayName": "Codex PC-01", "ownerActorId": "a-persona",
			"tool": "AGENT_TOOL_CODEX", "createdAt": "2026-11-02T10:00:00Z"},
		{"id": "a-viejo", "kind": "ACTOR_KIND_AGENT", "displayName": "Viejo", "ownerActorId": "a-persona",
			"tool": "AGENT_TOOL_OTHER", "createdAt": "2026-11-02T10:00:00Z", "revokedAt": "2026-11-03T10:00:00Z"},
	}
	f.tokens = []map[string]any{
		{"id": "t-codex", "actorId": "a-codex", "name": "Codex PC-01", "scopes": []string{"TOKEN_SCOPE_ISSUES_READ"},
			"createdAt": "2026-11-02T10:00:00Z", "expiresAt": "2026-12-02T10:00:00Z", "lastUsedAt": "2026-11-08T10:00:00Z"},
	}
	return f
}

// secretoToken es el valor en claro que devuelve el falso al crear un token:
// tiene que salir una sola vez (tk token nuevo) y en ningún archivo.
const secretoToken = "tka_VALOR-EN-CLARO-del-token-de-prueba-0000000"

func revocado(m map[string]any) bool { _, ok := m["revokedAt"]; return ok }

// actoresTokens atiende las rutas de ActorService. false: no es una de ellas.
func (f *falso) actoresTokens(w http.ResponseWriter, r *http.Request, cuerpo []byte) bool {
	ruta, accion, _ := strings.Cut(r.URL.Path, ":")
	switch {
	case ruta == "/v1/actors" && r.Method == http.MethodGet:
		out := []map[string]any{}
		for _, ac := range f.actores {
			if !revocado(ac) || r.URL.Query().Get("includeRevoked") == "true" {
				out = append(out, ac)
			}
		}
		responder(w, 200, map[string]any{"actors": out})
	case ruta == "/v1/agents" && r.Method == http.MethodPost:
		var b map[string]any
		_ = json.Unmarshal(cuerpo, &b)
		for _, ac := range f.actores {
			if !revocado(ac) && strings.EqualFold(fmt.Sprint(ac["displayName"]), fmt.Sprint(b["displayName"])) {
				errorGateway(w, 409, "ALREADY_EXISTS", "ya hay un agente con ese nombre")
				return true
			}
		}
		ag := map[string]any{"id": fmt.Sprintf("a-%d", len(f.actores)+1), "kind": "ACTOR_KIND_AGENT",
			"displayName": b["displayName"], "tool": b["tool"], "ownerActorId": "a-persona", "createdAt": "2026-11-09T12:00:00Z"}
		f.actores = append(f.actores, ag)
		responder(w, 200, map[string]any{"agent": ag})
	case strings.HasPrefix(ruta, "/v1/actors/") && accion == "revoke" && r.Method == http.MethodPost:
		id := strings.TrimPrefix(ruta, "/v1/actors/")
		for _, ac := range f.actores {
			if ac["id"] == id {
				ac["revokedAt"] = "2026-11-09T12:00:00Z"
				for _, t := range f.tokens {
					if t["actorId"] == id {
						t["revokedAt"] = "2026-11-09T12:00:00Z"
					}
				}
				responder(w, 200, map[string]any{"actor": ac})
				return true
			}
		}
		errorGateway(w, 404, "NOT_FOUND", "actor no encontrado")
	case ruta == "/v1/tokens" && r.Method == http.MethodPost:
		var b map[string]any
		_ = json.Unmarshal(cuerpo, &b)
		vence, _ := b["expiresAt"].(string)
		if vence == "" {
			vence = "2026-12-09T12:00:00Z"
		}
		tok := map[string]any{"id": fmt.Sprintf("t-%d", len(f.tokens)+1), "actorId": b["actorId"], "name": b["name"],
			"scopes": b["scopes"], "createdAt": "2026-11-09T12:00:00Z", "expiresAt": vence}
		f.tokens = append(f.tokens, tok)
		responder(w, 200, map[string]any{"token": tok, "secret": secretoToken})
	case ruta == "/v1/tokens" && r.Method == http.MethodGet:
		out := []map[string]any{}
		for i := len(f.tokens) - 1; i >= 0; i-- {
			if !revocado(f.tokens[i]) || r.URL.Query().Get("includeInactive") == "true" {
				out = append(out, f.tokens[i])
			}
		}
		responder(w, 200, map[string]any{"tokens": out})
	case strings.HasPrefix(ruta, "/v1/tokens/") && accion == "revoke" && r.Method == http.MethodPost:
		id := strings.TrimPrefix(ruta, "/v1/tokens/")
		for _, t := range f.tokens {
			if t["id"] == id {
				t["revokedAt"] = "2026-11-09T12:00:00Z"
				responder(w, 200, map[string]any{"token": t})
				return true
			}
		}
		errorGateway(w, 404, "NOT_FOUND", "token no encontrado")
	default:
		return false
	}
	return true
}

func issueFalso(n int, titulo, statusID, state, prio string) map[string]any {
	return map[string]any{
		"id": "i" + strconv.Itoa(n), "projectId": "p-dp", "number": n, "title": titulo,
		"description": "", "state": state, "priority": prio, "statusId": statusID,
		"labelIds": []string{}, "createdAt": "2026-11-01T10:00:00Z", "updatedAt": "2026-11-01T10:00:00Z",
		"rank": "a" + strconv.Itoa(n), "assigneeActorId": "", "sourceKey": "", "discard": nil,
	}
}

func (f *falso) RoundTrip(req *http.Request) (*http.Response, error) {
	var cuerpo []byte
	if req.Body != nil {
		cuerpo, _ = io.ReadAll(req.Body)
	}
	f.mu.Lock()
	f.pedidos = append(f.pedidos, pedido{req.Method, req.URL.Host, req.URL.Path, req.URL.Query(), string(cuerpo)})
	f.mu.Unlock()
	rec := httptest.NewRecorder()
	req.Body = io.NopCloser(bytes.NewReader(cuerpo))
	f.atender(rec, req, cuerpo)
	return rec.Result(), nil
}

func (f *falso) atender(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Host {
	case "google.falso":
		f.google(w, r)
	case "firebase.falso":
		f.firebase(w, r, cuerpo)
	default:
		f.api(w, r, cuerpo)
	}
}

func responder(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (f *falso) google(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	switch r.URL.Path {
	case "/device/code":
		if r.PostForm.Get("client_id") != "cliente-tv" || r.PostForm.Get("scope") != "openid email" {
			responder(w, 400, map[string]string{"error": "invalid_client"})
			return
		}
		responder(w, 200, map[string]any{
			"device_code": tokDevice, "user_code": "ABCD-EFGH",
			"verification_url": "https://www.google.com/device", "expires_in": 1800, "interval": 5,
		})
	case "/token":
		if r.PostForm.Get("device_code") != tokDevice || r.PostForm.Get("client_secret") != clientSecret {
			responder(w, 400, map[string]string{"error": "invalid_grant"})
			return
		}
		if f.pendientes > 0 {
			f.pendientes--
			responder(w, 428, map[string]string{"error": "authorization_pending"})
			return
		}
		responder(w, 200, map[string]any{"id_token": tokGoogle, "access_token": "at-SECRETO"})
	default:
		responder(w, 404, nil)
	}
}

func (f *falso) nuevoID() string {
	f.generacion++
	f.idVigente = fmt.Sprintf("fb-id-token-SECRETO-%d", f.generacion)
	return f.idVigente
}

func (f *falso) firebase(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	if r.URL.Query().Get("key") != config.FirebaseAPIKey {
		responder(w, 400, map[string]any{"error": map[string]string{"message": "API_KEY_INVALID"}})
		return
	}
	switch r.URL.Path {
	case "/signInWithIdp":
		var b struct {
			PostBody string `json:"postBody"`
		}
		_ = json.Unmarshal(cuerpo, &b)
		v, _ := url.ParseQuery(b.PostBody)
		if v.Get("id_token") != tokGoogle || v.Get("providerId") != "google.com" {
			responder(w, 400, map[string]any{"error": map[string]string{"message": "INVALID_IDP_RESPONSE"}})
			return
		}
		responder(w, 200, map[string]any{
			"idToken": f.nuevoID(), "refreshToken": tokRefresh, "expiresIn": "3600",
			"email": "victor@ejemplo.com", "localId": "u1",
		})
	case "/token":
		_ = r.ParseForm()
		f.renovaciones++
		if f.refreshInvalido || r.PostForm.Get("refresh_token") != tokRefresh {
			responder(w, 400, map[string]any{"error": map[string]string{"message": "INVALID_REFRESH_TOKEN"}})
			return
		}
		responder(w, 200, map[string]any{
			"id_token": f.nuevoID(), "refresh_token": tokRefresh, "expires_in": "3600", "user_id": "u1",
		})
	default:
		responder(w, 404, nil)
	}
}

func errorGateway(w http.ResponseWriter, status int, code, msg string) {
	responder(w, status, map[string]any{"error": map[string]string{"code": code, "message": msg, "request_id": "req-1"}})
}

func (f *falso) api(w http.ResponseWriter, r *http.Request, cuerpo []byte) {
	if r.Header.Get("Authorization") != "Bearer "+f.idVigente || f.rechazar401 > 0 {
		if f.rechazar401 > 0 {
			f.rechazar401--
		}
		errorGateway(w, 401, "UNAUTHENTICATED", "falta un token válido")
		return
	}
	if f.errorAPI != nil {
		errorGateway(w, f.errorAPI.status, f.errorAPI.code, f.errorAPI.msg)
		return
	}
	if f.actoresTokens(w, r, cuerpo) {
		return
	}
	partes := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// v1 projects {id} issues {n}[:move] [comments|events]
	switch {
	case r.URL.Path == "/v1/projects" && r.Method == http.MethodGet:
		responder(w, 200, map[string]any{"projects": f.proyectos, "page": map[string]any{"nextCursor": "", "hasNext": false}})
	case len(partes) == 4 && partes[3] == "statuses":
		responder(w, 200, map[string]any{"statuses": f.estados})
	case len(partes) == 4 && partes[3] == "labels":
		responder(w, 200, map[string]any{"labels": f.etiquetas})
	case len(partes) == 4 && partes[3] == "issues" && r.Method == http.MethodGet:
		f.listarIssues(w, r)
	case len(partes) == 4 && partes[3] == "issues" && r.Method == http.MethodPost:
		var b map[string]any
		_ = json.Unmarshal(cuerpo, &b)
		n := len(f.issues) + 1
		prio, _ := b["priority"].(string)
		if prio == "" {
			prio = "PRIORITY_MEDIUM"
		}
		i := issueFalso(n, b["title"].(string), "s1", "ISSUE_STATE_TODO", prio)
		if d, ok := b["description"].(string); ok {
			i["description"] = d
		}
		f.issues[n] = i
		responder(w, 200, map[string]any{"issue": i})
	case len(partes) >= 5 && partes[3] == "issues":
		numTxt, accion, _ := strings.Cut(partes[4], ":")
		n, _ := strconv.Atoi(numTxt)
		i, ok := f.issues[n]
		if !ok {
			errorGateway(w, 404, "NOT_FOUND", "issue no encontrado")
			return
		}
		switch {
		case accion == "move":
			var b map[string]string
			_ = json.Unmarshal(cuerpo, &b)
			for _, e := range f.estados {
				if e["id"] == b["statusId"] {
					i["statusId"], i["state"] = e["id"], e["category"]
				}
			}
			responder(w, 200, map[string]any{"issue": i})
		case len(partes) == 6 && partes[5] == "comments":
			var b map[string]string
			_ = json.Unmarshal(cuerpo, &b)
			responder(w, 200, map[string]any{"comment": map[string]any{
				"id": "c9", "issueId": i["id"], "authorId": "u1", "body": b["body"], "createdAt": "2026-11-09T12:00:00Z",
			}})
		case len(partes) == 6 && partes[5] == "events":
			responder(w, 200, map[string]any{"events": f.eventos, "page": map[string]any{"nextCursor": "", "hasNext": false}})
		default:
			responder(w, 200, map[string]any{"issue": i})
		}
	default:
		errorGateway(w, 404, "NOT_FOUND", "ruta inexistente")
	}
}

func (f *falso) listarIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	var out []map[string]any
	for n := 1; n <= len(f.issues); n++ {
		i := f.issues[n]
		if ids := q["statusIds"]; len(ids) > 0 && !contiene(ids, i["statusId"].(string)) {
			continue
		}
		if ps := q["priorities"]; len(ps) > 0 && !contiene(ps, i["priority"].(string)) {
			continue
		}
		if s := q.Get("query"); s != "" && !strings.Contains(strings.ToLower(i["title"].(string)), strings.ToLower(s)) {
			continue
		}
		out = append(out, i)
	}
	// Paginación: de a 2 para ejercitar el cursor.
	inicio, _ := strconv.Atoi(q.Get("page.cursor"))
	fin := min(inicio+2, len(out))
	pag := map[string]any{"nextCursor": "", "hasNext": false}
	if fin < len(out) {
		pag = map[string]any{"nextCursor": strconv.Itoa(fin), "hasNext": true}
	}
	if out == nil {
		out = []map[string]any{}
	}
	responder(w, 200, map[string]any{"issues": out[inicio:fin], "page": pag})
}

func contiene(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// --- Arnés -------------------------------------------------------------------

type resultado struct {
	codigo int
	out    string
	err    string
}

type arnes struct {
	t        *testing.T
	f        *falso
	dir      string
	llavero  auth.Llavero
	abiertas []string
	ahora    time.Time
}

func nuevoArnes(t *testing.T) *arnes {
	t.Helper()
	dir := t.TempDir()
	t.Setenv(config.EnvConfigDir, dir)
	t.Setenv(config.EnvAmbiente, "")
	t.Setenv(config.EnvAPIURL, "")
	t.Setenv(config.EnvClientID, "cliente-tv")
	t.Setenv(config.EnvClientSecret, clientSecret)
	return &arnes{t: t, f: nuevoFalso(t), dir: dir, ahora: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)}
}

func (a *arnes) correr(args ...string) resultado {
	a.t.Helper()
	var out, errb bytes.Buffer
	ent := Entorno{
		Out:  &out,
		Err:  &errb,
		HTTP: &http.Client{Transport: a.f},
		Auth: auth.Endpoints{
			DeviceCode:    "https://google.falso/device/code",
			Token:         "https://google.falso/token",
			SignInWithIdp: "https://firebase.falso/signInWithIdp",
			SecureToken:   "https://firebase.falso/token",
		},
		Llavero: a.llavero,
		Abrir: func(u string) error {
			a.abiertas = append(a.abiertas, u)
			return nil
		},
		Dormir: func(context.Context, time.Duration) error { return nil },
		Ahora:  func() time.Time { return a.ahora },
	}
	c := Ejecutar(context.Background(), args, ent)
	return resultado{codigo: c, out: out.String(), err: errb.String()}
}

// conSesion guarda una sesión vigente con el ID token que acepta el falso.
func (a *arnes) conSesion() {
	a.t.Helper()
	alm := &auth.Almacen{Dir: a.dir, Llavero: a.llavero}
	err := alm.Guardar(auth.Sesion{
		IDToken: a.f.idVigente, RefreshToken: tokRefresh, Expira: a.ahora.Add(time.Hour),
		Email: "victor@ejemplo.com", UID: "u1",
	})
	if err != nil {
		a.t.Fatal(err)
	}
}

func (a *arnes) pedidosAPI() []pedido {
	var ps []pedido
	for _, p := range a.f.pedidos {
		if p.Host != "google.falso" && p.Host != "firebase.falso" {
			ps = append(ps, p)
		}
	}
	return ps
}

// llaveroFalso es un llavero en memoria.
type llaveroFalso struct{ m map[string]string }

func (l *llaveroFalso) Get(s, c string) (string, error) {
	v, ok := l.m[s+"/"+c]
	if !ok {
		return "", fmt.Errorf("no está")
	}
	return v, nil
}
func (l *llaveroFalso) Set(s, c, v string) error { l.m[s+"/"+c] = v; return nil }
func (l *llaveroFalso) Delete(s, c string) error { delete(l.m, s+"/"+c); return nil }
