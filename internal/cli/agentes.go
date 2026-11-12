package cli

// Agentes y tokens tka_ (issue 030): tk agentes, tk agente nuevo/revocar,
// tk tokens y tk token nuevo/revocar. Todos con la sesión de Firebase de
// tk login: el gateway no deja crear agentes ni tokens con un token.
//
// El token en claro sale solo en la respuesta de CreateToken: tk lo imprime
// una vez por stdout y nunca lo escribe en un archivo.

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tekitla/tekitla-cli/internal/api"
)

const (
	venceMaximo = 90 * 24 * time.Hour
	// margenReloj: el servicio valida "a 90 días como máximo" con su reloj;
	// --vence 90d se manda un poco antes para que un reloj local adelantado
	// no lo haga fallar.
	margenReloj = 10 * time.Minute
)

var herramientas = map[string]string{
	"claude-code": "AGENT_TOOL_CLAUDE_CODE",
	"codex":       "AGENT_TOOL_CODEX",
	"cloud":       "AGENT_TOOL_CLOUD",
	"otra":        "AGENT_TOOL_OTHER",
}

func herramienta(t string) string {
	for k, v := range herramientas {
		if v == t {
			return k
		}
	}
	return strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(t, "AGENT_TOOL_")), "_", "-")
}

var permisos = map[string]string{
	"issues:read":  "TOKEN_SCOPE_ISSUES_READ",
	"issues:write": "TOKEN_SCOPE_ISSUES_WRITE",
}

func permiso(s string) string {
	for k, v := range permisos {
		if v == s {
			return k
		}
	}
	return strings.ToLower(strings.TrimPrefix(s, "TOKEN_SCOPE_"))
}

// --- JSON estable ------------------------------------------------------------

type agenteJSON struct {
	ID          string `json:"id"`
	Nombre      string `json:"nombre"`
	Herramienta string `json:"herramienta"`
	Creado      string `json:"creado"`
	Revocado    string `json:"revocado,omitempty"`
	// Ambiente: solo en las respuestas que escriben (agente nuevo/revocar).
	Ambiente string `json:"ambiente,omitempty"`
}

type tokenJSON struct {
	ID        string   `json:"id"`
	Nombre    string   `json:"nombre"`
	ActorID   string   `json:"actor_id"`
	Agente    string   `json:"agente"`
	Permisos  []string `json:"permisos"`
	Creado    string   `json:"creado"`
	Vence     string   `json:"vence"`
	UltimoUso string   `json:"ultimo_uso,omitempty"`
	Revocado  string   `json:"revocado,omitempty"`
	Estado    string   `json:"estado"` // activo, vencido o revocado
	// EsPersona: el token es de la persona misma, no de un agente.
	EsPersona bool `json:"es_persona"`
	// Ambiente: solo en tk token revocar (en token nuevo va arriba).
	Ambiente string `json:"ambiente,omitempty"`
}

type tokenNuevoJSON struct {
	Token   tokenJSON `json:"token"`
	Secreto string    `json:"secreto"`
	Aviso   string    `json:"aviso"`
	// Ambiente donde se creó el token (qa o prod).
	Ambiente string `json:"ambiente"`
}

const avisoUnaVez = "Este token no se vuelve a mostrar: guárdalo ahora (p. ej. en la variable de entorno de usuario TRACKER_TOKEN). Nunca en un archivo del repo."

func fechaOpt(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return fecha(*t)
}

func agenteDe(ac api.Actor) agenteJSON {
	return agenteJSON{ID: ac.ID, Nombre: ac.DisplayName, Herramienta: herramienta(ac.Tool),
		Creado: fecha(ac.CreatedAt), Revocado: fechaOpt(ac.RevokedAt)}
}

func (a *app) tokenDe(t api.Token, nombres map[string]string) tokenJSON {
	j := tokenJSON{ID: t.ID, Nombre: t.Name, ActorID: t.ActorID, Agente: nombres[t.ActorID],
		Creado: fecha(t.CreatedAt), Vence: fecha(t.ExpiresAt), UltimoUso: fechaOpt(t.LastUsedAt),
		Revocado: fechaOpt(t.RevokedAt), Permisos: []string{}}
	for _, s := range t.Scopes {
		j.Permisos = append(j.Permisos, permiso(s))
	}
	switch {
	case t.RevokedAt != nil:
		j.Estado = "revocado"
	case !t.ExpiresAt.IsZero() && !t.ExpiresAt.After(a.ahora()):
		j.Estado = "vencido"
	default:
		j.Estado = "activo"
	}
	return j
}

