package attachment

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const TransferAudience = "ainp:attachment-transfer"

const (
	OpUpload   = "upload"
	OpDownload = "download"
)

const (
	TransferTransportRaw       = "raw"
	TransferTransportMultipart = "multipart"
)

// TransferClaims are short-lived transfer tokens. They must not be logged in full.
type TransferClaims struct {
	TenantID     string `json:"tid"`
	PrincipalID  string `json:"principal_id,omitempty"`
	LegacyUserID string `json:"uid,omitempty"`
	AttachmentID string `json:"aid"`
	Op           string `json:"op"`
	Variant      string `json:"var,omitempty"` // original | thumb | preview；空 = original
	Transport    string `json:"transport,omitempty"`
	Size         int64  `json:"size,omitempty"`
	JTI          string `json:"jti"`
	jwt.RegisteredClaims
}

// TransferTokenService issues and parses attachment transfer JWTs.
type TransferTokenService struct {
	secret []byte
	issuer string
}

func NewTransferTokenService(secret, issuer string) (*TransferTokenService, error) {
	if secret == "" {
		return nil, fmt.Errorf("jwt secret is required")
	}
	if issuer == "" {
		issuer = "ainp"
	}
	return &TransferTokenService{secret: []byte(secret), issuer: issuer}, nil
}

type IssueTransferInput struct {
	TenantID     string
	PrincipalID  string
	AttachmentID string
	Op           string
	Variant      string // original | thumb | preview；空 = original
	Transport    string // raw | multipart；仅 upload 使用
	Size         int64
	TTL          time.Duration
}

func (s *TransferTokenService) Issue(in IssueTransferInput) (token string, jti string, exp time.Time, err error) {
	if in.TenantID == "" || in.PrincipalID == "" || in.AttachmentID == "" {
		return "", "", time.Time{}, fmt.Errorf("transfer token identity required")
	}
	if in.Op != OpUpload && in.Op != OpDownload {
		return "", "", time.Time{}, fmt.Errorf("invalid transfer op")
	}
	transport := strings.TrimSpace(strings.ToLower(in.Transport))
	if in.Op == OpUpload {
		if transport == "" {
			transport = TransferTransportRaw
		}
		switch transport {
		case TransferTransportRaw, TransferTransportMultipart:
		default:
			return "", "", time.Time{}, fmt.Errorf("invalid transfer transport")
		}
	} else if transport != "" {
		return "", "", time.Time{}, fmt.Errorf("download transfer transport is not supported")
	}
	variant := strings.TrimSpace(strings.ToLower(in.Variant))
	if variant == "" {
		variant = "original"
	}
	switch variant {
	case "original", "thumb", "preview":
	default:
		return "", "", time.Time{}, fmt.Errorf("invalid transfer variant")
	}
	if in.Op == OpDownload && in.Size <= 0 {
		return "", "", time.Time{}, fmt.Errorf("invalid download transfer size")
	}
	if in.TTL <= 0 {
		in.TTL = 15 * time.Minute
	}
	now := time.Now().UTC()
	exp = now.Add(in.TTL)
	jti = uuid.NewString()
	claims := TransferClaims{
		TenantID: in.TenantID, PrincipalID: in.PrincipalID, LegacyUserID: in.PrincipalID, AttachmentID: in.AttachmentID,
		Op: in.Op, Variant: variant, Transport: transport, Size: in.Size, JTI: jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   in.PrincipalID,
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			Audience:  jwt.ClaimStrings{TransferAudience},
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.secret)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return signed, jti, exp, nil
}

func (s *TransferTokenService) Parse(tokenString string) (*TransferClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TransferClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secret, nil
	}, jwt.WithIssuer(s.issuer), jwt.WithAudience(TransferAudience))
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*TransferClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid transfer token")
	}
	if claims.Subject != "" && claims.PrincipalID != "" && claims.Subject != claims.PrincipalID {
		return nil, fmt.Errorf("conflicting transfer token subject")
	}
	if claims.PrincipalID != "" && claims.LegacyUserID != "" && claims.PrincipalID != claims.LegacyUserID {
		return nil, fmt.Errorf("conflicting transfer token principal")
	}
	if claims.PrincipalID == "" {
		claims.PrincipalID = claims.Subject
	}
	if claims.PrincipalID == "" {
		claims.PrincipalID = claims.LegacyUserID
	}
	if claims.TenantID == "" || claims.PrincipalID == "" || claims.AttachmentID == "" || claims.JTI == "" {
		return nil, fmt.Errorf("incomplete transfer claims")
	}
	if claims.Op != OpUpload && claims.Op != OpDownload {
		return nil, fmt.Errorf("invalid transfer op")
	}
	claims.Transport = strings.TrimSpace(strings.ToLower(claims.Transport))
	if claims.Op == OpUpload {
		switch claims.Transport {
		case "", TransferTransportRaw, TransferTransportMultipart:
		default:
			return nil, fmt.Errorf("invalid transfer transport")
		}
	} else if claims.Transport != "" {
		return nil, fmt.Errorf("invalid download transfer transport")
	}
	if claims.Variant == "" {
		claims.Variant = "original"
	}
	switch claims.Variant {
	case "original", "thumb", "preview":
	default:
		return nil, fmt.Errorf("invalid transfer variant")
	}
	if claims.Op == OpDownload && claims.Size <= 0 {
		return nil, fmt.Errorf("invalid download transfer size")
	}
	return claims, nil
}

// EffectiveUploadTransport keeps legacy upload tokens without a transport claim raw-only.
func (c *TransferClaims) EffectiveUploadTransport() string {
	if c == nil || c.Transport == "" {
		return TransferTransportRaw
	}
	return c.Transport
}
