package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrIncidentReplay = errors.New("incident request id was reused with different terms")

type IncidentInput struct {
	RequestID string `json:"request_id"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

func (in IncidentInput) Normalize() (IncidentInput, error) {
	in.RequestID = strings.ToLower(strings.TrimSpace(in.RequestID))
	in.Type = strings.TrimSpace(in.Type)
	in.Message = strings.TrimSpace(in.Message)
	fields := map[string]string{}
	if !validUUID(in.RequestID) {
		fields["request_id"] = "must be a valid UUID"
	}
	if in.Type != "positive" && in.Type != "negative" {
		fields["type"] = "must be positive or negative"
	}
	if size := utf8.RuneCountInString(in.Message); size < 1 || size > 2000 {
		fields["message"] = "must contain 1 to 2000 characters"
	}
	if len(fields) > 0 {
		return IncidentInput{}, &ValidationError{Fields: fields}
	}
	return in, nil
}

type ExecutionEvent struct {
	ID          int64     `json:"id"`
	ExecutionID int64     `json:"execution_id"`
	Type        string    `json:"type"`
	Message     string    `json:"message"`
	ActorID     string    `json:"actor_id,omitempty"`
	ActorName   string    `json:"actor_name"`
	OccurredAt  time.Time `json:"occurred_at"`
}
