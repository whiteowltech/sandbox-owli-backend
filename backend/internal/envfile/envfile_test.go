package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comentario\nOWLI_T_A=uno\nOWLI_T_B=\"dos\"\nOWLI_T_C=no-pisa\nlinea-sin-igual\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OWLI_T_C", "del-entorno")
	if err := Load(path); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("OWLI_T_A") != "uno" || os.Getenv("OWLI_T_B") != "dos" {
		t.Errorf("A=%q B=%q", os.Getenv("OWLI_T_A"), os.Getenv("OWLI_T_B"))
	}
	if os.Getenv("OWLI_T_C") != "del-entorno" {
		t.Error("una variable ya definida no se pisa")
	}
	os.Unsetenv("OWLI_T_A")
	os.Unsetenv("OWLI_T_B")
	if err := Load(filepath.Join(t.TempDir(), "no-existe")); err != nil {
		t.Errorf("sin archivo no es error: %v", err)
	}
}
