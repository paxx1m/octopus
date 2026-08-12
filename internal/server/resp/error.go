package resp

const (
	ErrBadRequest        = "Invalid request parameters"
	ErrInvalidJSON       = "Invalid JSON format"
	ErrInvalidParam      = "Invalid parameter"
	ErrDuplicateResource = "Resource already exists"
	ErrInternalServer    = "An unexpected error occurred"
	ErrDatabase          = "Database operation failed"
	ErrUnauthorized      = "Authentication failed"
)
