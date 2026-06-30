package dreamina

import (
	"errors"
	"fmt"
)

var ErrInvalidRequest = errors.New("invalid dreamina request")

type HTTPStatusError struct {
	Method     string
	Path       string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("dreamina request returned non-2xx status: method=%s path=%s status=%d", e.Method, e.Path, e.StatusCode)
}

type UpstreamError struct {
	Ret     string
	Message string
}

func (e *UpstreamError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("dreamina upstream rejected request: ret=%s", e.Ret)
	}
	return fmt.Sprintf("dreamina upstream rejected request: ret=%s message=%s", e.Ret, e.Message)
}
