package aimodel

import (
	"context"
	"fmt"
	"os"
	"regexp"
)

type SecretResolver interface {
	Resolve(context.Context, string) (string, error)
}
type EnvSecretResolver struct{}

var envRefPattern = regexp.MustCompile(`^env:([A-Z][A-Z0-9_]{1,127})$`)

func (EnvSecretResolver) Resolve(_ context.Context, ref string) (string, error) {
	m := envRefPattern.FindStringSubmatch(ref)
	if m == nil {
		return "", fmt.Errorf("unsupported credential reference")
	}
	value, ok := os.LookupEnv(m[1])
	if !ok || value == "" {
		return "", fmt.Errorf("credential reference is unavailable")
	}
	return value, nil
}
