package openai

import (
	"errors"
	"fmt"

	"github.com/cloudwego/eino-ext/libs/acl/openai"
	openai2 "github.com/meguminnnnnnnnn/go-openai"
)

type ReasoningEffortLevel openai.ReasoningEffortLevel

const (
	ReasoningEffortLevelLow    = ReasoningEffortLevel(openai.ReasoningEffortLevelLow)
	ReasoningEffortLevelMedium = ReasoningEffortLevel(openai.ReasoningEffortLevelMedium)
	ReasoningEffortLevelHigh   = ReasoningEffortLevel(openai.ReasoningEffortLevelHigh)
)

type APIError struct {
	Code           any     `json:"code,omitempty"`
	Message        string  `json:"message"`
	Param          *string `json:"param,omitempty"`
	Type           string  `json:"type"`
	HTTPStatus     string  `json:"-"`
	HTTPStatusCode int     `json:"-"`
}

func (e *APIError) Error() string {
	if e.HTTPStatusCode > 0 {
		return fmt.Sprintf("error, status code: %d, status: %s, message: %s", e.HTTPStatusCode, e.HTTPStatus, e.Message)
	}

	return e.Message
}

func convOrigAPIError(err error) error {
	apiErr := &openai2.APIError{}
	if errors.As(err, &apiErr) {
		return &APIError{
			Code:           apiErr.Code,
			Message:        apiErr.Message,
			Param:          apiErr.Param,
			Type:           apiErr.Type,
			HTTPStatus:     apiErr.HTTPStatus,
			HTTPStatusCode: apiErr.HTTPStatusCode,
		}
	}
	return err
}
