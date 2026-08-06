package downstream

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/host"
)

func TestOneModuleDeclarationBuildsAPIAndWorkerCompositions(t *testing.T) {
	modules := []host.Module{Module{}}
	_, err := host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	_, err = host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
}
