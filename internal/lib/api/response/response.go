package response

import (
	"fmt"
	"net/http"

	"github.com/go-chi/render"
	"github.com/go-playground/validator/v10"
)

type Response struct {
	ResultCode int      `json:"resultCode"`
	Messages   []string `json:"messages,omitempty"`
}

const (
	StatusOK    = 0
	StatusError = 1
)

func OK() Response {
	return Response{
		ResultCode: StatusOK,
	}
}

func Error(code int, msgs []string) Response {
	if code == 0 {
		code = StatusError
	}
	return Response{
		ResultCode: code,
		Messages:   msgs,
	}
}

func ValidationError(errs validator.ValidationErrors) Response {
	var errMsgs []string

	for _, err := range errs {
		switch err.ActualTag() {
		case "required":
			errMsgs = append(errMsgs, fmt.Sprintf("field %s is a required field", err.Field()))
		case "url":
			errMsgs = append(errMsgs, fmt.Sprintf("field %s is not a valid URL", err.Field()))
		default:
			errMsgs = append(errMsgs, fmt.Sprintf("field %s is not valid", err.Field()))
		}
	}

	return Response{
		ResultCode: StatusError,
		Messages:   errMsgs,
	}
}

type errorResponse struct {
	Response
}

func HttpResponseError(w http.ResponseWriter, r *http.Request, statusCode int, code int, msgs []string) {
	render.Status(r, statusCode)
	render.JSON(w, r, errorResponse{
		Response: Error(code, msgs),
	})
}
