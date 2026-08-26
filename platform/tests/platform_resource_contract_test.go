package tests

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
)

func TestPlatformResourceDTOsDoNotExposePersistenceOrSecrets(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(platformresource.AttachmentMetadata{}),
		reflect.TypeOf(platformresource.AttachmentBinding{}),
		reflect.TypeOf(platformresource.OwnedUploadOpenRequest{}),
		reflect.TypeOf(platformresource.PrincipalProjection{}),
		reflect.TypeOf(platformresource.AccountIdentity{}),
		reflect.TypeOf(platformresource.AccountIdentityBindRequest{}),
		reflect.TypeOf(platformresource.AccountIdentityUnbindRequest{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name := strings.ToLower(field.Name)
			if strings.Contains(name, "key") || strings.Contains(name, "token") || strings.Contains(name, "gorm") || field.Type.String() == "*gorm.DB" {
				t.Fatalf("public platform resource DTO %s exposes forbidden field %s", typ.Name(), field.Name)
			}
		}
	}
}

func TestAccountIdentityBindingContractRejectsUnsafeInputs(t *testing.T) {
	valid := platformresource.AccountIdentityBindRequest{Provider: "wecom-oauth", Subject: "corp:user-1", NormalizedIdentifier: "user@example.com"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid binding request rejected: %v", err)
	}
	for name, request := range map[string]platformresource.AccountIdentityBindRequest{
		"empty provider":         {Subject: "subject", NormalizedIdentifier: "identifier"},
		"non canonical provider": {Provider: "WeCom", Subject: "subject", NormalizedIdentifier: "identifier"},
		"password provider":      {Provider: "password", Subject: "subject", NormalizedIdentifier: "identifier"},
		"trimmed subject":        {Provider: "wecom", Subject: " subject", NormalizedIdentifier: "identifier"},
		"control subject":        {Provider: "wecom", Subject: "subject\n", NormalizedIdentifier: "identifier"},
		"empty identifier":       {Provider: "wecom", Subject: "subject"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := request.Validate(); !errors.Is(err, platformresource.ErrInvalid) {
				t.Fatalf("Validate() error = %v", err)
			}
		})
	}
	for _, request := range []platformresource.AccountIdentityUnbindRequest{{}, {IdentityID: " identity-1"}, {IdentityID: "identity\n"}} {
		if err := request.Validate(); !errors.Is(err, platformresource.ErrInvalid) {
			t.Fatalf("unsafe unbind request accepted: %#v", request)
		}
	}
	identityType := reflect.TypeOf(platformresource.AccountIdentity{})
	for _, forbidden := range []string{"TenantID", "AccountID", "Metadata", "Token", "Secret"} {
		if _, ok := identityType.FieldByName(forbidden); ok {
			t.Fatalf("account identity projection exposes %s", forbidden)
		}
	}
}

func TestPlatformResourceServicesFailClosedWhenUnconfigured(t *testing.T) {
	if err := (platformresource.Services{}).Validate(); err == nil {
		t.Fatal("empty platform resource services must fail closed")
	}
}
