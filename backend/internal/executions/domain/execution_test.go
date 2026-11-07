package domain

import (
	"errors"
	"testing"
)

func TestCreateInputNormalize(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateInput
		wantErr bool
	}{
		{
			name: "multiple clients and workers are retained",
			input: CreateInput{
				ProjectID: 4, Units: 3, CustomerIDs: []int64{8, 9},
				PrincipalUserID: "user-a", WorkerIDs: []string{"user-b", "user-c"},
			},
		},
		{name: "customer is required", input: CreateInput{ProjectID: 4, Units: 1, PrincipalUserID: "user-a"}, wantErr: true},
		{name: "principal is required", input: CreateInput{ProjectID: 4, Units: 1, CustomerIDs: []int64{8}}, wantErr: true},
		{name: "units are whole and positive", input: CreateInput{ProjectID: 4, Units: 0, CustomerIDs: []int64{8}, PrincipalUserID: "user-a"}, wantErr: true},
		{name: "duration override must be positive", input: CreateInput{ProjectID: 4, Units: 1, EstimatedDays: -1, CustomerIDs: []int64{8}, PrincipalUserID: "user-a"}, wantErr: true},
		{name: "duplicate customer is rejected", input: CreateInput{ProjectID: 4, Units: 1, CustomerIDs: []int64{8, 8}, PrincipalUserID: "user-a"}, wantErr: true},
		{name: "principal cannot be assigned twice", input: CreateInput{ProjectID: 4, Units: 1, CustomerIDs: []int64{8}, PrincipalUserID: "user-a", WorkerIDs: []string{"user-a"}}, wantErr: true},
		{name: "duplicate worker is rejected", input: CreateInput{ProjectID: 4, Units: 1, CustomerIDs: []int64{8}, PrincipalUserID: "user-a", WorkerIDs: []string{"user-b", "user-b"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Normalize()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Fatalf("Normalize() error = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Normalize() error = %v", err)
			}
			if len(got.CustomerIDs) != 2 || len(got.WorkerIDs) != 2 {
				t.Fatalf("Normalize() lost requested assignments: %#v", got)
			}
		})
	}
}

func TestExecutionSnapshotDoesNotAliasTemplate(t *testing.T) {
	template := ProjectSnapshot{
		ProjectID: 4, Name: "Marmita", Description: "Acero inoxidable",
		Characteristics: []string{"Calentamiento"},
		Consumables:     []MaterialSnapshot{{ItemID: "item-1", QuantityPerUnit: 1250}},
		Tools:           []string{"asset-1"}, EstimatedDays: 12,
	}
	input := CreateInput{ProjectID: 4, Units: 2, CustomerIDs: []int64{8}, PrincipalUserID: "user-a", EstimatedDays: 9}
	execution, err := NewExecution(input, template)
	if err != nil {
		t.Fatal(err)
	}
	template.Characteristics[0] = "edited"
	template.Consumables[0].QuantityPerUnit = 7
	template.Tools[0] = "asset-2"
	if execution.Characteristics[0] != "Calentamiento" || execution.Consumables[0].QuantityPerUnit != 1250 || execution.Tools[0] != "asset-1" {
		t.Fatalf("execution snapshot changed with template: %#v", execution)
	}
	if execution.Units != 2 || execution.EstimatedDays != 9 {
		t.Fatalf("units/duration override not retained: units=%d days=%d", execution.Units, execution.EstimatedDays)
	}
}

func TestExecutionCanStartBeforeMaterialsAreAvailable(t *testing.T) {
	e := Execution{Status: StatusRegistered}
	if err := e.Start(); err != nil {
		t.Fatalf("Start() rejected partial start: %v", err)
	}
	if e.Status != StatusInProgress {
		t.Fatalf("status = %q, want %q", e.Status, StatusInProgress)
	}
}
