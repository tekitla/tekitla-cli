package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tekitla/tekitla-cli/internal/auth"
)

func (a *app) cmdLogin() *cobra.Command {
	var sinNavegador bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Inicia sesión con Google (device flow)",
		Long: `Inicia sesión con tu cuenta de Google (la misma del panel).

tk muestra una URL y un código: ábrela en cualquier dispositivo, ingresa el
código y aprueba. Funciona por SSH. La sesión queda en el llavero del sistema
(o en ~/.config/tk/sesion.json con permisos 0600 si no hay llavero).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			g := &auth.Google{
				HTTP:         a.ent.HTTP,
				Endpoints:    a.ent.Auth,
				ClientID:     a.cfg.OAuthClientID,
				ClientSecret: a.cfg.OAuthClientSecret,
				Dormir:       a.ent.Dormir,
			}
			codigo, err := g.PedirCodigo(ctx)
			if err != nil {
				return &errUso{msg: err.Error()}
			}
			// Las instrucciones van a stderr: stdout queda para el resultado
			// (--json).
			fmt.Fprintf(a.ent.Err, "Abre %s e ingresa el código: %s\n", codigo.VerificationURL, codigo.UserCode)
			if !sinNavegador {
				_ = a.ent.Abrir(codigo.VerificationURL)
			}
			fmt.Fprintln(a.ent.Err, "Esperando la aprobación…")
			idGoogle, err := g.EsperarAprobacion(ctx, codigo)
			if err != nil {
				return &errUso{msg: err.Error()}
			}
			s, err := a.firebase().Canjear(ctx, idGoogle)
			if err != nil {
				return &errUso{msg: err.Error()}
			}
			alm := a.almacen()
			if err := alm.Guardar(s); err != nil {
				return &errUso{msg: err.Error()}
			}
			if a.json {
				escribirJSON(a.ent.Out, map[string]string{"email": s.Email, "almacen": alm.Donde()})
				return nil
			}
			fmt.Fprintf(a.ent.Out, "Sesión iniciada como %s (guardada en el %s).\n", s.Email, alm.Donde())
			return nil
		},
	}
	cmd.Flags().BoolVar(&sinNavegador, "no-browser", false, "no abrir el navegador: solo imprime la URL y el código")
	return cmd
}

func (a *app) cmdLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Borra la sesión local",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := a.almacen().Borrar(); err != nil {
				return &errUso{msg: err.Error()}
			}
			if a.json {
				escribirJSON(a.ent.Out, map[string]bool{"sesion_borrada": true})
				return nil
			}
			fmt.Fprintln(a.ent.Out, "Sesión borrada.")
			return nil
		},
	}
}
