package auth

import (
	"fmt"
	"strings"
)

const CSRFHeader = "X-CSRF-Token"

func RefreshCookieName(surfaceID string) string {
	var builder strings.Builder
	builder.WriteString("ainp_refresh_")
	for _, char := range strings.ToLower(surfaceID) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			continue
		}
		_, _ = fmt.Fprintf(&builder, "_%02x", char)
	}
	return builder.String()
}

func RefreshCookiePath(surface ClientSurface) string {
	return strings.TrimSuffix(surface.PathPrefix, "/") + "/"
}
