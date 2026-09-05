package system

import (
	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/aiknowledge"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments"
	"gorm.io/gorm"
)

type (
	AttachmentService               = attachments.AttachmentService
	AttachmentView                  = attachments.AttachmentView
	TransferPrepareView             = attachments.TransferPrepareView
	AttachmentLinkView              = attachments.AttachmentLinkView
	AttachmentDetailView            = attachments.AttachmentDetailView
	AttachmentDetailToolView        = attachments.AttachmentDetailToolView
	AttachmentContentInspectRequest = attachments.AttachmentContentInspectRequest
	AttachmentContentInspectResult  = attachments.AttachmentContentInspectResult
	AttachmentContentPageStatus     = attachments.AttachmentContentPageStatus
	AttachmentContentObservation    = attachments.AttachmentContentObservation
	UploadPolicyView                = attachments.UploadPolicyView
	AttachmentResourceReader        = attachments.ResourceReader
	KnowledgeSourceResolver         = attachments.KnowledgeSourceResolver
)

const (
	InspectStatusReady          = attachments.InspectStatusReady
	InspectStatusEmpty          = attachments.InspectStatusEmpty
	InspectStatusLowQuality     = attachments.InspectStatusLowQuality
	InspectStatusPartialFailure = attachments.InspectStatusPartialFailure
	InspectStatusFailed         = attachments.InspectStatusFailed
)

func NewSettingUploadPolicyProvider(p *setting.Provider) attachment.PolicyProvider {
	return attachments.NewSettingUploadPolicyProvider(p)
}
func NewAttachmentService(db *gorm.DB, store storage.ObjectStore, tokens *attachment.TransferTokenService, policies *attachment.PolicyRegistry, provider attachment.PolicyProvider) *AttachmentService {
	return attachments.NewAttachmentService(db, store, tokens, policies, provider)
}

func AttachmentResources(svc *AttachmentService) platformresource.AttachmentReader {
	return svc.ResourceReader()
}
func NewKnowledgeSourceResolver(db *gorm.DB, store storage.ObjectStore, knowledge aiknowledge.Service) *KnowledgeSourceResolver {
	return attachments.NewKnowledgeSourceResolver(db, store, knowledge)
}
func RegisterAttachmentTransferRoutes(r gin.IRoutes, svc *AttachmentService) {
	attachments.RegisterAttachmentTransferRoutes(r, svc)
}
func ContentDisposition(filename string) string { return attachments.ContentDisposition(filename) }
func registerAttachmentActions(a *app.App, svc *AttachmentService) error {
	return attachments.RegisterActions(a, svc)
}
