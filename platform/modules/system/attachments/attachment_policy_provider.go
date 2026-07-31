package attachments

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

const SettingKeyAttachmentUploadPolicy = "attachment.upload_policy"

// settingUploadPolicyProvider resolves UploadPolicy from tenant settings (#13).
type settingUploadPolicyProvider struct {
	provider *setting.Provider
}

func NewSettingUploadPolicyProvider(p *setting.Provider) attachment.PolicyProvider {
	return &settingUploadPolicyProvider{provider: p}
}

type uploadPolicyValues struct {
	MaxSizeMB          float64  `json:"max_size_mb"`
	AllowedExt         []string `json:"allowed_ext"`
	AllowedMIME        []string `json:"allowed_mime"`
	TempRetainHours    float64  `json:"temp_retain_hours"`
	TransferTTLMinutes float64  `json:"transfer_ttl_minutes"`
	MaxUploadsOnce     float64  `json:"max_uploads_once"`
}

func (p *settingUploadPolicyProvider) UploadPolicy(ctx context.Context, tenantID string) (attachment.UploadPolicy, error) {
	base := attachment.DefaultUploadPolicy()
	if p == nil || p.provider == nil || tenantID == "" {
		return base, nil
	}
	eff, err := p.provider.Get(ctx, tenantID, SettingKeyAttachmentUploadPolicy)
	if err != nil {
		// 设置缺失时回落代码默认，不阻断上传
		return base, nil
	}
	raw, err := json.Marshal(eff.Values)
	if err != nil {
		return base, nil
	}
	var vals uploadPolicyValues
	if err := json.Unmarshal(raw, &vals); err != nil {
		return base, nil
	}
	pol := base
	if vals.MaxSizeMB > 0 {
		pol.MaxSizeBytes = int64(vals.MaxSizeMB * 1024 * 1024)
	}
	if len(vals.AllowedExt) > 0 {
		pol.AllowedExt = normalizeExtList(vals.AllowedExt)
	}
	if len(vals.AllowedMIME) > 0 {
		pol.AllowedMIME = normalizeStringList(vals.AllowedMIME)
	}
	if vals.TempRetainHours > 0 {
		pol.TempRetain = time.Duration(vals.TempRetainHours) * time.Hour
	}
	if vals.TransferTTLMinutes > 0 {
		pol.TransferTTL = time.Duration(vals.TransferTTLMinutes) * time.Minute
	}
	if vals.MaxUploadsOnce > 0 {
		pol.MaxUploadsOnce = int(vals.MaxUploadsOnce)
	}
	return pol, nil
}

func normalizeExtList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if !strings.HasPrefix(item, ".") {
			item = "." + item
		}
		out = append(out, item)
	}
	return out
}

func normalizeStringList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// UploadPolicyView is the API/DTO shape for frontend precheck.
type UploadPolicyView struct {
	MaxSizeBytes   int64    `json:"max_size_bytes"`
	AllowedExt     []string `json:"allowed_ext"`
	AllowedMIME    []string `json:"allowed_mime"`
	TempRetainSec  int64    `json:"temp_retain_sec"`
	TransferTTLSec int64    `json:"transfer_ttl_sec"`
	MaxUploadsOnce int      `json:"max_uploads_once"`
}

func toUploadPolicyView(pol attachment.UploadPolicy) UploadPolicyView {
	return UploadPolicyView{
		MaxSizeBytes:   pol.MaxSizeBytes,
		AllowedExt:     append([]string(nil), pol.AllowedExt...),
		AllowedMIME:    append([]string(nil), pol.AllowedMIME...),
		TempRetainSec:  int64(pol.TempRetain / time.Second),
		TransferTTLSec: int64(pol.TransferTTL / time.Second),
		MaxUploadsOnce: pol.MaxUploadsOnce,
	}
}
