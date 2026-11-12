package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tekitla/tekitla-cli/internal/api"
	"github.com/tekitla/tekitla-cli/internal/config"
)

// eventosEnDetalle: cuántos eventos recientes muestra tk issue.
const eventosEnDetalle = 10

// errEstado: el nombre de estado no existe en el proyecto (CU-029-4).
type errEstado struct {
	nombre  string
	validos []string
}

func (e *errEstado) Error() string {
	return fmt.Sprintf("el estado %q no existe en el proyecto; estados válidos: %s",
		e.nombre, strings.Join(e.validos, ", "))
}

// --- Resolución de proyectos y estados ---------------------------------------

func (a *app) proyecto(ctx context.Context, clave string) (api.Proyecto, error) {
	if !claveValida(strings.TrimSpace(clave)) {
		return api.Proyecto{}, usoErr("%q no es una clave de proyecto (2 a 10 letras o dígitos, p. ej. TK)", clave)
	}
	ps, err := a.api().ListarProyectos(ctx)
	if err != nil {
		return api.Proyecto{}, err
	}
	for _, p := range ps {
		if strings.EqualFold(p.Key, strings.TrimSpace(clave)) {
			return p, nil
		}
	}
	claves := make([]string, 0, len(ps))
	for _, p := range ps {
		claves = append(claves, p.Key)
	}
	if len(claves) == 0 {
		return api.Proyecto{}, usoErr("no existe el proyecto %s (no ves ningún proyecto)", strings.ToUpper(clave))
	}
	return api.Proyecto{}, usoErr("no existe el proyecto %s; proyectos: %s", strings.ToUpper(clave), strings.Join(claves, ", "))
}

func buscarEstado(estados []api.Estado, nombre string) (api.Estado, error) {
	n := normalizar(nombre)
	for _, e := range estados {
		if normalizar(e.Name) == n {
			return e, nil
		}
	}
	validos := make([]string, 0, len(estados))
	for _, e := range estados {
		validos = append(validos, e.Name)
	}
	return api.Estado{}, &errEstado{nombre: nombre, validos: validos}
}

func nombresEstados(estados []api.Estado) map[string]string {
	m := make(map[string]string, len(estados))
	for _, e := range estados {
		m[e.ID] = e.Name
	}
	return m
}

func resumenIssue(p api.Proyecto, i api.Issue, estados map[string]string) issueJSON {
	return issueJSON{
		Clave:     claveIssue(p, i.Number),
		Numero:    i.Number,
		Titulo:    i.Title,
		Estado:    estados[i.StatusID],
		Categoria: categoria(i.State),
		Prioridad: prioridad(i.Priority),
	}
}

// issueDe resuelve CLAVE-N al proyecto y número.
func (a *app) issueDe(ctx context.Context, clave string) (api.Proyecto, int, error) {
	k, n, err := parsearClave(clave)
	if err != nil {
		return api.Proyecto{}, 0, err
	}
	p, err := a.proyecto(ctx, k)
	return p, n, err
}

// --- Comandos ----------------------------------------------------------------

func (a *app) cmdProyectos() *cobra.Command {
	return &cobra.Command{
		Use:   "proyectos",
		Short: "Lista los proyectos visibles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ps, err := a.api().ListarProyectos(cmd.Context())
			if err != nil {
				return err
			}
			if a.json {
				out := make([]proyectoJSON, 0, len(ps))
				for _, p := range ps {
					out = append(out, proyectoJSON{Clave: p.Key, Nombre: p.Name, Descripcion: p.Description, ID: p.ID})
				}
				escribirJSON(a.ent.Out, out)
				return nil
			}
			if len(ps) == 0 {
				fmt.Fprintln(a.ent.Out, "No hay proyectos.")
				return nil
			}
			filas := make([][]string, 0, len(ps))
			for _, p := range ps {
				filas = append(filas, []string{p.Key, p.Name})
			}
			tabla(a.ent.Out, []string{"CLAVE", "NOMBRE"}, filas)
			return nil
		},
	}
}