func (a *app) ahora() time.Time {
	if a.ent.Ahora != nil {
		return a.ent.Ahora()
	}
	return time.Now()
}

// nombresActores: id -> nombre (el display_name; el de la persona puede
// venir vacío). La tabla de tk tokens muestra "(tú)" para la persona; el
// JSON lleva el nombre tal cual y es_persona.
func nombresActores(actores []api.Actor) (map[string]string, map[string]bool) {
	m := make(map[string]string, len(actores))
	personas := map[string]bool{}
	for _, ac := range actores {
		m[ac.ID] = ac.DisplayName
		if ac.Kind == "ACTOR_KIND_PERSON" {
			personas[ac.ID] = true
		}
	}
	return m, personas
}

// pareceToken: el valor de un tka_ pegado donde va un nombre o un id. Se
// rechaza sin repetirlo en el error.
func pareceToken(s string) bool { return strings.HasPrefix(strings.TrimSpace(s), "tka_") }

var errPareceToken = usoErr("eso parece el valor de un token tka_, no un nombre ni un id: no lo pegues en comandos (queda en el historial); si se filtró, revócalo")

// agentePorNombre busca un agente vigente por nombre (sin distinguir
// mayúsculas, acentos ni espacios de más).
func (a *app) agentePorNombre(ctx context.Context, nombre string) (api.Actor, error) {
	if pareceToken(nombre) {
		return api.Actor{}, errPareceToken
	}
	actores, err := a.api().ListarActores(ctx, false)
	if err != nil {
		return api.Actor{}, err
	}
	var vigentes []string
	for _, ac := range actores {
		if ac.Kind != "ACTOR_KIND_AGENT" || ac.RevokedAt != nil {
			continue
		}
		if normalizar(ac.DisplayName) == normalizar(nombre) {
			return ac, nil
		}
		vigentes = append(vigentes, ac.DisplayName)
	}
	if len(vigentes) == 0 {
		return api.Actor{}, usoErr("no hay un agente %q: no tienes agentes vigentes (%s)", nombre, a.sugerir("tk agente nuevo"))
	}
	return api.Actor{}, usoErr("no hay un agente %q; agentes: %s", nombre, strings.Join(vigentes, ", "))
}

// parsearVence acepta "30d" o "12h"; entre 1 h y 90 días.
func parsearVence(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	unidad := time.Duration(0)
	n := s
	if v, ok := strings.CutSuffix(s, "d"); ok {
		n, unidad = v, 24*time.Hour
	} else if v, ok := strings.CutSuffix(s, "h"); ok {
		n, unidad = v, time.Hour
	}
	v, err := strconv.Atoi(n)
	if unidad == 0 || err != nil {
		return 0, usoErr("--vence %q: usa días u horas, p. ej. 30d o 12h", s)
	}
	// Rango antes de multiplicar: un número enorme no puede desbordar.
	if v < 1 || int64(v) > int64(venceMaximo/unidad) {
		return 0, usoErr("--vence %q: entre 1h y 90d", s)
	}
	return time.Duration(v) * unidad, nil
}

// --- Comandos ----------------------------------------------------------------

func (a *app) cmdAgentes() *cobra.Command {
	var revocados bool
	cmd := &cobra.Command{
		Use:   "agentes",
		Short: "Lista tus agentes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			actores, err := a.api().ListarActores(cmd.Context(), revocados)
			if err != nil {
				return err
			}
			out := []agenteJSON{}
			for _, ac := range actores {
				if ac.Kind == "ACTOR_KIND_AGENT" {
					out = append(out, agenteDe(ac))
				}
			}
			if a.json {
				escribirJSON(a.ent.Out, out)
				return nil
			}
			if len(out) == 0 {
				fmt.Fprintf(a.ent.Out, "No tienes agentes. Crea uno con %s.\n", a.sugerir(`tk agente nuevo "<nombre>" --herramienta claude-code`))
				return nil
			}
			filas := make([][]string, 0, len(out))
			for _, g := range out {
				filas = append(filas, []string{g.Nombre, g.Herramienta, g.ID, g.Creado, g.Revocado})
			}
			tabla(a.ent.Out, []string{"NOMBRE", "HERRAMIENTA", "ID", "CREADO", "REVOCADO"}, filas)
			return nil
		},
	}
	cmd.Flags().BoolVar(&revocados, "revocados", false, "incluye los agentes revocados")
	return cmd
}

func (a *app) cmdAgente() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agente",
		Short: "Da de alta o revoca un agente",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return usoErr("falta el subcomando: tk agente nuevo | tk agente revocar")
		},
	}
	cmd.AddCommand(a.cmdAgenteNuevo(), a.cmdAgenteRevocar())
	return cmd
}

