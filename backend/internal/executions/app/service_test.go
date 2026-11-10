package app

import (
	"context"
	"testing"

	"github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	projectdomain "github.com/edgar/inventario/backend/internal/projects/domain"
)

type templateReaderFake struct{ project projectdomain.Project }

func (f templateReaderFake) Get(_ context.Context, id int64) (projectdomain.Project, error) {
	if f.project.ID != id {
		return projectdomain.Project{}, projectdomain.ErrNotFound
	}
	return f.project.Clone(), nil
}

type executionRepoFake struct{ saved domain.Execution }

type addedToolCall struct {
	executionID int64
	requestID   string
	userID      string
	itemID      string
	fingerprint string
}
type toolRepoFake struct {
	executionRepoFake
	toolCall addedToolCall
}

func (f *executionRepoFake) Create(_ context.Context, e domain.Execution) (domain.Execution, error) {
	f.saved = e.Clone()
	return f.saved, nil
}

func (f *executionRepoFake) LinkPurchaseOrderLine(context.Context, int64, int64, quantity.Quantity) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *executionRepoFake) AddMaterialRequirement(context.Context, int64, string, string, quantity.Quantity, string) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *executionRepoFake) AddToolRequirement(context.Context, int64, string, string, string, string) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *toolRepoFake) AddToolRequirement(_ context.Context, executionID int64, requestID, userID, itemID, fingerprint string) (domain.Execution, error) {
	f.toolCall = addedToolCall{executionID, requestID, userID, itemID, fingerprint}
	return domain.Execution{}, nil
}

func (f *executionRepoFake) Get(context.Context, int64) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *executionRepoFake) List(context.Context, domain.Filter) ([]domain.Execution, int, error) {
	return nil, 0, nil
}

func (f *executionRepoFake) Transition(context.Context, int64, domain.Status, domain.Status) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *executionRepoFake) Close(context.Context, int64, domain.Status) (domain.Execution, error) {
	return domain.Execution{}, nil
}

func (f *executionRepoFake) Produce(context.Context, int64, string, domain.ProduceInput) ([]domain.FinishedUnit, error) {
	return []domain.FinishedUnit{}, nil
}

func (f *executionRepoFake) ListUnits(context.Context, int64) ([]domain.FinishedUnit, error) {
	return []domain.FinishedUnit{}, nil
}

func (f *executionRepoFake) DeliverFinishedUnits(context.Context, int64, string, domain.DeliveryInput, string) (domain.DeliveryReceipt, error) {
	return domain.DeliveryReceipt{}, nil
}

func (f *executionRepoFake) ListDeliveries(context.Context, int64) ([]domain.DeliveryReceipt, error) {
	return []domain.DeliveryReceipt{}, nil
}

func (f *executionRepoFake) ListAssignableUsers(context.Context) ([]domain.AssignableUser, error) {
	return nil, nil
}

func TestCreateCopiesTemplateAndAppliesRegistrationValues(t *testing.T) {
	template := projectdomain.Project{ID: 5, Input: projectdomain.Input{
		Name: "Marmita", Description: "Inox", Characteristics: []string{"Calentamiento"},
		Consumables: []projectdomain.Material{{ItemID: "steel", Quantity: quantity.Quantity(1250)}},
		Tools:       []string{"welder"}, EstimatedDays: 12,
	}}
	repo := &executionRepoFake{}
	svc := NewService(repo, templateReaderFake{project: template})
	in := domain.CreateInput{ProjectID: 5, Units: 2, CustomerIDs: []int64{9, 10}, PrincipalUserID: "user-a", WorkerIDs: []string{"user-b"}}
	got, err := svc.Create(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Marmita" || got.Consumables[0].QuantityPerUnit != quantity.Quantity(1250) || got.EstimatedDays != 12 {
		t.Fatalf("template snapshot/default duration = %#v", got)
	}
	if got.Units != 2 || len(got.CustomerIDs) != 2 || got.PrincipalUserID != "user-a" || len(got.WorkerIDs) != 1 {
		t.Fatalf("registration values = %#v", got)
	}
	template.Characteristics[0] = "modified later"
	template.Consumables[0].Quantity = 1
	if repo.saved.Characteristics[0] != "Calentamiento" || repo.saved.Consumables[0].QuantityPerUnit != quantity.Quantity(1250) {
		t.Fatalf("stored snapshot aliases template: %#v", repo.saved)
	}
}

