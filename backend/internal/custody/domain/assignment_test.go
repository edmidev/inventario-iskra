package domain

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)

func ptr[T any](v T) *T { return &v }

func person() *Place    { return &Place{ID: 7, Name: "Alejandro", Kind: PlacePerson} }
func container() *Place { return &Place{ID: 8, Name: "Maleta", Kind: PlaceContainer} }

func okState() ItemState {
	return ItemState{Kind: KindAsset, Status: StatusFunctional, LocationID: ptr(int64(1)), Assignee: person()}
}

func fieldsOf(t *testing.T, err error) map[string]string {
	t.Helper()
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("want ValidationError, got %v", err)
	}
	return ve.Fields
}

func TestDecideAssign(t *testing.T) {
	active := &Assignment{ID: 3}
	tests := []struct {
		name      string
		mutate    func(*ItemState)
		req       AssignRequest
		wantErr   error
		wantField string
	}{
		{"asset to person", func(*ItemState) {}, AssignRequest{AssigneeLocationID: 7}, nil, ""},
		{"asset to container", func(s *ItemState) { s.Assignee = container() }, AssignRequest{AssigneeLocationID: 8}, nil, ""},
		{"future due date", func(*ItemState) {}, AssignRequest{AssigneeLocationID: 7, DueAt: ptr(t0.Add(time.Hour))}, nil, ""},
		{"consumable", func(s *ItemState) { s.Kind = KindConsumable }, AssignRequest{AssigneeLocationID: 7}, ErrNotApplicable, ""},
		{"inoperative", func(s *ItemState) { s.Status = "inoperative" }, AssignRequest{AssigneeLocationID: 7}, ErrItemNotAvailable, ""},
		{"in repair", func(s *ItemState) { s.Status = "in_repair" }, AssignRequest{AssigneeLocationID: 7}, ErrItemNotAvailable, ""},
		{"already assigned", func(s *ItemState) { s.Active = active }, AssignRequest{AssigneeLocationID: 7}, ErrAlreadyAssigned, ""},
		{"missing assignee id", func(*ItemState) {}, AssignRequest{}, ErrInvalidInput, "assignee_location_id"},
		{"unknown assignee", func(s *ItemState) { s.Assignee = nil }, AssignRequest{AssigneeLocationID: 99}, ErrInvalidInput, "assignee_location_id"},
		{"storage assignee", func(s *ItemState) { s.Assignee = &Place{ID: 2, Name: "Bodega", Kind: PlaceStorage} },
			AssignRequest{AssigneeLocationID: 2}, ErrInvalidInput, "assignee_location_id"},
		{"due in the past", func(*ItemState) {}, AssignRequest{AssigneeLocationID: 7, DueAt: ptr(t0.Add(-time.Minute))}, ErrInvalidInput, "due_at"},
		{"due now", func(*ItemState) {}, AssignRequest{AssigneeLocationID: 7, DueAt: ptr(t0)}, ErrInvalidInput, "due_at"},
		{"note too long", func(*ItemState) {}, AssignRequest{AssigneeLocationID: 7, Note: string(make([]byte, 501))}, ErrInvalidInput, "note"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cur := okState()
			tc.mutate(&cur)
			out, err := DecideAssign(cur, tc.req, t0)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatal(err)
				}
				if out.LocationID == nil || *out.LocationID != cur.Assignee.ID {
					t.Errorf("item must move to the assignee, got %v", out.LocationID)
				}
				if o := out.Assignment.OriginLocationID; o == nil || *o != 1 {
					t.Errorf("origin not remembered: %v", o)
				}
				if !out.Assignment.AssignedAt.Equal(t0) {
					t.Errorf("assigned_at = %v", out.Assignment.AssignedAt)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if tc.wantField != "" && fieldsOf(t, err)[tc.wantField] == "" {
				t.Errorf("no error on field %s: %v", tc.wantField, err)
			}
		})
	}
}

