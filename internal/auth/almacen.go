package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// Sesion es la sesión de Firebase. IDToken y RefreshToken nunca se imprimen.
type Sesion struct {
	IDToken      string
	RefreshToken string
	Expira       time.Time
	Email        string
	UID          string
}

// ErrSinSesion: no hay sesión guardada.
var ErrSinSesion = errors.New("no hay sesión")

// Llavero es el llavero del sistema (go-keyring). nil: sin llavero.
type Llavero interface {
	Get(servicio, cuenta string) (string, error)
	Set(servicio, cuenta, valor string) error
	Delete(servicio, cuenta string) error
}

// LlaveroSistema usa el llavero del sistema operativo (Credential Manager,
// Keychain, Secret Service).
type LlaveroSistema struct{}

func (LlaveroSistema) Get(s, c string) (string, error) { return keyring.Get(s, c) }
func (LlaveroSistema) Set(s, c, v string) error        { return keyring.Set(s, c, v) }
func (LlaveroSistema) Delete(s, c string) error        { return keyring.Delete(s, c) }

const (
	servicioLlavero = "tekitla-tk"
	cuentaRefresh   = "refresh_token"
	cuentaIDToken   = "id_token"
	archivoSesion   = "sesion.json"

	dondeLlavero = "llavero"
	dondeArchivo = "archivo"
)

// Almacen guarda la sesión: los tokens en el llavero y los datos no secretos
// en <dir>/sesion.json. Sin llavero, todo va a sesion.json con permisos 0600.
type Almacen struct {
	Dir     string
	Llavero Llavero
}

type archivoSesionJSON struct {
	Donde        string    `json:"donde"`
	Email        string    `json:"email,omitempty"`
	UID          string    `json:"uid,omitempty"`
	Expira       time.Time `json:"expira"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IDToken      string    `json:"id_token,omitempty"`
}

// Donde dice dónde quedó guardada la sesión: "llavero" o "archivo".
func (a *Almacen) Donde() string {
	m, err := a.leerArchivo()
	if err != nil {
		return ""
	}
	return m.Donde
}

// Guardar guarda la sesión; si el llavero falla, cae al archivo 0600.
func (a *Almacen) Guardar(s Sesion) error {
	m := archivoSesionJSON{Donde: dondeArchivo, Email: s.Email, UID: s.UID, Expira: s.Expira}
	if a.Llavero != nil && a.guardarEnLlavero(s) == nil {
		m.Donde = dondeLlavero
	} else {
		m.RefreshToken = s.RefreshToken
		m.IDToken = s.IDToken
	}
	return a.escribirArchivo(m)
}

func (a *Almacen) guardarEnLlavero(s Sesion) error {
	if err := a.Llavero.Set(servicioLlavero, cuentaRefresh, s.RefreshToken); err != nil {
		return err
	}
	v := strconv.FormatInt(s.Expira.Unix(), 10) + "|" + s.IDToken
	if err := a.Llavero.Set(servicioLlavero, cuentaIDToken, v); err != nil {
		_ = a.Llavero.Delete(servicioLlavero, cuentaRefresh)
		return err
	}
	return nil
}

// Leer devuelve la sesión guardada o ErrSinSesion.
func (a *Almacen) Leer() (Sesion, error) {
	m, err := a.leerArchivo()
	if err != nil {
		return Sesion{}, err
	}
	s := Sesion{Email: m.Email, UID: m.UID, Expira: m.Expira}
	switch m.Donde {
	case dondeArchivo:
		s.RefreshToken, s.IDToken = m.RefreshToken, m.IDToken
	case dondeLlavero:
		if a.Llavero == nil {
			return Sesion{}, ErrSinSesion
		}
		rt, err := a.Llavero.Get(servicioLlavero, cuentaRefresh)
		if err != nil {
			return Sesion{}, ErrSinSesion
		}
		s.RefreshToken = rt
		if v, err := a.Llavero.Get(servicioLlavero, cuentaIDToken); err == nil {
			if exp, tok, ok := strings.Cut(v, "|"); ok {
				if n, err := strconv.ParseInt(exp, 10, 64); err == nil {
					s.Expira, s.IDToken = time.Unix(n, 0), tok
				}
			}
		}
	default:
		return Sesion{}, ErrSinSesion
	}
	if s.RefreshToken == "" {
		return Sesion{}, ErrSinSesion
	}
	return s, nil
}

// Borrar elimina la sesión del llavero y del archivo.
func (a *Almacen) Borrar() error {
	if a.Llavero != nil {
		_ = a.Llavero.Delete(servicioLlavero, cuentaRefresh)
		_ = a.Llavero.Delete(servicioLlavero, cuentaIDToken)
	}
	err := os.Remove(filepath.Join(a.Dir, archivoSesion))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("no pude borrar %s: %w", archivoSesion, err)
	}
	return nil
}

func (a *Almacen) leerArchivo() (archivoSesionJSON, error) {
	var m archivoSesionJSON
	b, err := os.ReadFile(filepath.Join(a.Dir, archivoSesion))
	if errors.Is(err, fs.ErrNotExist) {
		return m, ErrSinSesion
	}
	if err != nil {
		return m, fmt.Errorf("no pude leer la sesión: %w", err)
	}
	if json.Unmarshal(b, &m) != nil {
		return m, ErrSinSesion
	}
	return m, nil
}

func (a *Almacen) escribirArchivo(m archivoSesionJSON) error {
	if err := os.MkdirAll(a.Dir, 0o700); err != nil {
		return fmt.Errorf("no pude crear %s: %w", a.Dir, err)
	}
	// Los tokens van al archivo solo sin llavero (modo archivo 0600, ADR 0005).
	b, _ := json.MarshalIndent(m, "", "  ") //nolint:gosec // G117: intencional, ver arriba
	destino := filepath.Join(a.Dir, archivoSesion)
	// CreateTemp crea el archivo con permisos 0600.
	tmp, err := os.CreateTemp(a.Dir, archivoSesion+".*")
	if err != nil {
		return fmt.Errorf("no pude guardar la sesión: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("no pude guardar la sesión: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("no pude guardar la sesión: %w", err)
	}
	if err := os.Rename(tmp.Name(), destino); err != nil {
		return fmt.Errorf("no pude guardar la sesión: %w", err)
	}
	return nil
}
