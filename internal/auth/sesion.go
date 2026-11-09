package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

// margen: el ID token se renueva si le queda menos que esto.
const margen = 5 * time.Minute

// Fuente entrega un ID token válido, renovándolo con el refresh token cuando
// hace falta, y guarda la sesión renovada.
type Fuente struct {
	Almacen  *Almacen
	Firebase *Firebase

	mu     sync.Mutex
	sesion *Sesion
}

// IDToken devuelve un ID token vigente. forzar renueva aunque no haya
// vencido (después de un 401). Sin sesión, o con una revocada, devuelve
// ErrSinSesion o ErrSesionInvalida.
func (f *Fuente) IDToken(ctx context.Context, forzar bool) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sesion == nil {
		s, err := f.Almacen.Leer()
		if err != nil {
			return "", err
		}
		f.sesion = &s
	}
	s := *f.sesion
	if !forzar && s.IDToken != "" && f.Firebase.ahora().Add(margen).Before(s.Expira) {
		return s.IDToken, nil
	}
	nueva, err := f.Firebase.Renovar(ctx, s)
	if err != nil {
		if errors.Is(err, ErrSesionInvalida) {
			_ = f.Almacen.Borrar()
		}
		return "", err
	}
	f.sesion = &nueva
	// Si no se puede guardar, el token sirve igual para este comando; el
	// siguiente vuelve a renovar con el refresh token que ya estaba.
	_ = f.Almacen.Guardar(nueva)
	return nueva.IDToken, nil
}
