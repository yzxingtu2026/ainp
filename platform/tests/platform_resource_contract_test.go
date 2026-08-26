package tests

import (
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

func TestPlatformResourceServicesFailClosedWhenUnconfigured(t *testing.T) {
	if err := (platformresource.Services{}).Validate(); err == nil {
		t.Fatal("empty platform resource services must fail closed")
	}
}
