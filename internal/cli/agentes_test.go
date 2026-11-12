package cli

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentes(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("agentes", "--json")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	ags := decodificar[[]agenteJSON](t, r.out)
	if len(ags) != 1 || ags[0].Nombre != "Codex PC-01" || ags[0].Herramienta != "codex" || ags[0].ID != "a-codex" {
		t.Fatalf("agentes = %+v", ags)
	}
	if r := a.correr("agentes", "--revocados", "--json"); len(decodificar[[]agenteJSON](t, r.out)) != 2 {
		t.Fatalf("--revocados: %s", r.out)
	}
	if r := a.correr("agentes"); r.codigo != 0 || !strings.Contains(r.out, "Codex PC-01") || !strings.Contains(r.out, "HERRAMIENTA") {
		t.Fatalf("tabla: %q", r.out)
	}
}

// CU-030-7: agente nuevo + token nuevo; el token sale una vez, tk tokens lo
// lista sin el valor y no queda en ningún archivo.
func TestAgenteYTokenNuevo(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()

	r := a.correr("agente", "nuevo", "Claude Code PC-01", "--herramienta", "claude-code", "--json")
	if r.codigo != 0 {
		t.Fatalf("agente nuevo: código %d: %s", r.codigo, r.err)
	}
	ag := decodificar[agenteJSON](t, r.out)
	if ag.Nombre != "Claude Code PC-01" || ag.Herramienta != "claude-code" {
		t.Fatalf("agente = %+v", ag)
	}
	ps := a.pedidosAPI()
	if p := ps[len(ps)-1]; p.Metodo != "POST" || p.Ruta != "/v1/agents" ||
		!strings.Contains(p.Cuerpo, `"tool":"AGENT_TOOL_CLAUDE_CODE"`) || !strings.Contains(p.Cuerpo, `"displayName":"Claude Code PC-01"`) {
		t.Fatalf("pedido = %+v", p)
	}

	// Texto: el token solo por stdout, el aviso por stderr.
	r = a.correr("token", "nuevo", "--agente", "claude code pc-01", "--permiso", "issues:read")
	if r.codigo != 0 {
		t.Fatalf("token nuevo: código %d: %s", r.codigo, r.err)
	}
	if strings.TrimSpace(r.out) != secretoToken {
		t.Fatalf("stdout = %q, se esperaba solo el token", r.out)
	}
	if !strings.Contains(r.err, "no se vuelve a mostrar") || strings.Contains(r.err, secretoToken) {
		t.Fatalf("stderr = %q", r.err)
	}
	ps = a.pedidosAPI()
	if p := ps[len(ps)-1]; p.Ruta != "/v1/tokens" || !strings.Contains(p.Cuerpo, `"actorId":"a-4"`) ||
		!strings.Contains(p.Cuerpo, `"scopes":["TOKEN_SCOPE_ISSUES_READ"]`) || strings.Contains(p.Cuerpo, "expiresAt") {
		t.Fatalf("pedido = %+v", p)
	}

	// JSON: secreto, aviso y datos del token.
	r = a.correr("token", "nuevo", "--agente", "Claude Code PC-01", "--permiso", "issues:read", "--permiso", "issues:write", "--vence", "7d", "--json")
	if r.codigo != 0 {
		t.Fatalf("token nuevo --json: código %d: %s", r.codigo, r.err)
	}
	nt := decodificar[tokenNuevoJSON](t, r.out)
	if nt.Secreto != secretoToken || nt.Aviso == "" || nt.Token.Agente != "Claude Code PC-01" ||
		strings.Join(nt.Token.Permisos, ",") != "issues:read,issues:write" || nt.Token.Vence != "2026-11-16T12:00:00Z" {
		t.Fatalf("token = %+v", nt)
	}
	if strings.Count(r.out, secretoToken) != 1 {
		t.Fatalf("el token sale más de una vez:\n%s", r.out)
	}
	// --vence 90d se manda con un margen por el reloj del servicio.
	r = a.correr("token", "nuevo", "--agente", "Claude Code PC-01", "--permiso", "issues:read", "--vence", "90d", "--json")
	if nt := decodificar[tokenNuevoJSON](t, r.out); nt.Token.Vence != "2027-02-07T11:50:00Z" {
		t.Fatalf("--vence 90d: vence %s, se esperaba 90d menos el margen", nt.Token.Vence)
	}

	// tk tokens: lista sin el valor.
	for _, args := range [][]string{{"tokens", "--json"}, {"tokens"}, {"tokens", "--todos", "--json"}} {
		r = a.correr(args...)
		if r.codigo != 0 || strings.Contains(r.out+r.err, secretoToken) || strings.Contains(r.out, "tka_") {
			t.Fatalf("%v: código %d, out=%s err=%s", args, r.codigo, r.out, r.err)
		}
	}
	toks := decodificar[[]tokenJSON](t, a.correr("tokens", "--json").out)
	if len(toks) != 4 || toks[0].Agente != "Claude Code PC-01" || toks[0].Estado != "activo" || toks[3].Agente != "Codex PC-01" || toks[3].UltimoUso == "" {
		t.Fatalf("tokens = %+v", toks)
	}

	// Ningún archivo del directorio de tk tiene el token.
	_ = filepath.WalkDir(a.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, _ := os.ReadFile(path)
		if strings.Contains(string(b), secretoToken) {
			t.Errorf("%s tiene el token en claro", path)
		}
		return nil
	})
}