func (a *app) cmdIssues() *cobra.Command {
	var proyecto, estado, buscar string
	var prioridadesFlag []string
	cmd := &cobra.Command{
		Use:   "issues -p <CLAVE>",
		Short: "Lista los issues de un proyecto",
		Example: `  tk issues -p TK
  tk issues -p TK --estado "En curso" --prioridad alta,urgente
  tk issues -p TK --buscar login --json`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			var f api.FiltrosIssues
			for _, p := range prioridadesFlag {
				pr, err := parsearPrioridad(p)
				if err != nil {
					return err
				}
				f.Priorities = append(f.Priorities, pr)
			}
			f.Query = strings.TrimSpace(buscar)
			p, err := a.proyecto(ctx, proyecto)
			if err != nil {
				return err
			}
			estados, err := a.api().ListarEstados(ctx, p.ID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(estado) != "" {
				e, err := buscarEstado(estados, estado)
				if err != nil {
					return err
				}
				f.StatusIDs = []string{e.ID}
			}
			issues, err := a.api().ListarIssues(ctx, p.ID, f)
			if err != nil {
				return err
			}
			nombres := nombresEstados(estados)
			out := make([]issueJSON, 0, len(issues))
			for _, i := range issues {
				out = append(out, resumenIssue(p, i, nombres))
			}
			if a.json {
				escribirJSON(a.ent.Out, out)
				return nil
			}
			if len(out) == 0 {
				fmt.Fprintln(a.ent.Out, "No hay issues.")
				return nil
			}
			filas := make([][]string, 0, len(out))
			for _, i := range out {
				filas = append(filas, []string{i.Clave, i.Estado, i.Prioridad, i.Titulo})
			}
			tabla(a.ent.Out, []string{"CLAVE", "ESTADO", "PRIORIDAD", "TÍTULO"}, filas)
			return nil
		},
	}
	cmd.Flags().StringVarP(&proyecto, "proyecto", "p", "", "clave del proyecto (obligatorio)")
	cmd.Flags().StringVar(&estado, "estado", "", "nombre del estado, p. ej. \"En curso\"")
	cmd.Flags().StringSliceVar(&prioridadesFlag, "prioridad", nil, "baja, media, alta o urgente (varias separadas por coma)")
	cmd.Flags().StringVar(&buscar, "buscar", "", "texto en el título, o la clave o número del issue")
	_ = cmd.MarkFlagRequired("proyecto")
	return cmd
}

func (a *app) cmdIssue() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue <CLAVE-N>",
		Short: "Detalle de un issue (o tk issue nuevo para crear uno)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p, n, err := a.issueDe(ctx, args[0])
			if err != nil {
				return err
			}
			c := a.api()
			i, err := c.ObtenerIssue(ctx, p.ID, n)
			if err != nil {
				return err
			}
			estados, err := c.ListarEstados(ctx, p.ID)
			if err != nil {
				return err
			}
			etiquetas, err := c.ListarEtiquetas(ctx, p.ID)
			if err != nil {
				return err
			}
			eventos, err := c.ListarEventos(ctx, p.ID, n)
			if err != nil {
				return err
			}
			d := detalle(p, i, estados, etiquetas, eventos)
			if a.json {
				escribirJSON(a.ent.Out, d)
				return nil
			}
			imprimirDetalle(a, d)
			return nil
		},
	}
	cmd.AddCommand(a.cmdIssueNuevo())
	return cmd
}

func detalle(p api.Proyecto, i api.Issue, estados []api.Estado, etiquetas []api.Etiqueta, eventos []api.Evento) issueDetalleJSON {
	nombresEt := make(map[string]string, len(etiquetas))
	for _, e := range etiquetas {
		nombresEt[e.ID] = e.Name
	}
	ets := make([]string, 0, len(i.LabelIDs))
	for _, id := range i.LabelIDs {
		if n, ok := nombresEt[id]; ok {
			ets = append(ets, n)
		}
	}
	if len(eventos) > eventosEnDetalle {
		eventos = eventos[len(eventos)-eventosEnDetalle:]
	}
	evs := make([]eventoJSON, 0, len(eventos))
	for _, e := range eventos {
		evs = append(evs, describirEvento(e))
	}
	return issueDetalleJSON{
		issueJSON:   resumenIssue(p, i, nombresEstados(estados)),
		Etiquetas:   ets,
		Descripcion: i.Description,
		Creado:      fecha(i.CreatedAt),
		Actualizado: fecha(i.UpdatedAt),
		Eventos:     evs,
	}
}

