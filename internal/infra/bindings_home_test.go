package infra

import (
	"os"
	"strings"
	"testing"

	"github.com/GabiHert/phonon/internal/infra/paths"
)

func TestIsolateHomeOffLeavesTheDataFolder(t *testing.T) {
	t.Setenv(paths.HomeEnv, "")
	done, err := isolateHome(false)
	if err != nil {
		t.Fatal(err)
	}
	done()
	if v := os.Getenv(paths.HomeEnv); v != "" {
		t.Fatalf("%s = %q, want unset", paths.HomeEnv, v)
	}
}

func TestIsolateHomeOnUsesAThrowawayFolder(t *testing.T) {
	t.Setenv(paths.HomeEnv, "")
	home, _ := os.UserHomeDir()
	done, err := isolateHome(true)
	if err != nil {
		t.Fatal(err)
	}
	dir := paths.Home()
	if dir == "" || strings.HasPrefix(dir, home+string(os.PathSeparator)+".") {
		t.Fatalf("Home() = %q, want a throwaway folder", dir)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("throwaway folder: %v", err)
	}
	done()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("throwaway folder still there: %v", err)
	}
}

// The binary wails build runs is built with the bindings tag; every other build is not.
func TestGeneratingBindingsIsOffInNormalBuilds(t *testing.T) {
	if generatingBindings {
		t.Fatal("generatingBindings is true without the bindings tag")
	}
}