func TestCreateAllowsExecutionDurationOverride(t *testing.T) {
	template := projectdomain.Project{ID: 5, Input: projectdomain.Input{Name: "Marmita", EstimatedDays: 12}}
	svc := NewService(&executionRepoFake{}, templateReaderFake{project: template})
	got, err := svc.Create(context.Background(), domain.CreateInput{ProjectID: 5, Units: 1, EstimatedDays: 7, CustomerIDs: []int64{9}, PrincipalUserID: "user-a"})
	if err != nil {
		t.Fatal(err)
	}
	if got.EstimatedDays != 7 {
		t.Fatalf("estimated days = %d, want override 7", got.EstimatedDays)
	}
}

func TestCreateCopiesSupplementalMaterialsWithoutScaling(t *testing.T) {
	template := projectdomain.Project{ID: 5, Input: projectdomain.Input{
		Name: "Marmita", EstimatedDays: 12,
		Consumables: []projectdomain.Material{{ItemID: "00000000-0000-4000-8000-000000000010", Quantity: 1250}},
	}}
	repo := &executionRepoFake{}
	svc := NewService(repo, templateReaderFake{project: template})
	_, err := svc.Create(context.Background(), domain.CreateInput{
		ProjectID: 5, Units: 4, CustomerIDs: []int64{9}, PrincipalUserID: "user-a",
		AdditionalMaterials: []domain.AdditionalMaterial{{ItemID: "00000000-0000-4000-8000-000000000011", Quantity: 750}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.saved.Units != 4 || len(repo.saved.AdditionalMaterials) != 1 || repo.saved.AdditionalMaterials[0].Quantity != 750 {
		t.Fatalf("supplemental quantity was not retained as an execution total: %+v", repo.saved)
	}
}

func TestCreateKeepsSupplementalToolsExecutionSpecific(t *testing.T) {
	template := projectdomain.Project{ID: 5, Input: projectdomain.Input{Name: "Marmita", Tools: []string{"template-tool"}, EstimatedDays: 12}}
	repo := &executionRepoFake{}
	svc := NewService(repo, templateReaderFake{project: template})
	_, err := svc.Create(context.Background(), domain.CreateInput{
		ProjectID: 5, Units: 1, CustomerIDs: []int64{9}, PrincipalUserID: "user-a",
		AdditionalTools: []string{"00000000-0000-4000-8000-000000000011"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.saved.Tools) != 1 || repo.saved.Tools[0] != "template-tool" || len(repo.saved.AdditionalTools) != 1 {
		t.Fatalf("template tools must remain separate from execution-only tools: %+v", repo.saved)
	}
}

func TestAddToolRequirementFingerprintsCanonicalExecutionRequest(t *testing.T) {
	repo := &toolRepoFake{}
	svc := NewService(repo, templateReaderFake{})
	_, err := svc.AddToolRequirement(context.Background(), 42, "user-a", domain.AddToolInput{
		RequestID: "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA",
		ItemID:    "BBBBBBBB-BBBB-4BBB-8BBB-BBBBBBBBBBBB",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.toolCall.executionID != 42 || repo.toolCall.userID != "user-a" || repo.toolCall.requestID != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || repo.toolCall.itemID != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" || repo.toolCall.fingerprint == "" {
		t.Fatalf("tool addition request was not canonicalized/fingerprinted: %+v", repo.toolCall)
	}
}