func fecha(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func describirEvento(e api.Evento) eventoJSON {
	out := eventoJSON{Fecha: fecha(e.CreatedAt), ActorID: e.ActorID, AgentID: e.AgentID}
	switch {
	case e.Created != nil:
		out.Tipo, out.Resumen = "creado", "creó el issue"
	case e.StatusChanged != nil:
		out.Tipo = "estado"
		out.Resumen = fmt.Sprintf("movió de «%s» a «%s»", e.StatusChanged.FromStatusName, e.StatusChanged.ToStatusName)
	case e.TitleChanged != nil:
		out.Tipo, out.Resumen = "titulo", fmt.Sprintf("cambió el título a «%s»", e.TitleChanged.To)
	case e.DescChanged != nil:
		out.Tipo, out.Resumen = "descripcion", "editó la descripción"
	case e.PriorityChanged != nil:
		out.Tipo = "prioridad"
		out.Resumen = fmt.Sprintf("prioridad: %s → %s", prioridad(e.PriorityChanged.From), prioridad(e.PriorityChanged.To))
	case e.LabelsChanged != nil:
		var partes []string
		for _, n := range e.LabelsChanged.AddedLabelNames {
			partes = append(partes, "+"+n)
		}
		for _, n := range e.LabelsChanged.RemovedLabelNames {
			partes = append(partes, "-"+n)
		}
		out.Tipo, out.Resumen = "etiquetas", "etiquetas: "+strings.Join(partes, " ")
	case e.Commented != nil:
		out.Tipo = "comentario"
		out.Resumen = "comentó: " + primeraLinea(e.Commented.Body, 80)
		out.Texto = e.Commented.Body
	case e.Discarded != nil:
		out.Tipo, out.Resumen = "descartado", "descartó: "+e.Discarded.Reason
	case e.Restored != nil:
		out.Tipo, out.Resumen = "restaurado", fmt.Sprintf("restauró a «%s»", e.Restored.ToStatusName)
	case e.AssigneeChanged != nil:
		out.Tipo = "asignado"
		if e.AssigneeChanged.ToDisplayName == "" {
			out.Resumen = "quitó el asignado"
		} else {
			out.Resumen = "asignó a " + e.AssigneeChanged.ToDisplayName
		}
	default:
		out.Tipo, out.Resumen = "otro", "cambio"
	}
	return out
}

func imprimirDetalle(a *app, d issueDetalleJSON) {
	w := a.ent.Out
	fmt.Fprintf(w, "%s  %s\n", d.Clave, d.Titulo)
	fmt.Fprintf(w, "Estado:    %s (%s)\n", d.Estado, d.Categoria)
	fmt.Fprintf(w, "Prioridad: %s\n", d.Prioridad)
	if len(d.Etiquetas) > 0 {
		fmt.Fprintf(w, "Etiquetas: %s\n", strings.Join(d.Etiquetas, ", "))
	}
	if strings.TrimSpace(d.Descripcion) != "" {
		fmt.Fprintf(w, "\n%s\n", strings.TrimSpace(d.Descripcion))
	}
	if len(d.Eventos) > 0 {
		fmt.Fprintln(w, "\nÚltimos eventos:")
		for _, e := range d.Eventos {
			fmt.Fprintf(w, "  %s  %s\n", e.Fecha, e.Resumen)
		}
	}
}

func (a *app) cmdIssueNuevo() *cobra.Command {
	var proyecto, prio, desc string
	cmd := &cobra.Command{
		Use:     "nuevo -p <CLAVE> \"<título>\"",
		Short:   "Crea un issue",
		Example: `  tk issue nuevo -p TK "Exportar a CSV" --prioridad alta --descripcion "Desde el tablero"`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			titulo := strings.TrimSpace(args[0])
			if titulo == "" {
				return usoErr("el título no puede estar vacío")
			}
			n := api.NuevoIssue{Title: titulo, Description: desc}
			if prio != "" {
				p, err := parsearPrioridad(prio)
				if err != nil {
					return err
				}
				n.Priority = p
			}
			p, err := a.proyecto(ctx, proyecto)
			if err != nil {
				return err
			}
			i, err := a.api().CrearIssue(ctx, p.ID, n)
			if err != nil {
				return err
			}
			estados, err := a.api().ListarEstados(ctx, p.ID)
			if err != nil {
				return err
			}
			r := resumenIssue(p, i, nombresEstados(estados))
			if a.json {
				escribirJSON(a.ent.Out, r)
				return nil
			}
			fmt.Fprintf(a.ent.Out, "Creado %s: %s\n", r.Clave, r.Titulo)
			return nil
		},
	}
	cmd.Flags().StringVarP(&proyecto, "proyecto", "p", "", "clave del proyecto (obligatorio)")
	cmd.Flags().StringVar(&prio, "prioridad", "", "baja, media, alta o urgente (default: media)")
	cmd.Flags().StringVar(&desc, "descripcion", "", "descripción en Markdown")
	_ = cmd.MarkFlagRequired("proyecto")
	return cmd
}

