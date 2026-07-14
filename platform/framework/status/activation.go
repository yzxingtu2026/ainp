package status

// ActivationStatus is shared only by models whose lifecycle is exactly enabled/disabled.
type ActivationStatus string

const (
	ActivationEnabled  ActivationStatus = "enabled"
	ActivationDisabled ActivationStatus = "disabled"
)

func (s ActivationStatus) Valid() bool {
	return s == ActivationEnabled || s == ActivationDisabled
}
