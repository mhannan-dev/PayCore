package handler

import (
	"encoding/json"
	"errors"
	"io"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

var errTrailingJSON = errors.New("request must contain exactly one JSON value")

// bindJSON rejects unknown fields and trailing JSON documents, then applies the
// same binding-tag validator as Gin. Callers set their endpoint-specific byte limit.
func bindJSON[T any](c *gin.Context, destination *T) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errTrailingJSON
	}
	return binding.Validator.ValidateStruct(destination)
}