func TestTokenNuevoErrores(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	for nombre, c := range map[string]struct {
		args []string
		want string
	}{
		"sin permiso":        {[]string{"token", "nuevo", "--agente", "Codex PC-01"}, "--permiso"},
		"permiso inválido":   {[]string{"token", "nuevo", "--agente", "Codex PC-01", "--permiso", "admin"}, "issues:read"},
		"vence de más":       {[]string{"token", "nuevo", "--agente", "Codex PC-01", "--permiso", "issues:read", "--vence", "91d"}, "90d"},
		"vence mal escrito":  {[]string{"token", "nuevo", "--agente", "Codex PC-01", "--permiso", "issues:read", "--vence", "1 mes"}, "30d"},
		"agente inexistente": {[]string{"token", "nuevo", "--agente", "Nadie", "--permiso", "issues:read"}, "Codex PC-01"},
		"agente revocado":    {[]string{"token", "nuevo", "--agente", "Viejo", "--permiso", "issues:read"}, "no hay un agente"},
		"sin --agente":       {[]string{"token", "nuevo", "--permiso", "issues:read"}, "agente"},
		"herramienta mala":   {[]string{"agente", "nuevo", "X", "--herramienta", "vim"}, "claude-code"},
		"revocar con valor":  {[]string{"token", "revocar", secretoToken}, "id del token"},
		"sin subcomando":     {[]string{"token"}, "subcomando"},
		"vence desborda":     {[]string{"token", "nuevo", "--agente", "Codex PC-01", "--permiso", "issues:read", "--vence", "106752d"}, "90d"},
		"vence cero":         {[]string{"token", "nuevo", "--agente", "Codex PC-01", "--permiso", "issues:read", "--vence", "0h"}, "1h"},
		"token como agente":  {[]string{"token", "nuevo", "--agente", secretoToken, "--permiso", "issues:read"}, "parece el valor"},
		"revocar agente tka": {[]string{"agente", "revocar", secretoToken}, "parece el valor"},
		"agente nuevo tka":   {[]string{"agente", "nuevo", secretoToken, "--herramienta", "codex"}, "parece el valor"},
	} {
		r := a.correr(c.args...)
		if r.codigo != 1 || !strings.Contains(r.err, c.want) {
			t.Errorf("%s: código %d err=%q", nombre, r.codigo, r.err)
		}
		if strings.Contains(r.out+r.err, secretoToken) {
			t.Errorf("%s: el token apareció en la salida", nombre)
		}
	}
	for _, p := range a.pedidosAPI() {
		if p.Metodo == "POST" {
			t.Fatalf("un error de uso llegó a escribir: %+v", p)
		}
	}
}

