package webui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

func TestErrors(t *testing.T) {
	ers := apiErrors{}

	n := &apiError{
		Title:  "Error Title",
		Detail: "Error detail",
		Status: http.StatusInternalServerError,
	}

	ers.Errors = append(ers.Errors, n)
	spew.Dump(ers)

	b, err := json.MarshalIndent(ers, "", "  ")
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Print(string(b))
}
