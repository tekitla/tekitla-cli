package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func decodificar[T any](t *testing.T, s string) T {
	t.Helper()
	var v T
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("JSON inválido: %v\n%s", err, s)
	}
	return v
}

// CU-029-1: login sin navegador imprime URL y código, guarda la sesión y
// después tk proyectos responde.
func TestLoginSinNavegador(t *testing.T) {
	a := nuevoArnes(t)
	a.f.pendientes = 2

	r := a.correr("login", "--no-browser")
	if r.codigo != 0 {
		t.Fatalf("login: código %d, err=%s", r.codigo, r.err)
	}
	if !strings.Contains(r.err, "https://www.google.com/device") || !strings.Contains(r.err, "ABCD-EFGH") {
		t.Errorf("faltan URL o código en stderr: %q", r.err)
	}
	if len(a.abiertas) != 0 {
		t.Errorf("--no-browser abrió el navegador: %v", a.abiertas)
	}
	if !strings.Contains(r.out, "victor@ejemplo.com") {
		t.Errorf("stdout = %q", r.out)
	}

	r = a.correr("proyectos")
	if r.codigo != 0 || !strings.Contains(r.out, "DP") || !strings.Contains(r.out, "Tekitla") {
		t.Fatalf("proyectos: código %d out=%q err=%q", r.codigo, r.out, r.err)
	}
}

func TestLoginAbreNavegador(t *testing.T) {
	a := nuevoArnes(t)
	if r := a.correr("login"); r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	if len(a.abiertas) != 1 || a.abiertas[0] != "https://www.google.com/device" {
		t.Errorf("abiertas = %v", a.abiertas)
	}
}

func TestLoginConLlaveroNoEscribeTokensEnDisco(t *testing.T) {
	a := nuevoArnes(t)
	l := &llaveroFalso{m: map[string]string{}}
	a.llavero = l
	r := a.correr("login", "--json", "--no-browser")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	if got := decodificar[map[string]string](t, r.out); got["almacen"] != "llavero" {
		t.Errorf("almacen = %q", got["almacen"])
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "sesion.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "SECRETO") {
		t.Errorf("sesion.json tiene tokens aunque hay llavero:\n%s", b)
	}
	if l.m["tekitla-tk/refresh_token"] != tokRefresh {
		t.Errorf("el refresh token no quedó en el llavero")
	}
	if r := a.correr("proyectos", "--json"); r.codigo != 0 {
		t.Fatalf("proyectos con llavero: %d %s", r.codigo, r.err)
	}
}

func TestLoginSinArchivoUsaPermisos0600(t *testing.T) {
	a := nuevoArnes(t)
	if r := a.correr("login", "--no-browser"); r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	st, err := os.Stat(filepath.Join(a.dir, "sesion.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm&0o077 != 0 && !esWindowsTest() {
		t.Errorf("permisos de sesion.json = %o, quiero 0600", perm)
	}
}

func TestLoginSinClienteOAuth(t *testing.T) {
	a := nuevoArnes(t)
	t.Setenv("TK_OAUTH_CLIENT_ID", "")
	t.Setenv("TK_OAUTH_CLIENT_SECRET", "")
	r := a.correr("login", "--no-browser")
	if r.codigo != 1 || !strings.Contains(r.err, "TK_OAUTH_CLIENT_ID") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
}

func TestLoginRechazado(t *testing.T) {
	a := nuevoArnes(t)
	t.Setenv("TK_OAUTH_CLIENT_SECRET", "otro")
	r := a.correr("login", "--no-browser")
	if r.codigo != 1 || !strings.Contains(r.err, "invalid_grant") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
}

func TestLogout(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	if r := a.correr("logout"); r.codigo != 0 {
		t.Fatalf("logout: %d %s", r.codigo, r.err)
	}
	if r := a.correr("proyectos"); r.codigo != 2 {
		t.Fatalf("después de logout: código %d", r.codigo)
	}
}

// CU-029-2: tk issues -p DP --json con clave, número, título, estado y
// prioridad (todas las páginas).
func TestIssuesJSON(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issues", "-p", "dp", "--json")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	got := decodificar[[]map[string]any](t, r.out)
	if len(got) != 3 {
		t.Fatalf("quiero 3 issues (2 páginas), hay %d", len(got))
	}
	i := got[2]
	for k, v := range map[string]any{"clave": "DP-3", "numero": float64(3), "titulo": "Conchas",
		"estado": "Por hacer", "categoria": "por_hacer", "prioridad": "alta"} {
		if i[k] != v {
			t.Errorf("%s = %v, quiero %v", k, i[k], v)
		}
	}
}

func TestIssuesFiltros(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issues", "-p", "DP", "--estado", "por HACER", "--prioridad", "alta,urgente", "--buscar", "con")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	if !strings.Contains(r.out, "DP-3") || strings.Contains(r.out, "DP-1") {
		t.Errorf("salida: %q", r.out)
	}
	var lista *pedido
	for _, p := range a.pedidosAPI() {
		if strings.HasSuffix(p.Ruta, "/issues") {
			lista = &p
		}
	}
	if lista == nil {
		t.Fatal("no se llamó a ListIssues")
	}
	if lista.Ruta != "/v1/projects/p-dp/issues" ||
		strings.Join(lista.Query["statusIds"], ",") != "s1" ||
		strings.Join(lista.Query["priorities"], ",") != "PRIORITY_HIGH,PRIORITY_URGENT" ||
		lista.Query.Get("query") != "con" {
		t.Errorf("ListIssues: %s %v", lista.Ruta, lista.Query)
	}
}