func TestAgenteYaExiste(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("agente", "nuevo", "codex pc-01", "--herramienta", "codex", "--json")
	if r.codigo != 1 {
		t.Fatalf("código %d", r.codigo)
	}
	var e struct{ Error map[string]any }
	if err := json.Unmarshal([]byte(r.err), &e); err != nil || e.Error["code"] != "ALREADY_EXISTS" {
		t.Fatalf("stderr = %s", r.err)
	}
}

func TestRevocar(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()

	r := a.correr("token", "revocar", "t-codex", "--json")
	if r.codigo != 0 {
		t.Fatalf("token revocar: %d %s", r.codigo, r.err)
	}
	if tj := decodificar[tokenJSON](t, r.out); tj.Estado != "revocado" || tj.Revocado == "" || tj.Agente != "Codex PC-01" {
		t.Fatalf("token = %+v", tj)
	}
	if r := a.correr("token", "revocar", "t-nada"); r.codigo != 1 || !strings.Contains(r.err, "NOT_FOUND") {
		t.Fatalf("token inexistente: %d %s", r.codigo, r.err)
	}

	r = a.correr("agente", "revocar", "Codex PC-01")
	if r.codigo != 0 || !strings.Contains(r.out, "revocado") {
		t.Fatalf("agente revocar: %d out=%s err=%s", r.codigo, r.out, r.err)
	}
	ps := a.pedidosAPI()
	if p := ps[len(ps)-1]; p.Metodo != "POST" || p.Ruta != "/v1/actors/a-codex:revoke" {
		t.Fatalf("pedido = %+v", p)
	}
	if ags := decodificar[[]agenteJSON](t, a.correr("agentes", "--json").out); len(ags) != 0 {
		t.Fatalf("quedaron agentes: %+v", ags)
	}
	if r := a.correr("agente", "revocar", "Codex PC-01"); r.codigo != 1 {
		t.Fatalf("revocar dos veces: código %d", r.codigo)
	}
}

// Sin sesión, los comandos nuevos piden tk login (código 2) sin tocar la API.
func TestAgentesSinSesion(t *testing.T) {
	a := nuevoArnes(t)
	for _, args := range [][]string{{"agentes"}, {"tokens"}, {"agente", "nuevo", "X", "--herramienta", "codex"},
		{"token", "nuevo", "--agente", "X", "--permiso", "issues:read"}, {"token", "revocar", "t-1"}, {"agente", "revocar", "X"}} {
		if r := a.correr(args...); r.codigo != 2 {
			t.Errorf("%v: código %d, err=%s", args, r.codigo, r.err)
		}
	}
	if n := len(a.pedidosAPI()); n != 0 {
		t.Fatalf("sin sesión llegaron %d pedidos a la API", n)
	}
}

// CU-037-1: la sugerencia de tk agente nuevo repite --ambiente qa; copiada
// tal cual, el token se crea en el mismo ambiente.
func TestAgenteNuevoSugiereElAmbiente(t *testing.T) {
	casos := []struct {
		nombre string
		env    string
		args   []string
		quiero string
		host   string
	}{
		{"flag qa", "", []string{"--ambiente", "qa"},
			`tk token nuevo --agente "X" --permiso issues:read --ambiente qa`, "api-qa.tekitla.victorrb1015.com"},
		{"env qa", "qa", nil,
			`tk token nuevo --agente "X" --permiso issues:read --ambiente qa`, "api-qa.tekitla.victorrb1015.com"},
		// Con TK_AMBIENTE=qa en esta terminal, --ambiente prod se repite: en
		// otra terminal sin la variable daría igual, aquí no.
		{"flag prod pisa env", "qa", []string{"--ambiente", "prod"},
			`tk token nuevo --agente "X" --permiso issues:read --ambiente prod`, "api.tekitla.victorrb1015.com"},
		{"default prod", "", nil,
			`tk token nuevo --agente "X" --permiso issues:read` + "\n", "api.tekitla.victorrb1015.com"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a := nuevoArnes(t)
			t.Setenv("TK_AMBIENTE", c.env)
			a.conSesion()
			r := a.correr(append([]string{"agente", "nuevo", "X", "--herramienta", "otra"}, c.args...)...)
			if r.codigo != 0 {
				t.Fatalf("código %d: %s", r.codigo, r.err)
			}
			if !strings.Contains(r.out, c.quiero) {
				t.Fatalf("salida = %q, quiero la sugerencia %q", r.out, c.quiero)
			}
			for _, p := range a.pedidosAPI() {
				if p.Host != c.host {
					t.Errorf("llamó a %s, quiero %s", p.Host, c.host)
				}
			}
		})
	}
}

