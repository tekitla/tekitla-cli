package auth

import (
	"errors"
	"testing"
	"time"
)

type llaveroMem struct {
	m     map[string]string
	falla bool
}

func (l *llaveroMem) Get(s, c string) (string, error) {
	v, ok := l.m[s+"/"+c]
	if !ok {
		return "", errors.New("no está")
	}
	return v, nil
}

func (l *llaveroMem) Set(s, c, v string) error {
	if l.falla {
		return errors.New("sin llavero")
	}
	l.m[s+"/"+c] = v
	return nil
}

func (l *llaveroMem) Delete(s, c string) error { delete(l.m, s+"/"+c); return nil }

func sesionDePrueba() Sesion {
	return Sesion{IDToken: "id", RefreshToken: "rt", Expira: time.Unix(1_800_000_000, 0), Email: "a@b.c", UID: "u"}
}

func TestAlmacenLlavero(t *testing.T) {
	l := &llaveroMem{m: map[string]string{}}
	a := &Almacen{Dir: t.TempDir(), Llavero: l}
	if _, err := a.Leer(); !errors.Is(err, ErrSinSesion) {
		t.Fatalf("vacío: %v", err)
	}
	if err := a.Guardar(sesionDePrueba()); err != nil {
		t.Fatal(err)
	}
	if a.Donde() != "llavero" {
		t.Errorf("donde = %q", a.Donde())
	}
	s, err := a.Leer()
	if err != nil || s.IDToken != "id" || s.RefreshToken != "rt" || !s.Expira.Equal(sesionDePrueba().Expira) || s.Email != "a@b.c" {
		t.Fatalf("leída = %+v, %v", s, err)
	}
	if err := a.Borrar(); err != nil {
		t.Fatal(err)
	}
	if len(l.m) != 0 {
		t.Errorf("quedaron entradas en el llavero: %v", l.m)
	}
	if _, err := a.Leer(); !errors.Is(err, ErrSinSesion) {
		t.Errorf("después de borrar: %v", err)
	}
}

func TestAlmacenCaeAlArchivo(t *testing.T) {
	a := &Almacen{Dir: t.TempDir(), Llavero: &llaveroMem{m: map[string]string{}, falla: true}}
	if err := a.Guardar(sesionDePrueba()); err != nil {
		t.Fatal(err)
	}
	if a.Donde() != "archivo" {
		t.Errorf("donde = %q", a.Donde())
	}
	if s, err := a.Leer(); err != nil || s.RefreshToken != "rt" {
		t.Fatalf("%+v %v", s, err)
	}
}

func TestAlmacenLlaveroPerdido(t *testing.T) {
	dir := t.TempDir()
	l := &llaveroMem{m: map[string]string{}}
	if err := (&Almacen{Dir: dir, Llavero: l}).Guardar(sesionDePrueba()); err != nil {
		t.Fatal(err)
	}
	l.m = map[string]string{} // alguien borró la entrada del llavero
	if _, err := (&Almacen{Dir: dir, Llavero: l}).Leer(); !errors.Is(err, ErrSinSesion) {
		t.Errorf("err = %v", err)
	}
}
