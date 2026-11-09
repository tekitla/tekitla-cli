package cli

import "testing"

func TestParsearClave(t *testing.T) {
	buenas := map[string]struct {
		k string
		n int
	}{"DP-3": {"DP", 3}, "tk-42": {"TK", 42}, " R2D2-1 ": {"R2D2", 1}}
	for in, want := range buenas {
		k, n, err := parsearClave(in)
		if err != nil || k != want.k || n != want.n {
			t.Errorf("%q → %q %d %v", in, k, n, err)
		}
	}
	for _, in := range []string{"", "DP", "DP-", "DP-0", "DP--1", "D-1", "2D-1", "ABCDEFGHIJK-1", "DP-1x", "D_P-1"} {
		if _, _, err := parsearClave(in); err == nil {
			t.Errorf("%q debería fallar", in)
		}
	}
}

func TestNormalizar(t *testing.T) {
	if normalizar("  En   Revisión ") != normalizar("en revision") {
		t.Error("acentos, mayúsculas y espacios deberían dar igual")
	}
}

func TestParsearPrioridad(t *testing.T) {
	for in, want := range map[string]string{"alta": "PRIORITY_HIGH", "URGENTE": "PRIORITY_URGENT", "low": "PRIORITY_LOW", "Media": "PRIORITY_MEDIUM"} {
		if got, err := parsearPrioridad(in); err != nil || got != want {
			t.Errorf("%q → %q %v", in, got, err)
		}
	}
	if _, err := parsearPrioridad("ya"); err == nil {
		t.Error("quiero error")
	}
}