// CU-037-2: las respuestas que escriben dicen en qué ambiente fue, en texto
// («[qa] …») y con --json (campo ambiente).
func TestRespuestasDicenElAmbiente(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	qa := []string{"--ambiente", "qa"}
	correr := func(args ...string) resultado {
		t.Helper()
		r := a.correr(append(args, qa...)...)
		if r.codigo != 0 {
			t.Fatalf("%v: código %d: %s", args, r.codigo, r.err)
		}
		return r
	}

	if r := correr("agente", "nuevo", "X", "--herramienta", "otra"); !strings.HasPrefix(r.out, `[qa] Agente "X" creado`) {
		t.Errorf("agente nuevo: %q", r.out)
	}
	if g := decodificar[agenteJSON](t, correr("agente", "nuevo", "Y", "--herramienta", "otra", "--json").out); g.Ambiente != "qa" {
		t.Errorf("agente nuevo --json: %+v", g)
	}

	r := correr("token", "nuevo", "--agente", "X", "--permiso", "issues:read")
	if !strings.HasPrefix(r.err, "[qa] Token ") || strings.TrimSpace(r.out) != secretoToken {
		t.Errorf("token nuevo: out=%q err=%q", r.out, r.err)
	}
	r = correr("token", "nuevo", "--agente", "X", "--permiso", "issues:read", "--json")
	if nt := decodificar[tokenNuevoJSON](t, r.out); nt.Ambiente != "qa" || nt.Secreto != secretoToken {
		t.Errorf("token nuevo --json: %+v", nt)
	}
	if !strings.Contains(r.out, `"ambiente": "qa"`) {
		t.Errorf("token nuevo --json sin \"ambiente\": \"qa\": %s", r.out)
	}

	toks := decodificar[[]tokenJSON](t, correr("tokens", "--json").out)
	if len(toks) < 2 || toks[0].Ambiente != "" {
		t.Fatalf("tk tokens no lleva ambiente por token: %+v", toks)
	}
	if r := correr("token", "revocar", toks[0].ID); !strings.HasPrefix(r.out, "[qa] Token "+toks[0].ID+" revocado") {
		t.Errorf("token revocar: %q", r.out)
	}
	if j := decodificar[tokenJSON](t, correr("token", "revocar", toks[1].ID, "--json").out); j.Ambiente != "qa" {
		t.Errorf("token revocar --json: %+v", j)
	}

	if r := correr("agente", "revocar", "Y"); !strings.HasPrefix(r.out, `[qa] Agente "Y" revocado`) {
		t.Errorf("agente revocar: %q", r.out)
	}
	if g := decodificar[agenteJSON](t, correr("agente", "revocar", "X", "--json").out); g.Ambiente != "qa" {
		t.Errorf("agente revocar --json: %+v", g)
	}

	for _, p := range a.pedidosAPI() {
		if p.Host != "api-qa.tekitla.victorrb1015.com" {
			t.Errorf("llamó a %s", p.Host)
		}
	}
}

// Con TK_API_URL la etiqueta dice la URL a la que fue, no el ambiente.
func TestEtiquetaConURLPisada(t *testing.T) {
	a := nuevoArnes(t)
	t.Setenv("TK_API_URL", "https://api-qa.tekitla.victorrb1015.com")
	a.conSesion()
	r := a.correr("agente", "nuevo", "X", "--herramienta", "otra")
	if r.codigo != 0 || !strings.HasPrefix(r.out, `[https://api-qa.tekitla.victorrb1015.com] Agente "X" creado`) {
		t.Fatalf("código %d out=%q err=%q", r.codigo, r.out, r.err)
	}
}
