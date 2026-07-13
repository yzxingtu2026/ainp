package auth

import "context"

type contextKey struct{}

type Context struct {
	RequestID         string
	TraceID           string
	TenantID          string
	UserID            string
	OrgID             string
	DeptID            string
	Roles             []string
	Permissions       []string
	SessionID         string
	Client            string
	IsSystemOperator  bool
	Authenticated     bool
}

func WithContext(ctx context.Context, ac *Context) context.Context {
	return context.WithValue(ctx, contextKey{}, ac)
}

func FromContext(ctx context.Context) (*Context, bool) {
	ac, ok := ctx.Value(contextKey{}).(*Context)
	return ac, ok
}

func MustFromContext(ctx context.Context) *Context {
	ac, ok := FromContext(ctx)
	if !ok || ac == nil {
		return &Context{}
	}
	return ac
}

func (c *Context) HasPermission(code string) bool {
	if c == nil {
		return false
	}
	for _, p := range c.Permissions {
		if p == code || p == "*" {
			return true
		}
	}
	return false
}
