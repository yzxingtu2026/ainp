package catalog

type orderCreateRequest struct {
	Reference string `json:"reference" validate:"required,min=1,max=64"`
}

type orderBatchCreateRequest struct {
	Items []orderCreateRequest `json:"items" validate:"required,min=1,max=50,dive"`
}

type orderCreateResponse struct {
	Reference string `json:"reference"`
}

type orderBatchCreateResponse struct {
	CreatedCount int                   `json:"created_count"`
	Items        []orderCreateResponse `json:"items"`
}
