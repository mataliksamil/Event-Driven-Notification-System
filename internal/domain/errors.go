package domain

import "fmt"

type ErrDuplicateBatch struct {
	BatchID string
}

func (e *ErrDuplicateBatch) Error() string {
	return fmt.Sprintf("duplicate batch: %s", e.BatchID)
}

type ErrValidation struct {
	Field   string
	Message string
}

func (e *ErrValidation) Error() string {
	return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}