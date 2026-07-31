package demo

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	demoruntime "github.com/yzxingtu2026/ainp/platform/modules/demo/showcase"
	"gorm.io/gorm"
)

func NewRevisionDocService(db *gorm.DB) *RevisionDocService {
	return demoruntime.NewRevisionDocService(db)
}

func RegisterRevisionDocActions(a *app.App, svc *RevisionDocService) error {
	return demoruntime.RegisterRevisionDocActions(a, svc)
}
