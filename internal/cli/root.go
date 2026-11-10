// Package cli arma los comandos de tk.
//
// Códigos de salida (issue 029):
//
//	0  bien
//	1  error de uso o de la API
//	2  sin sesión o sesión vencida: hay que correr tk login
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime/debug"
	"time"

	"github.com/spf13/cobra"

	"github.com/tekitla/tekitla-cli/internal/api"
	"github.com/tekitla/tekitla-cli/internal/auth"
	"github.com/tekitla/tekitla-cli/internal/config"
)

// Códigos de salida.
const (
	SalidaOK     = 0
	SalidaError  = 1
	SalidaSesion = 2
)

// Versión, commit y fecha: los llena GoReleaser con -ldflags.
var (
	Version = "dev"
	Commit  = ""
	Fecha   = ""
)

// Entorno son las dependencias externas de tk; los tests las reemplazan.
type Entorno struct {
	Out, Err io.Writer
	// HTTP para Google, Firebase y la API. nil: un cliente con timeout.
	HTTP *http.Client
	// Endpoints de Google y Firebase. Vacío: los reales.
	Auth auth.Endpoints
	// Llavero del sistema. nil: sin llavero (sesión en archivo 0600).
	Llavero auth.Llavero
	// Abrir abre una URL en el navegador.
	Abrir func(url string) error
	// Dormir espera entre sondeos del device flow.
	Dormir func(ctx context.Context, d time.Duration) error
	// Ahora para la expiración de los tokens.
	Ahora func() time.Time
}

// EntornoReal usa la terminal, el llavero del sistema y la red.
func EntornoReal() Entorno {
	return Entorno{
		Out:     os.Stdout,
		Err:     os.Stderr,
		Llavero: auth.LlaveroSistema{},
		Abrir:   abrirNavegador,
	}
}

type app struct {
	ent          Entorno
	json         bool
	flagAmbiente string
	cfg          config.Config
	cliente      *api.Cliente
}

// errUso marca un error de uso (código 1) con un mensaje propio.
type errUso struct{ msg string }

func (e *errUso) Error() string { return e.msg }

func usoErr(format string, a ...any) error { return &errUso{msg: fmt.Sprintf(format, a...)} }

// Ejecutar corre tk con args y devuelve el código de salida.
func Ejecutar(ctx context.Context, args []string, ent Entorno) int {
	if ent.HTTP == nil {
		ent.HTTP = &http.Client{Timeout: 30 * time.Second}
	}
	if ent.Auth == (auth.Endpoints{}) {
		ent.Auth = auth.EndpointsReales
	}
	if ent.Abrir == nil {
		ent.Abrir = func(string) error { return nil }
	}
	a := &app{ent: ent}
	root := a.raiz()
	root.SetArgs(args)
	root.SetOut(ent.Out)
	root.SetErr(ent.Err)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return SalidaOK
	}
	return a.reportar(err)
}

func (a *app) raiz() *cobra.Command {
	root := &cobra.Command{
		Use:           "tk",
		Short:         "Tekitla desde la terminal: proyectos, issues, agentes y tokens",
		Long:          "tk es la CLI de Tekitla. Usa --json para una salida estable (agentes y scripts).",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Resolver(a.flagAmbiente)
			if err != nil {
				return &errUso{msg: err.Error()}
			}
			a.cfg = cfg
			return nil
		},
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "salida JSON estable, sin colores ni tablas")
	root.PersistentFlags().StringVar(&a.flagAmbiente, "ambiente", "", "qa o prod (default: TK_AMBIENTE, config.toml o prod)")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return &errUso{msg: err.Error()} })

	root.AddCommand(
		a.cmdLogin(),
		a.cmdLogout(),
		a.cmdProyectos(),
		a.cmdIssues(),
		a.cmdIssue(),
		a.cmdMover(),
		a.cmdComentar(),
		a.cmdAgentes(),
		a.cmdAgente(),
		a.cmdTokens(),
		a.cmdToken(),
		a.cmdVersion(),
	)
	return root
}

// almacen y api se arman bajo demanda: version y logout no tocan la red.
func (a *app) almacen() *auth.Almacen {
	return &auth.Almacen{Dir: a.cfg.Dir, Llavero: a.ent.Llavero}
}

func (a *app) firebase() *auth.Firebase {
	return &auth.Firebase{HTTP: a.ent.HTTP, Endpoints: a.ent.Auth, APIKey: config.FirebaseAPIKey, Ahora: a.ent.Ahora}
}

func (a *app) api() *api.Cliente {
	if a.cliente == nil {
		fuente := &auth.Fuente{Almacen: a.almacen(), Firebase: a.firebase()}
		a.cliente = api.Nuevo(a.cfg.APIURL, fuente, "tk/"+version())
		a.cliente.HTTP = a.ent.HTTP
	}
	return a.cliente
}

func version() string {
	if Version != "dev" {
		return Version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return Version
}

// reportar escribe el error (texto o JSON en stderr) y devuelve el código.
func (a *app) reportar(err error) int {
	codigo := SalidaError
	var es *api.ErrorSesion
	if errors.As(err, &es) || errors.Is(err, auth.ErrSinSesion) || errors.Is(err, auth.ErrSesionInvalida) {
		codigo = SalidaSesion
		err = &errUso{msg: "no hay sesión o venció: corre tk login"}
	}
	if a.json {
		cuerpo := map[string]any{"message": err.Error()}
		var ea *api.ErrorAPI
		switch {
		case codigo == SalidaSesion:
			cuerpo["code"] = "UNAUTHENTICATED"
		case errors.As(err, &ea):
			cuerpo["code"] = ea.Code
			cuerpo["message"] = ea.Message
			cuerpo["status"] = ea.Status
			if ea.Reason != "" {
				cuerpo["reason"] = ea.Reason
			}
			if ea.RequestID != "" {
				cuerpo["request_id"] = ea.RequestID
			}
		default:
			var ee *errEstado
			if errors.As(err, &ee) {
				cuerpo["code"] = "ESTADO_DESCONOCIDO"
				cuerpo["estados_validos"] = ee.validos
			} else {
				cuerpo["code"] = "USO"
			}
		}
		escribirJSON(a.ent.Err, map[string]any{"error": cuerpo})
		return codigo
	}
	fmt.Fprintln(a.ent.Err, "tk:", err)
	return codigo
}
