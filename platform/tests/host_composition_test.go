package tests

import (
	"fmt"
	"testing"
	"testing/fstest"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/host"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

type testHostModule struct {
	id       string
	events   *[]string
	prepare  bool
	source   *migrations.Source
	register func(*app.App) error
}

func (m testHostModule) ID() string { return m.id }
func (m testHostModule) Register(application *app.App) error {
	if m.events != nil {
		*m.events = append(*m.events, "register:"+m.id)
	}
	if m.register != nil {
		return m.register(application)
	}
	return nil
}
func (m testHostModule) Prepare(*app.App) error {
	if m.prepare && m.events != nil {
		*m.events = append(*m.events, "prepare:"+m.id)
	}
	return nil
}
func (m testHostModule) MigrationSource() migrations.Source {
	if m.source == nil {
		return migrations.Source{}
	}
	return *m.source
}

func TestHostCompositionRejectsInvalidAndDuplicateModuleIDs(t *testing.T) {
	for _, test := range []struct {
		name    string
		modules []host.Module
	}{
		{name: "invalid", modules: []host.Module{testHostModule{id: "Bad ID"}}},
		{name: "duplicate", modules: []host.Module{testHostModule{id: "orders"}, testHostModule{id: "orders"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := host.New(host.WithModules(test.modules...)); err == nil {
				t.Fatal("expected fail-closed module validation")
			}
		})
	}
}

func TestHostCompositionRejectsMigrationOwnerDifferentFromModule(t *testing.T) {
	source := migrations.Source{Owner: "other", FS: fstest.MapFS{"0001_init.sql": {Data: []byte("SELECT 1;")}}}
	_, err := host.New(host.WithModules(testHostModule{id: "orders", source: &source}))
	if err == nil {
		t.Fatal("expected migration owner mismatch")
	}
	if got := fmt.Sprint(err); got != `host: module "orders" migration owner must match module id` {
		t.Fatalf("unexpected error: %s", got)
	}
}