func TestDecideAssignTrimsNote(t *testing.T) {
	out, err := DecideAssign(okState(), AssignRequest{AssigneeLocationID: 7, Note: "  for job 4  "}, t0)
	if err != nil || out.Assignment.AssignNote != "for job 4" {
		t.Fatalf("%v %q", err, out.Assignment.AssignNote)
	}
}

func TestDecideReturn(t *testing.T) {
	loan := &Assignment{ID: 3, AssigneeLocationID: 7, OriginLocationID: ptr(int64(1)), AssignedAt: t0.Add(-time.Hour)}
	state := func() ItemState {
		return ItemState{Kind: KindAsset, Status: StatusFunctional, LocationID: ptr(int64(7)), Active: loan}
	}
	t.Run("returns to origin", func(t *testing.T) {
		out, err := DecideReturn(state(), ReturnRequest{Note: " ok "}, t0)
		if err != nil || out.ReturnedTo == nil || *out.ReturnedTo != 1 || out.AssignmentID != 3 || out.ReturnNote != "ok" {
			t.Fatalf("%+v %v", out, err)
		}
	})
	t.Run("explicit location wins", func(t *testing.T) {
		cur := state()
		cur.ReturnTo = &Place{ID: 5, Name: "Taller", Kind: PlaceStorage}
		out, err := DecideReturn(cur, ReturnRequest{ReturnLocationID: ptr(int64(5))}, t0)
		if err != nil || *out.ReturnedTo != 5 {
			t.Fatalf("%+v %v", out, err)
		}
	})
	t.Run("origin gone leaves item without location", func(t *testing.T) {
		cur := state()
		cur.Active = &Assignment{ID: 3, AssignedAt: t0.Add(-time.Hour)}
		out, err := DecideReturn(cur, ReturnRequest{}, t0)
		if err != nil || out.ReturnedTo != nil {
			t.Fatalf("%+v %v", out, err)
		}
	})
	t.Run("status is irrelevant", func(t *testing.T) {
		cur := state()
		cur.Status = "in_repair"
		if _, err := DecideReturn(cur, ReturnRequest{}, t0); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("not assigned", func(t *testing.T) {
		cur := state()
		cur.Active = nil
		if _, err := DecideReturn(cur, ReturnRequest{}, t0); !errors.Is(err, ErrNotAssigned) {
			t.Fatal(err)
		}
	})
	t.Run("consumable", func(t *testing.T) {
		cur := state()
		cur.Kind = KindConsumable
		if _, err := DecideReturn(cur, ReturnRequest{}, t0); !errors.Is(err, ErrNotApplicable) {
			t.Fatal(err)
		}
	})
	t.Run("unknown return location", func(t *testing.T) {
		_, err := DecideReturn(state(), ReturnRequest{ReturnLocationID: ptr(int64(99))}, t0)
		if fieldsOf(t, err)["return_location_id"] == "" {
			t.Fatal(err)
		}
	})
	t.Run("clock skew never returns before assignment", func(t *testing.T) {
		out, err := DecideReturn(state(), ReturnRequest{}, t0.Add(-2*time.Hour))
		if err != nil || !out.ReturnedAt.Equal(loan.AssignedAt) {
			t.Fatalf("%v %v", out.ReturnedAt, err)
		}
	})
}

func TestOverdue(t *testing.T) {
	past, future := t0.Add(-time.Hour), t0.Add(time.Hour)
	if !(Assignment{DueAt: &past}).Overdue(t0) {
		t.Error("past due date must be overdue")
	}
	if (Assignment{DueAt: &future}).Overdue(t0) || (Assignment{}).Overdue(t0) {
		t.Error("future or missing due date is not overdue")
	}
	if (Assignment{DueAt: &past, ReturnedAt: &t0}).Overdue(t0) {
		t.Error("a returned loan is never overdue")
	}
}

func TestNormalizePage(t *testing.T) {
	if p, err := NormalizePage(0, 0); err != nil || p.Limit != DefaultLimit {
		t.Fatal(p, err)
	}
	if f := fieldsOf(t, func() error { _, e := NormalizePage(101, -1); return e }()); f["limit"] == "" || f["offset"] == "" {
		t.Fatal(f)
	}
}
