package action

type IDRequest struct {
	ID string `json:"id" validate:"required"`
}