func (a *app) cmdAgenteNuevo() *cobra.Command {
	var herr string
	cmd := &cobra.Command{
		Use:     "nuevo \"<nombre>\" --herramienta claude-code|codex|cloud|otra",
		Short:   "Da de alta un agente",
		Example: `  tk agente nuevo "Claude Code PC-01" --herramienta claude-code`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			nombre := strings.TrimSpace(args[0])
			if pareceToken(nombre) {
				return errPareceToken
			}
			if nombre == "" || len([]rune(nombre)) > 80 {
				return usoErr("el nombre del agente va de 1 a 80 caracteres")
			}
			tool, ok := herramientas[strings.ToLower(strings.TrimSpace(herr))]
			if !ok {
				return usoErr("--herramienta %q: usa claude-code, codex, cloud u otra", herr)
			}
			ag, err := a.api().CrearAgente(cmd.Context(), nombre, tool)
			if err != nil {
				return err
			}
			g := agenteDe(ag)
			g.Ambiente = a.cfg.Ambiente
			if a.json {
				escribirJSON(a.ent.Out, g)
				return nil
			}
			fmt.Fprintf(a.ent.Out, "%sAgente %q creado (%s, id %s). Sácale un token con:\n  %s\n",
				a.etiqueta(), g.Nombre, g.Herramienta, g.ID,
				a.sugerir(fmt.Sprintf("tk token nuevo --agente %q --permiso issues:read", g.Nombre)))
			return nil
		},
	}
	cmd.Flags().StringVar(&herr, "herramienta", "", "claude-code, codex, cloud u otra (obligatorio)")
	_ = cmd.MarkFlagRequired("herramienta")
	return cmd
}

func (a *app) cmdAgenteRevocar() *cobra.Command {
	return &cobra.Command{
		Use:     "revocar \"<nombre>\"",
		Short:   "Revoca un agente y todos sus tokens (no se deshace)",
		Example: `  tk agente revocar "Claude Code PC-01"`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			ag, err := a.agentePorNombre(ctx, args[0])
			if err != nil {
				return err
			}
			rev, err := a.api().RevocarActor(ctx, ag.ID)
			if err != nil {
				return err
			}
			g := agenteDe(rev)
			g.Ambiente = a.cfg.Ambiente
			if a.json {
				escribirJSON(a.ent.Out, g)
				return nil
			}
			fmt.Fprintf(a.ent.Out, "%sAgente %q revocado, con todos sus tokens.\n", a.etiqueta(), g.Nombre)
			return nil
		},
	}
}

func (a *app) cmdTokens() *cobra.Command {
	var todos bool
	cmd := &cobra.Command{
		Use:   "tokens",
		Short: "Lista tus tokens y los de tus agentes (nunca el valor)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			toks, err := a.api().ListarTokens(ctx, todos)
			if err != nil {
				return err
			}
			actores, err := a.api().ListarActores(ctx, true)
			if err != nil {
				return err
			}
			nombres, personas := nombresActores(actores)
			out := make([]tokenJSON, 0, len(toks))
			for _, t := range toks {
				j := a.tokenDe(t, nombres)
				j.EsPersona = personas[t.ActorID]
				out = append(out, j)
			}
			if a.json {
				escribirJSON(a.ent.Out, out)
				return nil
			}
			if len(out) == 0 {
				fmt.Fprintf(a.ent.Out, "No hay tokens. Crea uno con %s.\n", a.sugerir(`tk token nuevo --agente "<nombre>" --permiso issues:read`))
				return nil
			}
			filas := make([][]string, 0, len(out))
			for _, t := range out {
				agente := t.Agente
				if t.EsPersona {
					agente = "(tú)"
				}
				filas = append(filas, []string{t.ID, agente, strings.Join(t.Permisos, ","), t.Vence, t.Estado, t.UltimoUso})
			}
			tabla(a.ent.Out, []string{"ID", "AGENTE", "PERMISOS", "VENCE", "ESTADO", "ÚLTIMO USO"}, filas)
			return nil
		},
	}
	cmd.Flags().BoolVar(&todos, "todos", false, "incluye los revocados y los vencidos")
	return cmd
}

func (a *app) cmdToken() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Crea o revoca un token de agente",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return usoErr("falta el subcomando: tk token nuevo | tk token revocar")
		},
	}
	cmd.AddCommand(a.cmdTokenNuevo(), a.cmdTokenRevocar())
	return cmd
}

