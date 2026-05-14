package sqltype

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
)

var (
	_ sql.Scanner   = (*JSON[struct{}])(nil)
	_ driver.Valuer = (*JSON[struct{}])(nil)
)

var (
	ErrInvalidType = errors.New("invalid type")
)

type JSON[T any] struct {
	value T
	raw   json.RawMessage
}

func NewJSON[T any](v T) *JSON[T] {
	return &JSON[T]{value: v}
}

func (j *JSON[T]) Unwrap() T {
	return j.value
}

func (j *JSON[T]) MarshalJSON() ([]byte, error) {
	if j.raw != nil {
		return j.raw, nil
	}
	return []byte("{}"), nil
}

func (j *JSON[T]) Value() (driver.Value, error) {
	if j.raw != nil {
		return []byte(j.raw), nil
	}
	return json.Marshal(j.value)
}

func (j *JSON[T]) Scan(src any) error {
	switch src := src.(type) {
	case nil:
		return nil
	case []byte:
		if len(src) == 0 || string(src) == "null" {
			return nil
		}
		j.raw = src
		return json.Unmarshal(src, &j.value)
	case string:
		if len(src) == 0 || src == "null" {
			return nil
		}
		j.raw = json.RawMessage(src)
		return json.Unmarshal([]byte(src), &j.value)
	default:
		return ErrInvalidType
	}
}