func TestIssuesPrioridadInvalida(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issues", "-p", "DP", "--prioridad", "altísima")
	if r.codigo != 1 || !strings.Contains(r.err, "baja, media, alta o urgente") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
}

func TestProyectoInexistente(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issues", "-p", "ZZ")
	if r.codigo != 1 || !strings.Contains(r.err, "DP, TK") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
}

// CU-029-3: tk mover DP-3 "En curso" cambia el estado y tk issue lo muestra.
func TestMover(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("mover", "DP-3", "en curso")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	var mov *pedido
	for _, p := range a.pedidosAPI() {
		if strings.HasSuffix(p.Ruta, ":move") {
			mov = &p
		}
	}
	if mov == nil || mov.Metodo != "POST" || mov.Ruta != "/v1/projects/p-dp/issues/3:move" || mov.Cuerpo != `{"statusId":"s2"}` {
		t.Fatalf("MoveIssue: %+v", mov)
	}
	r = a.correr("issue", "DP-3", "--json")
	if r.codigo != 0 {
		t.Fatalf("issue: %d %s", r.codigo, r.err)
	}
	if d := decodificar[map[string]any](t, r.out); d["estado"] != "En curso" || d["categoria"] != "en_curso" {
		t.Errorf("estado = %v / %v", d["estado"], d["categoria"])
	}
}

// CU-029-4: un estado que no existe sale con 1 y lista los válidos.
func TestMoverEstadoInexistente(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("mover", "DP-3", "En revisión")
	if r.codigo != 1 {
		t.Fatalf("código %d", r.codigo)
	}
	for _, e := range []string{"Por hacer", "En curso", "Hecho", "Descartado"} {
		if !strings.Contains(r.err, e) {
			t.Errorf("falta %q en %q", e, r.err)
		}
	}
	r = a.correr("mover", "DP-3", "En revisión", "--json")
	if r.codigo != 1 {
		t.Fatalf("--json: código %d", r.codigo)
	}
	got := decodificar[map[string]map[string]any](t, r.err)
	if got["error"]["code"] != "ESTADO_DESCONOCIDO" || len(got["error"]["estados_validos"].([]any)) != 4 {
		t.Errorf("error JSON = %v", got)
	}
	if r.out != "" {
		t.Errorf("stdout debería quedar vacío: %q", r.out)
	}
}

func TestClaveInvalida(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	for _, c := range []string{"DP", "DP-0", "D-3", "3-DP", "DP-x"} {
		if r := a.correr("issue", c); r.codigo != 1 {
			t.Errorf("%s: código %d", c, r.codigo)
		}
	}
}

// CU-029-5: sin sesión, todo comando que usa la API sale con 2 y pide login.
func TestSinSesion(t *testing.T) {
	a := nuevoArnes(t)
	comandos := [][]string{
		{"proyectos"},
		{"issues", "-p", "DP"},
		{"issue", "DP-1"},
		{"issue", "nuevo", "-p", "DP", "x"},
		{"mover", "DP-1", "Hecho"},
		{"comentar", "DP-1", "hola"},
	}
	for _, c := range comandos {
		for _, js := range []bool{false, true} {
			args := c
			if js {
				args = append(append([]string{}, c...), "--json")
			}
			r := a.correr(args...)
			if r.codigo != 2 || !strings.Contains(r.err, "tk login") {
				t.Errorf("%v: código %d err=%q", args, r.codigo, r.err)
			}
		}
	}
	if n := len(a.pedidosAPI()); n != 0 {
		t.Errorf("sin sesión no debería llamar a la API (%d llamadas)", n)
	}
}

