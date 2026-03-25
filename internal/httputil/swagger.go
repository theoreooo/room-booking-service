package httputil

type ErrorPayload struct {
	Code    string `json:"code" example:"INVALID_REQUEST"`
	Message string `json:"message" example:"invalid request"`
}

type ErrorResponse struct {
	Error ErrorPayload `json:"error"`
}
