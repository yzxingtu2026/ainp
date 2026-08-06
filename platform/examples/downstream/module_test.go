package downstream

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/host"
)

func TestOneModuleDeclarationBuildsAPIAndWorkerCompositions(t *testing.T) {
	modules := []host.Module{Module{}}
	api, err := host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	if api.DemoEnabled() || worker.DemoEnabled() {
		t.Fatal("production downstream compositions must exclude Demo by default")
	}

	showcase, err := host.New(host.WithModules(modules...), host.WithDemo())
	if err != nil {
		t.Fatal(err)
	}
	if !showcase.DemoEnabled() {
		t.Fatal("downstream showcase must opt in to Demo explicitly")
	}
}