// CU-029-6: --ambiente qa (o TK_AMBIENTE) llama a api-qa y no a prod.
func TestAmbienteQA(t *testing.T) {
	casos := []struct {
		nombre string
		env    string
		args   []string
		host   string
	}{
		{"flag qa", "", []string{"--ambiente", "qa", "issues", "-p", "DP"}, "api-qa.tekitla.victorrb1015.com"},
		{"env qa", "qa", []string{"issues", "-p", "DP"}, "api-qa.tekitla.victorrb1015.com"},
		{"flag pisa env", "qa", []string{"--ambiente", "prod", "proyectos"}, "api.tekitla.victorrb1015.com"},
		{"default prod", "", []string{"proyectos"}, "api.tekitla.victorrb1015.com"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			a := nuevoArnes(t)
			t.Setenv("TK_AMBIENTE", c.env)
			a.conSesion()
			if r := a.correr(c.args...); r.codigo != 0 {
				t.Fatalf("código %d: %s", r.codigo, r.err)
			}
			for _, p := range a.pedidosAPI() {
				if p.Host != c.host {
					t.Errorf("llamó a %s, quiero %s", p.Host, c.host)
				}
			}
			if len(a.pedidosAPI()) == 0 {
				t.Error("no hubo llamadas a la API")
			}
		})
	}
}

func TestAmbienteDesconocido(t *testing.T) {
	a := nuevoArnes(t)
	r := a.correr("--ambiente", "staging", "version")
	if r.codigo != 1 || !strings.Contains(r.err, "qa o prod") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
}

