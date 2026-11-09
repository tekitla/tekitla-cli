package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/tekitla/tekitla-cli/internal/api"
)

// --- JSON estable -----------------------------------------------------------
//
// La salida --json de tk es un contrato con los agentes: nombres en español,
// snake_case, y solo se agregan campos (nunca se renombran ni se quitan).

type proyectoJSON struct {
	Clave       string `json:"clave"`
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion"`
	ID          string `json:"id"`
}

type issueJSON struct {
	Clave     string `json:"clave"`
	Numero    int    `json:"numero"`
	Titulo    string `json:"titulo"`
	Estado    string `json:"estado"`
	Categoria string `json:"categoria"`
	Prioridad string `json:"prioridad"`
}

type issueDetalleJSON struct {
	issueJSON
	Etiquetas   []string     `json:"etiquetas"`
	Descripcion string       `json:"descripcion"`
	Creado      string       `json:"creado"`
	Actualizado string       `json:"actualizado"`
	Eventos     []eventoJSON `json:"eventos"`
}

type eventoJSON struct {
	Fecha   string `json:"fecha"`
	Tipo    string `json:"tipo"`
	Resumen string `json:"resumen"`
	Texto   string `json:"texto,omitempty"`
	ActorID string `json:"actor_id"`
	AgentID string `json:"agente_id,omitempty"`
}

func escribirJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func tabla(w io.Writer, encabezado []string, filas [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(encabezado, "\t"))
	for _, f := range filas {
		fmt.Fprintln(tw, strings.Join(f, "\t"))
	}
	_ = tw.Flush()
}

// --- Enums ------------------------------------------------------------------

var categorias = map[string]string{
	"ISSUE_STATE_TODO":        "por_hacer",
	"ISSUE_STATE_IN_PROGRESS": "en_curso",
	"ISSUE_STATE_DONE":        "hecho",
	"ISSUE_STATE_CANCELED":    "descartado",
}

func categoria(s string) string {
	if c, ok := categorias[s]; ok {
		return c
	}
	return strings.ToLower(strings.TrimPrefix(s, "ISSUE_STATE_"))
}

var prioridades = map[string]string{
	"PRIORITY_LOW":    "baja",
	"PRIORITY_MEDIUM": "media",
	"PRIORITY_HIGH":   "alta",
	"PRIORITY_URGENT": "urgente",
}

func prioridad(p string) string {
	if v, ok := prioridades[p]; ok {
		return v
	}
	return strings.ToLower(strings.TrimPrefix(p, "PRIORITY_"))
}

var prioridadesEntrada = map[string]string{
	"baja": "PRIORITY_LOW", "low": "PRIORITY_LOW",
	"media": "PRIORITY_MEDIUM", "medium": "PRIORITY_MEDIUM",
	"alta": "PRIORITY_HIGH", "high": "PRIORITY_HIGH",
	"urgente": "PRIORITY_URGENT", "urgent": "PRIORITY_URGENT",
}

func parsearPrioridad(s string) (string, error) {
	if p, ok := prioridadesEntrada[normalizar(s)]; ok {
		return p, nil
	}
	return "", usoErr("prioridad %q desconocida: usa baja, media, alta o urgente", s)
}

// --- Claves -----------------------------------------------------------------

// parsearClave separa "DP-3" en ("DP", 3).
func parsearClave(s string) (string, int, error) {
	k, n, ok := strings.Cut(strings.TrimSpace(s), "-")
	num, err := strconv.Atoi(n)
	if !ok || err != nil || num < 1 || !claveValida(k) {
		return "", 0, usoErr("%q no es una clave de issue: usa CLAVE-N, p. ej. TK-42", s)
	}
	return strings.ToUpper(k), num, nil
}

// claveValida: 2 a 10 letras o dígitos, empieza con letra (Project.key).
func claveValida(k string) bool {
	if len(k) < 2 || len(k) > 10 {
		return false
	}
	for i, r := range k {
		esLetra := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		esDigito := r >= '0' && r <= '9'
		if !esLetra && (i == 0 || !esDigito) {
			return false
		}
	}
	return true
}

func claveIssue(p api.Proyecto, n int) string { return p.Key + "-" + strconv.Itoa(n) }

// normalizar compara nombres sin mayúsculas, acentos ni espacios de más:
// «en curso» == «En Curso» == «En  cúrso».
func normalizar(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(s)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func primeraLinea(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if r := []rune(s); len(r) > max {
		s = string(r[:max-1]) + "…"
	}
	return s
}

// abrirNavegador lanza el navegador sin esperarlo. El contexto no se cancela
// con el de tk: el proceso sigue aunque tk termine.
func abrirNavegador(url string) error {
	ctx := context.Background()
	var c *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		c = exec.CommandContext(ctx, "rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		c = exec.CommandContext(ctx, "open", url)
	default:
		c = exec.CommandContext(ctx, "xdg-open", url)
	}
	return c.Start()
}
