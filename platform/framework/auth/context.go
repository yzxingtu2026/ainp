package auth

import "context"

type Realm string

const (
	RealmTenant   Realm = "tenant"
	RealmPlatform Realm = "platform"
)

func (r Realm) Valid() bool { return r == RealmTenant || r == RealmPlatform }

func NormalizeRealm(r Realm) Realm {
	if r == "" {
		return RealmTenant
	}
	return r
}

type contextKey struct{}

type Context struct {
	RequestID         string
	TraceID           string
	TenantID          string
	Realm             Realm
	UserID            string
	OrgID             string
	DeptID            string
	Roles             []string
	Permissions       []string
	SessionID         string
	AuthVersion       uint64
	Client            string
	Language          string // 由 Accept-Language 中间件协商；空表示未设置
	IsSystemOperator  bool
	Authenticated     bool
	IdempotencyKey    string // 从 Idempotency-Key 请求头提取
	AuditResourceType string
	AuditResourceID   string
	AuditSummary      string
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
