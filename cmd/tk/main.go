// Comando tk: la CLI de Tekitla.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/tekitla/tekitla-cli/internal/cli"
)

func main() {
	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt)
	codigo := cli.Ejecutar(ctx, os.Args[1:], cli.EntornoReal())
	cancelar()
	os.Exit(codigo)
}
