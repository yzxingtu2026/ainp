package aimodelruntime

func validateAdapterImages(in AdapterRequest, maxBytes int64) error {
	for _, message := range in.Request.Messages {
		for _, part := range message.Content {
			if part.Type != PartInputImage {
				continue
			}
			if part.ResolvedImage == nil || len(part.ResolvedImage.Data) == 0 || int64(len(part.ResolvedImage.Data)) != part.ResolvedImage.Size {
				return &RuntimeError{Category: ErrorInvalidRequest, Message: "image attachment was not resolved"}
			}
			if part.ResolvedImage.Size > maxBytes {
				return &RuntimeError{Category: ErrorInvalidRequest, Message: "image exceeds provider size limit"}
			}
		}
	}
	return nil
}