func TestAmbienteDesdeConfigTOML(t *testing.T) {
	a := nuevoArnes(t)
	if err := os.WriteFile(filepath.Join(a.dir, "config.toml"), []byte("ambiente = \"qa\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := a.correr("version", "--json")
	if got := decodificar[map[string]string](t, r.out); got["ambiente"] != "qa" ||
		got["api"] != "https://api-qa.tekitla.victorrb1015.com" {
		t.Errorf("version = %v", got)
	}
}

// CU-029-7: ni el ID token ni el refresh token aparecen en la salida.
func TestNoSeFiltranTokens(t *testing.T) {
	a := nuevoArnes(t)
	a.f.pendientes = 1
	todas := []resultado{a.correr("login", "--no-browser", "--json")}
	comandos := [][]string{
		{"proyectos"},
		{"issues", "-p", "DP"},
		{"issue", "DP-3"},
		{"issue", "nuevo", "-p", "DP", "Nuevo"},
		{"mover", "DP-1", "Hecho"},
		{"mover", "DP-1", "Inexistente"},
		{"comentar", "DP-1", "hola"},
		{"issue", "DP-99"},
		{"agentes"},
		{"tokens"},
		{"token", "revocar", "t-nada"},
		{"version"},
	}
	for _, c := range comandos {
		todas = append(todas, a.correr(append(c, "--json")...), a.correr(c...))
	}
	// Con el ID token vencido (renovación) y con el refresh revocado.
	a.ahora = a.ahora.Add(2 * time.Hour)
	todas = append(todas, a.correr("proyectos", "--json"))
	a.f.refreshInvalido = true
	a.ahora = a.ahora.Add(2 * time.Hour)
	todas = append(todas, a.correr("proyectos", "--json"))

	for _, r := range todas {
		if strings.Contains(r.out+r.err, "SECRETO") || strings.Contains(r.out+r.err, "AIza") {
			t.Errorf("se filtró un token:\nout=%s\nerr=%s", r.out, r.err)
		}
	}
}

func TestRenuevaIDTokenVencido(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	a.ahora = a.ahora.Add(58 * time.Minute) // dentro del margen de 5 min
	if r := a.correr("proyectos"); r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	if a.f.renovaciones != 1 {
		t.Fatalf("renovaciones = %d", a.f.renovaciones)
	}
	// El token renovado quedó guardado: el comando siguiente no renueva.
	if r := a.correr("proyectos"); r.codigo != 0 || a.f.renovaciones != 1 {
		t.Fatalf("código %d, renovaciones = %d", r.codigo, a.f.renovaciones)
	}
}

func TestReintentaUnaVezTras401(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	a.f.rechazar401 = 1
	if r := a.correr("proyectos"); r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	if a.f.renovaciones != 1 {
		t.Errorf("renovaciones = %d", a.f.renovaciones)
	}
	a.f.rechazar401 = 2
	if r := a.correr("proyectos"); r.codigo != 2 {
		t.Errorf("dos 401 seguidos: código %d", r.codigo)
	}
}

func TestRefreshRevocadoBorraSesion(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	a.f.refreshInvalido = true
	a.ahora = a.ahora.Add(2 * time.Hour)
	r := a.correr("proyectos")
	if r.codigo != 2 || !strings.Contains(r.err, "tk login") {
		t.Fatalf("código %d err=%q", r.codigo, r.err)
	}
	if _, err := os.Stat(filepath.Join(a.dir, "sesion.json")); !os.IsNotExist(err) {
		t.Errorf("la sesión revocada debería borrarse (err=%v)", err)
	}
}

func TestErrorDeLaAPI(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issue", "DP-99", "--json")
	if r.codigo != 1 {
		t.Fatalf("código %d", r.codigo)
	}
	got := decodificar[map[string]map[string]any](t, r.err)
	if got["error"]["code"] != "NOT_FOUND" || got["error"]["status"] != float64(404) || got["error"]["request_id"] != "req-1" {
		t.Errorf("error = %v", got)
	}
	a.f.errorAPI = &errorFalso{403, "PERMISSION_DENIED", "no tenés permiso para esta operación"}
	r = a.correr("proyectos")
	if r.codigo != 1 || !strings.Contains(r.err, "PERMISSION_DENIED") {
		t.Errorf("403: código %d err=%q", r.codigo, r.err)
	}
}

func TestIssueDetalle(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issue", "DP-3", "--json")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	d := decodificar[issueDetalleJSON](t, r.out)
	if d.Clave != "DP-3" || d.Descripcion != "Con azúcar" || len(d.Etiquetas) != 1 || d.Etiquetas[0] != "backend" {
		t.Errorf("detalle = %+v", d)
	}
	if len(d.Eventos) != 2 || d.Eventos[0].Tipo != "creado" || d.Eventos[1].Tipo != "comentario" ||
		d.Eventos[1].Texto != "Primera línea\nsegunda" {
		t.Errorf("eventos = %+v", d.Eventos)
	}
	r = a.correr("issue", "DP-3")
	for _, s := range []string{"DP-3  Conchas", "Por hacer", "alta", "backend", "Con azúcar", "comentó: Primera línea …"} {
		if !strings.Contains(r.out, s) {
			t.Errorf("falta %q en:\n%s", s, r.out)
		}
	}
}

func TestIssueNuevo(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("issue", "nuevo", "-p", "DP", "Galletas", "--prioridad", "urgente", "--descripcion", "De nata", "--json")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	got := decodificar[map[string]any](t, r.out)
	if got["clave"] != "DP-4" || got["prioridad"] != "urgente" || got["estado"] != "Por hacer" {
		t.Errorf("nuevo = %v", got)
	}
	var crear *pedido
	for _, p := range a.pedidosAPI() {
		if p.Metodo == "POST" && strings.HasSuffix(p.Ruta, "/issues") {
			crear = &p
		}
	}
	if crear == nil || crear.Cuerpo != `{"title":"Galletas","description":"De nata","priority":"PRIORITY_URGENT"}` {
		t.Errorf("CreateIssue: %+v", crear)
	}
	if r := a.correr("issue", "nuevo", "-p", "DP", "  "); r.codigo != 1 {
		t.Errorf("título vacío: código %d", r.codigo)
	}
	if r := a.correr("issue", "nuevo", "Sin proyecto"); r.codigo != 1 {
		t.Errorf("sin -p: código %d", r.codigo)
	}
}

func TestComentar(t *testing.T) {
	a := nuevoArnes(t)
	a.conSesion()
	r := a.correr("comentar", "DP-2", "Listo", "--json")
	if r.codigo != 0 {
		t.Fatalf("código %d: %s", r.codigo, r.err)
	}
	got := decodificar[map[string]any](t, r.out)
	if got["clave"] != "DP-2" || got["comentario_id"] != "c9" {
		t.Errorf("comentar = %v", got)
	}
	ps := a.pedidosAPI()
	if last := ps[len(ps)-1]; last.Ruta != "/v1/projects/p-dp/issues/2/comments" || last.Cuerpo != `{"body":"Listo"}` {
		t.Errorf("CreateComment: %+v", last)
	}
}

func TestVersionNoTocaLaRed(t *testing.T) {
	a := nuevoArnes(t)
	r := a.correr("version")
	if r.codigo != 0 || !strings.Contains(r.out, "ambiente: prod") {
		t.Fatalf("código %d out=%q", r.codigo, r.out)
	}
	if len(a.f.pedidos) != 0 {
		t.Errorf("version hizo %d pedidos", len(a.f.pedidos))
	}
}

func TestErrorDeUso(t *testing.T) {
	a := nuevoArnes(t)
	for _, args := range [][]string{{"mover", "DP-1"}, {"--bandera-rara"}, {"comandoraro"}} {
		if r := a.correr(args...); r.codigo != 1 {
			t.Errorf("%v: código %d", args, r.codigo)
		}
	}
}

func esWindowsTest() bool { return runtime.GOOS == "windows" }
