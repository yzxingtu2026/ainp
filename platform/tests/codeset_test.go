package tests

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

func TestCodeSetRegistryValidationAndVersion(t *testing.T) {
	r := codeset.NewRegistry()
	if err := r.Register(codeset.ActivationStatus()); err != nil {
		t.Fatal(err)
	}
	if r.Version() == "" {
		t.Fatal("registry version must not be empty")
	}
	set, ok := r.Get(codeset.KeyActivationStatus)
	if !ok {
		t.Fatal("activation codeset not found")
	}
	if set.Items[0].Value != string(status.ActivationEnabled) {
		t.Fatalf("unexpected first value: %s", set.Items[0].Value)
	}
	if err := r.Register(codeset.ActivationStatus()); err == nil {
		t.Fatal("duplicate key must fail")
	}
	invalid := codeset.CodeSet{Key: "bad", NameKey: "bad.name", Items: []codeset.CodeItem{{Value: "Not Valid", LabelKey: "bad.value", Semantic: codeset.Semantic("purple")}}}
	if err := codeset.Validate(invalid); err == nil {
		t.Fatal("invalid key/value/semantic must fail")
	}
}

func TestCodeSetI18nIsComplete(t *testing.T) {
	if err := i18n.LoadDir("../i18n"); err != nil {
		t.Fatal(err)
	}
	set := codeset.ActivationStatus()
	for _, lang := range i18n.AvailableLangs() {
		if got := i18n.T(lang, set.NameKey); got == set.NameKey {
			t.Fatalf("missing %s in %s", set.NameKey, lang)
		}
		for _, item := range set.Items {
			if got := i18n.T(lang, item.LabelKey); got == item.LabelKey {
				t.Fatalf("missing %s in %s", item.LabelKey, lang)
			}
		}
	}
}

func TestActivationStatus(t *testing.T) {
	if !status.ActivationEnabled.Valid() || !status.ActivationDisabled.Valid() {
		t.Fatal("known activation statuses must be valid")
	}
	if status.ActivationStatus("pending").Valid() {
		t.Fatal("unknown activation status must be invalid")
	}
}
