package attachment

import "context"

// AvatarValidator validates an attachment selected as an agent avatar.
type AvatarValidator interface {
	ValidateAvatar(ctx context.Context, attachmentID, userID string, canManage bool) error
}
