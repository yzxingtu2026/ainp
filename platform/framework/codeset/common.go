package codeset

import "github.com/yzxingtu2026/ainp/platform/framework/status"

const (
	KeyActivationStatus     = "common.activation_status"
	NameActivationStatus    = "codeset.common.activation_status"
	LabelActivationEnabled  = "codeset.common.activation_status.enabled"
	LabelActivationDisabled = "codeset.common.activation_status.disabled"
)

func ActivationStatus() CodeSet {
	return CodeSet{
		Key: KeyActivationStatus, NameKey: NameActivationStatus,
		Items: []CodeItem{
			{Value: string(status.ActivationEnabled), LabelKey: LabelActivationEnabled, Semantic: SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 10},
			{Value: string(status.ActivationDisabled), LabelKey: LabelActivationDisabled, Semantic: SemanticMuted, Icon: "mdi:cancel", Order: 20},
		},
	}
}
