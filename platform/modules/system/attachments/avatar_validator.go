package attachments

import (
	"context"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments/model"
	"gorm.io/gorm"
)

type AvatarValidator struct{ db *gorm.DB }

func NewAvatarValidator(db *gorm.DB) *AvatarValidator { return &AvatarValidator{db: db} }

func (v *AvatarValidator) ValidateAvatar(ctx context.Context, id, userID string, canManage bool) error {
	tdb, err := tenantdata.TenantDB(ctx, v.db)
	if err != nil {
		return err
	}
	repo, err := tenantdata.NewTenantRepository[model.Attachment](tdb)
	if err != nil {
		return err
	}
	row, err := repo.First("id=? AND status=?", id, "ready")
	if err != nil || (!canManage && row.UploaderID != userID) {
		return errs.New(errs.CodeNotFound, "avatar attachment not found")
	}
	if !strings.HasPrefix(row.ContentType, "image/") {
		return errs.New(errs.CodeValidation, "avatar attachment must be an image")
	}
	return nil
}