func (a *app) cmdMover() *cobra.Command {
	return &cobra.Command{
		Use:     "mover <CLAVE-N> \"<estado>\"",
		Short:   "Cambia el estado de un issue por nombre",
		Example: `  tk mover TK-42 "En curso"`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			p, n, err := a.issueDe(ctx, args[0])
			if err != nil {
				return err
			}
			estados, err := a.api().ListarEstados(ctx, p.ID)
			if err != nil {
				return err
			}
			e, err := buscarEstado(estados, args[1])
			if err != nil {
				return err
			}
			i, err := a.api().MoverIssue(ctx, p.ID, n, e.ID)
			if err != nil {
				return err
			}
			r := resumenIssue(p, i, nombresEstados(estados))
			if a.json {
				escribirJSON(a.ent.Out, r)
				return nil
			}
			fmt.Fprintf(a.ent.Out, "%s → %s\n", r.Clave, r.Estado)
			return nil
		},
	}
}

func (a *app) cmdComentar() *cobra.Command {
	return &cobra.Command{
		Use:     "comentar <CLAVE-N> \"<texto>\"",
		Short:   "Agrega un comentario a un issue",
		Example: `  tk comentar TK-42 "Listo para revisar"`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			texto := strings.TrimSpace(args[1])
			if texto == "" {
				return usoErr("el comentario no puede estar vacío")
			}
			p, n, err := a.issueDe(ctx, args[0])
			if err != nil {
				return err
			}
			c, err := a.api().Comentar(ctx, p.ID, n, texto)
			if err != nil {
				return err
			}
			if a.json {
				escribirJSON(a.ent.Out, map[string]any{
					"clave":         claveIssue(p, n),
					"comentario_id": c.ID,
					"creado":        fecha(c.CreatedAt),
				})
				return nil
			}
			fmt.Fprintf(a.ent.Out, "Comentario agregado a %s\n", claveIssue(p, n))
			return nil
		},
	}
}

func (a *app) cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Versión de tk y ambiente por defecto",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			// El ambiente por defecto es el que usa un comando sin --ambiente
			// (TK_AMBIENTE, config.toml o prod), no el del último comando.
			// Sin el flag es a.cfg. Con el flag se resuelve aparte; si
			// TK_AMBIENTE o config.toml traen algo inválido, version lo dice
			// en vez de fallar (sin el flag, ya falló en PersistentPreRunE).
			def := a.cfg
			var errDef error
			if a.flagAmbiente != "" {
				def, errDef = config.Resolver("")
			}
			porDefecto := def.Ambiente // "" en el JSON si no es válido
			if errDef != nil {
				porDefecto = ""
			}
			v := map[string]string{
				"version":              version(),
				"commit":               Commit,
				"fecha":                Fecha,
				"ambiente":             a.cfg.Ambiente,
				"api":                  a.cfg.APIURL,
				"ambiente_por_defecto": porDefecto,
			}
			if a.json {
				escribirJSON(a.ent.Out, v)
				return nil
			}
			extra := ""
			if Commit != "" {
				extra = " (" + Commit + ")"
			}
			fmt.Fprintf(a.ent.Out, "tk %s%s\n", version(), extra)
			if errDef != nil {
				fmt.Fprintf(a.ent.Out, "ambiente por defecto: no válido (%v)\n", errDef)
			} else {
				fmt.Fprintf(a.ent.Out, "ambiente por defecto: %s (%s)\n", def.Ambiente, def.APIURL)
			}
			if errDef != nil || a.cfg.Ambiente != def.Ambiente || a.cfg.APIURL != def.APIURL {
				fmt.Fprintf(a.ent.Out, "con --ambiente: %s (%s)\n", a.cfg.Ambiente, a.cfg.APIURL)
			}
			return nil
		},
	}
}
