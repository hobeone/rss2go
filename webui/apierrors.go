package webui

import (
	"net/http"

	"github.com/labstack/echo"
)

type apiError struct {
	Title  string            `json:"title"`
	Detail string            `json:"detail,omitempty"`
	Source map[string]string `json:"source,omitempty"`
	Status int               `json:"status,omitempty"`
}

type apiErrors struct {
	Errors []*apiError `json:"errors"`
}

// Error creates a new apiErrors struct, adds an error and marshals it back to
// the client.
func newError(c echo.Context, title string, err error) error {
	a := &apiErrors{}
	a.addError(title, err, http.StatusUnprocessableEntity)
	return c.JSON(http.StatusUnprocessableEntity, a)
}

func (e *apiErrors) addError(title string, err error, status int) {
	e.Errors = append(e.Errors, &apiError{
		Title:  title,
		Detail: err.Error(),
		Source: map[string]string{
			"pointer": "data",
		},
		Status: status,
	})
}
