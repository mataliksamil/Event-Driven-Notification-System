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

type ErrNotFound struct {
	Resource string
	ID       string
}

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%s not found: %s", e.Resource, e.ID)
}

type ErrNotCancellable struct {
	ID     string
	Status string
}

func (e *ErrNotCancellable) Error() string {
	return fmt.Sprintf("notification %s cannot be cancelled: current status is %s", e.ID, e.Status)
}