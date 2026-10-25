package contracts

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type AssistTask string

const (
	TaskDraftText		AssistTask	= "draft_text"
	TaskContinueText	AssistTask	= "continue_text"
	TaskImproveText		AssistTask	= "improve_text"
	TaskSummarize		AssistTask	= "summarize"
	TaskExplain		AssistTask	= "explain"
	TaskPrepareFields	AssistTask	= "prepare_fields"
)

func validTask(t AssistTask) bool {
	switch t {
	case TaskDraftText, TaskContinueText, TaskImproveText,
		TaskSummarize, TaskExplain, TaskPrepareFields:
		return true
	default:
		return false
	}
}

// ProjectionDescriptor is the server-owned allowlist contract for one
// AI-facing read model. It is metadata only: it grants no access by itself.
type ProjectionDescriptor struct {
	ID			string
	Version			int
	SourceEntities		[]string
	AllowedFields		[]string
	RequiredPermission	string
	AllowedTasks		[]AssistTask
	ReadFacade		string
	MaxRows			int
	MaxBytes		int
	MaxAge			time.Duration
	RedactionPolicy		string
	Owner			string
}

var (
	ErrInvalidProjection	= errors.New("invalid AI projection descriptor")
	ErrUnknownProjection	= errors.New("AI projection is not registered")
	ErrDeniedProjection	= errors.New("AI projection use is not allowed")
)

func validateDescriptor(d ProjectionDescriptor) error {
	if strings.TrimSpace(d.ID) == "" || d.Version <= 0 ||
		len(d.SourceEntities) == 0 || len(d.AllowedFields) == 0 ||
		len(d.AllowedTasks) == 0 || d.MaxRows <= 0 || d.MaxBytes <= 0 ||
		d.MaxAge < 0 || strings.TrimSpace(d.RequiredPermission) == "" ||
		strings.TrimSpace(d.ReadFacade) == "" ||
		strings.TrimSpace(d.RedactionPolicy) == "" || strings.TrimSpace(d.Owner) == "" {
		return fmt.Errorf("%w: required contract metadata is missing", ErrInvalidProjection)
	}
	seenSources := make(map[string]struct{}, len(d.SourceEntities))
	for _, entity := range d.SourceEntities {
		if strings.TrimSpace(entity) == "" {
			return fmt.Errorf("%w: empty source", ErrInvalidProjection)
		}
		if _, exists := seenSources[entity]; exists {
			return fmt.Errorf("%w: duplicate source", ErrInvalidProjection)
		}
		seenSources[entity] = struct{}{}
	}
	seenFields := make(map[string]struct{}, len(d.AllowedFields))
	for _, field := range d.AllowedFields {
		if strings.TrimSpace(field) == "" {
			return fmt.Errorf("%w: empty field", ErrInvalidProjection)
		}
		if _, exists := seenFields[field]; exists {
			return fmt.Errorf("%w: duplicate field", ErrInvalidProjection)
		}
		seenFields[field] = struct{}{}
	}
	seenTasks := make(map[AssistTask]struct{}, len(d.AllowedTasks))
	for _, task := range d.AllowedTasks {
		if !validTask(task) {
			return fmt.Errorf("%w: unknown task", ErrInvalidProjection)
		}
		if _, exists := seenTasks[task]; exists {
			return fmt.Errorf("%w: duplicate task", ErrInvalidProjection)
		}
		seenTasks[task] = struct{}{}
	}
	return nil
}

func cloneDescriptor(d ProjectionDescriptor) ProjectionDescriptor {
	d.SourceEntities = append([]string(nil), d.SourceEntities...)
	d.AllowedFields = append([]string(nil), d.AllowedFields...)
	d.AllowedTasks = append([]AssistTask(nil), d.AllowedTasks...)
	return d
}

// Registry is immutable after construction; unknown projections, tasks,
// permissions and fields are denied. Authorization must still be checked
// against the authenticated principal by the application service.
type Registry struct {
	descriptors map[string]ProjectionDescriptor
}

func NewRegistry(entries ...ProjectionDescriptor) (*Registry, error) {
	r := &Registry{descriptors: make(map[string]ProjectionDescriptor, len(entries))}
	for _, entry := range entries {
		if err := validateDescriptor(entry); err != nil {
			return nil, err
		}
		if _, exists := r.descriptors[entry.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate projection ID", ErrInvalidProjection)
		}
		r.descriptors[entry.ID] = cloneDescriptor(entry)
	}
	return r, nil
}

func (r *Registry) Lookup(id string) (ProjectionDescriptor, bool) {
	if r == nil {
		return ProjectionDescriptor{}, false
	}
	d, ok := r.descriptors[id]
	if !ok {
		return ProjectionDescriptor{}, false
	}
	return cloneDescriptor(d), true
}

// CheckSelection checks only allowlist metadata; it DOES NOT authorize a
// user, read records, or verify tenant ownership. The caller must do all three.
func (r *Registry) CheckSelection(id string, task AssistTask, field, requiredPermission string) error {
	d, ok := r.Lookup(id)
	if !ok {
		return ErrUnknownProjection
	}
	if d.RequiredPermission != requiredPermission || requiredPermission == "" {
		return ErrDeniedProjection
	}
	taskAllowed := false
	for _, allowed := range d.AllowedTasks {
		if allowed == task {
			taskAllowed = true
			break
		}
	}
	if !taskAllowed {
		return ErrDeniedProjection
	}
	for _, allowed := range d.AllowedFields {
		if allowed == field {
			return nil
		}
	}
	return ErrDeniedProjection
}
