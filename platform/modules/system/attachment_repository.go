package system

import (
	"context"
	"sync"

	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

var attachmentModelRegistration sync.Once

func registerAttachmentTenantModels() {
	attachmentModelRegistration.Do(func() {
		if err := tenant.DefaultRegistry.Register(model.Attachment{}, tenant.ModelOptions{
			Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true, SoftDelete: true,
		}); err != nil {
			panic(err)
		}
		if err := tenant.DefaultRegistry.Register(model.AttachmentLink{}, tenant.ModelOptions{
			Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true,
		}); err != nil {
			panic(err)
		}
	})
}

type attachmentRepositories struct {
	db          tenant.TenantDatabase
	attachments *tenant.Repository[model.Attachment]
	links       *tenant.Repository[model.AttachmentLink]
}

func newAttachmentRepositories(ctx context.Context, db *gorm.DB) (*attachmentRepositories, error) {
	registerAttachmentTenantModels()
	tdb, err := tenant.TenantDB(ctx, db)
	if err != nil {
		return nil, err
	}
	attachments, _ := tenant.NewTenantRepository[model.Attachment](tdb)
	links, _ := tenant.NewTenantRepository[model.AttachmentLink](tdb)
	return &attachmentRepositories{db: tdb, attachments: attachments, links: links}, nil
}

func (r *attachmentRepositories) transaction(fn func(*attachmentRepositories) error) error {
	return r.db.Transaction(func(tx tenant.TenantDatabase) error {
		attachments, _ := tenant.NewTenantRepository[model.Attachment](tx)
		links, _ := tenant.NewTenantRepository[model.AttachmentLink](tx)
		return fn(&attachmentRepositories{db: tx, attachments: attachments, links: links})
	})
}
