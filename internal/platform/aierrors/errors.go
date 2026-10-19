// Package aierrors identifies failures at the component where they originate.
package aierrors

import "fmt"

type Kind string

const (
	Application   Kind = "application_internal"
	CatalogBudget Kind = "catalog_budget_exceeded"
	CatalogData   Kind = "catalog_data_error"
	ModelOutput   Kind = "invalid_ai_output"
)

type Failure struct {
	Kind  Kind
	Cause error
}

func (e *Failure) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%s: %v", e.Kind, e.Cause)
}

func (e *Failure) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func Wrap(kind Kind, err error) error {
	if err == nil {
		return nil
	}
	return &Failure{Kind: kind, Cause: err}
}