func (a *app) cmdTokenNuevo() *cobra.Command {
	var agente, vence, nombre string
	var perms []string
	cmd := &cobra.Command{
		Use:   "nuevo --agente \"<nombre>\" --permiso issues:read [--permiso issues:write] [--vence 30d]",
		Short: "Crea un token tka_ para un agente y lo muestra una sola vez",
		Example: `  tk token nuevo --agente "Claude Code PC-01" --permiso issues:read
  tk token nuevo --agente "Codex PC-01" --permiso issues:read --permiso issues:write --vence 7d --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			scopes := map[string]bool{}
			for _, p := range perms {
				for _, s := range strings.Split(p, ",") {
					v, ok := permisos[strings.ToLower(strings.TrimSpace(s))]
					if !ok {
						return usoErr("--permiso %q: usa issues:read o issues:write", s)
					}
					scopes[v] = true
				}
			}
			if len(scopes) == 0 {
				return usoErr("falta --permiso (issues:read y/o issues:write)")
			}
			n := api.NuevoToken{Name: strings.TrimSpace(nombre)}
			for s := range scopes {
				n.Scopes = append(n.Scopes, s)
			}
			sort.Strings(n.Scopes) // READ antes que WRITE
			if len([]rune(n.Name)) > 80 {
				return usoErr("--nombre va hasta 80 caracteres")
			}
			if vence != "" {
				d, err := parsearVence(vence)
				if err != nil {
					return err
				}
				if d > venceMaximo-margenReloj {
					d = venceMaximo - margenReloj
				}
				t := a.ahora().Add(d).UTC().Truncate(time.Second)
				n.ExpiresAt = &t
			}
			ag, err := a.agentePorNombre(ctx, agente)
			if err != nil {
				return err
			}
			n.ActorID = ag.ID
			if n.Name == "" {
				n.Name = ag.DisplayName
			}
			tok, secreto, err := a.api().CrearToken(ctx, n)
			if err != nil {
				return err
			}
			if secreto == "" {
				return fmt.Errorf("la API no devolvió el token (id %s): revócalo con %s y crea otro", tok.ID, a.sugerir("tk token revocar "+tok.ID))
			}
			j := a.tokenDe(tok, map[string]string{ag.ID: ag.DisplayName})
			if a.json {
				escribirJSON(a.ent.Out, tokenNuevoJSON{Token: j, Secreto: secreto, Aviso: avisoUnaVez, Ambiente: a.cfg.Ambiente})
				return nil
			}
			// El aviso y los datos a stderr; stdout solo el token, para poder
			// capturarlo sin copiarlo a mano.
			fmt.Fprintf(a.ent.Err, "%sToken %s para %q (%s), vence %s.\n%s\n",
				a.etiqueta(), j.ID, j.Agente, strings.Join(j.Permisos, ", "), j.Vence, avisoUnaVez)
			fmt.Fprintln(a.ent.Out, secreto)
			return nil
		},
	}
	cmd.Flags().StringVar(&agente, "agente", "", "nombre del agente (obligatorio)")
	cmd.Flags().StringArrayVar(&perms, "permiso", nil, "issues:read o issues:write (repetible)")
	cmd.Flags().StringVar(&vence, "vence", "", "vencimiento, p. ej. 30d o 12h (default 30d, máximo 90d)")
	cmd.Flags().StringVar(&nombre, "nombre", "", "nombre del token para reconocerlo en tk tokens (default: el del agente)")
	_ = cmd.MarkFlagRequired("agente")
	return cmd
}

func (a *app) cmdTokenRevocar() *cobra.Command {
	return &cobra.Command{
		Use:     "revocar <id>",
		Short:   "Revoca un token (no se deshace; el gateway puede aceptarlo hasta 60 s más)",
		Example: `  tk token revocar 8f0c…`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" || pareceToken(id) {
				return usoErr("usa el id del token (columna ID de %s), no su valor", a.sugerir("tk tokens"))
			}
			ctx := cmd.Context()
			tok, err := a.api().RevocarToken(ctx, id)
			if err != nil {
				return err
			}
			actores, err := a.api().ListarActores(ctx, true)
			if err != nil {
				return err
			}
			nombres, personas := nombresActores(actores)
			j := a.tokenDe(tok, nombres)
			j.EsPersona = personas[tok.ActorID]
			j.Ambiente = a.cfg.Ambiente
			if a.json {
				escribirJSON(a.ent.Out, j)
				return nil
			}
			fmt.Fprintf(a.ent.Out, "%sToken %s revocado. El gateway puede aceptarlo hasta 60 s más.\n", a.etiqueta(), j.ID)
			return nil
		},
	}
}
