package domain

import (
	"errors"
	"testing"

	"github.com/edgar/inventario/backend/internal/platform/quantity"
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

func TestCreateInputMergesSupplementalTotalsByItem(t *testing.T) {
	itemID := "00000000-0000-4a00-8000-000000000098"
	input := CreateInput{
		ProjectID: 4, Units: 5, CustomerIDs: []int64{8}, PrincipalUserID: "user-a",
		AdditionalMaterials: []AdditionalMaterial{{ItemID: itemID, Quantity: quantity.Quantity(250)}, {ItemID: "00000000-0000-4A00-8000-000000000098", Quantity: quantity.Quantity(750)}},
	}
	normalized, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.AdditionalMaterials) != 1 || normalized.AdditionalMaterials[0].Quantity != quantity.Quantity(1000) {
		t.Fatalf("same-item additions should combine without unit scaling: %+v", normalized.AdditionalMaterials)
	}
	if normalized.AdditionalMaterials[0].ItemID != itemID {
		t.Fatalf("item UUID should be canonical lowercase: %q", normalized.AdditionalMaterials[0].ItemID)
	}
}

func TestAddMaterialInputRequiresPositiveTotalAndUUIDs(t *testing.T) {
	_, err := (AddMaterialInput{RequestID: "not-a-uuid", ItemID: "not-a-uuid", Quantity: 0}).Normalize()
	var validation *ValidationError
	if !errors.As(err, &validation) || len(validation.Fields) != 3 {
		t.Fatalf("Normalize() error = %v, want request/item UUID and positive total field errors", err)
	}
}

func TestAddMaterialInputCanonicalizesUUIDs(t *testing.T) {
	input, err := (AddMaterialInput{
		RequestID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA",
		ItemID:    "BBBBBBBB-BBBB-4BBB-8BBB-BBBBBBBBBBBB",
		Quantity:  1,
	}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if input.RequestID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || input.ItemID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
		t.Fatalf("UUIDs should be canonical lowercase: %+v", input)
	}
}

func TestCreateInputCanonicalizesAndDeduplicatesAdditionalTools(t *testing.T) {
	input := CreateInput{
		ProjectID: 4, Units: 1, CustomerIDs: []int64{8}, PrincipalUserID: "user-a",
		AdditionalTools: []string{
			"AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA",
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		},
	}
	normalized, err := input.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if len(normalized.AdditionalTools) != 1 || normalized.AdditionalTools[0] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("additional tools should be canonical and unique: %#v", normalized.AdditionalTools)
	}
}

func TestAddToolInputCanonicalizesUUIDs(t *testing.T) {
	input, err := (AddToolInput{
		RequestID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA",
		ItemID:    "BBBBBBBB-BBBB-4BBB-8BBB-BBBBBBBBBBBB",
	}).Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if input.RequestID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || input.ItemID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
		t.Fatalf("UUIDs should be canonical lowercase: %+v", input)
	}
}

func TestProduceInputRequiresUUIDAndPositiveWholeQuantity(t *testing.T) {
	if _, err := (ProduceInput{RequestID: "bad", Quantity: 0}).Normalize(); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid production input = %v", err)
	}
	input, err := (ProduceInput{RequestID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", Quantity: 2}).Normalize()
	if err != nil || input.RequestID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" {
		t.Fatalf("normalized production input = %+v, err=%v", input, err)
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

func TestSupplementalMaterialQuantityIsExecutionTotal(t *testing.T) {
	template := ProjectSnapshot{
		ProjectID: 4, Name: "Marmita", EstimatedDays: 12,
		Consumables: []MaterialSnapshot{{ItemID: "item-template", QuantityPerUnit: 1000}},
	}
	input := CreateInput{
		ProjectID: 4, Units: 3, CustomerIDs: []int64{8}, PrincipalUserID: "user-a",
		AdditionalMaterials: []AdditionalMaterial{{ItemID: "00000000-0000-4000-8000-000000000099", Quantity: 1250}},
	}
	execution, err := NewExecution(input, template)
	if err != nil {
		t.Fatal(err)
	}
	if execution.Units != 3 || len(execution.AdditionalMaterials) != 1 || execution.AdditionalMaterials[0].Quantity != 1250 {
		t.Fatalf("supplemental quantity must remain a total for the execution: %+v", execution)
	}
}

func TestNewExecutionKeepsSupplementalToolsSeparateFromTemplate(t *testing.T) {
	template := ProjectSnapshot{ProjectID: 4, Name: "Marmita", Tools: []string{"tool-template"}, EstimatedDays: 12}
	input := CreateInput{ProjectID: 4, Units: 2, CustomerIDs: []int64{8}, PrincipalUserID: "user-a", AdditionalTools: []string{"00000000-0000-4000-8000-000000000099"}}
	execution, err := NewExecution(input, template)
	if err != nil {
		t.Fatal(err)
	}
	if len(execution.Tools) != 1 || execution.Tools[0] != "tool-template" || len(execution.AdditionalTools) != 1 {
		t.Fatalf("template and supplemental tool snapshots should remain separate: %+v", execution)
	}
	clone := execution.Clone()
	clone.AdditionalTools[0] = "changed"
	if execution.AdditionalTools[0] == "changed" {
		t.Fatal("clone aliases supplemental tool list")
	}
}
