package attachment

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const TransferAudience = "ainp:attachment-transfer"

const (
	OpUpload   = "upload"
	OpDownload = "download"
)

// TransferClaims are short-lived transfer tokens. They must not be logged in full.
type TransferClaims struct {
	TenantID     string `json:"tid"`
	UserID       string `json:"uid"`
	AttachmentID string `json:"aid"`
	Op           string `json:"op"`
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
	UserID       string
	AttachmentID string
	Op           string
	Size         int64
	TTL          time.Duration
}

func (s *TransferTokenService) Issue(in IssueTransferInput) (token string, jti string, exp time.Time, err error) {
	if in.TenantID == "" || in.UserID == "" || in.AttachmentID == "" {
		return "", "", time.Time{}, fmt.Errorf("transfer token identity required")
	}
	if in.Op != OpUpload && in.Op != OpDownload {
		return "", "", time.Time{}, fmt.Errorf("invalid transfer op")
	}
	if in.TTL <= 0 {
		in.TTL = 15 * time.Minute
	}
	now := time.Now().UTC()
	exp = now.Add(in.TTL)
	jti = uuid.NewString()
	claims := TransferClaims{
		TenantID: in.TenantID, UserID: in.UserID, AttachmentID: in.AttachmentID,
		Op: in.Op, Size: in.Size, JTI: jti,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   in.UserID,
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
	if claims.TenantID == "" || claims.UserID == "" || claims.AttachmentID == "" || claims.JTI == "" {
		return nil, fmt.Errorf("incomplete transfer claims")
	}
	return claims, nil
}
