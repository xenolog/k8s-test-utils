package jsonpatch

import (
	"errors"

	evanJP "github.com/evanphx/json-patch/v5"
)

var (
	ErrTestFailed   = evanJP.ErrTestFailed
	ErrMissing      = evanJP.ErrMissing
	ErrUnknownType  = evanJP.ErrUnknownType
	ErrInvalid      = evanJP.ErrInvalid
	ErrInvalidIndex = evanJP.ErrInvalidIndex

	ErrBadJSONDoc  = errors.New("invalid JSON document")
	ErrMarshalJSON = errors.New("JSON Marshal error")
)
