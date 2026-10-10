package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/edgar/inventario/backend/internal/maintenance/domain"
)

var now = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

func tp(t time.Time) *time.Time { return &t }
func sp(s string) *string       { return &s }
func fp(f float64) *float64     { return &f }
func ip(i int) *int             { return &i }

var asset = domain.ItemState{Kind: domain.ItemAsset, Status: domain.ItemFunctional}

// check asserts err is nil, a sentinel, or a validation error on wantField.
func check(t *testing.T, name string, err, wantErr error, wantField string) bool {
	t.Helper()
	switch {
	case wantErr != nil:
		if !errors.Is(err, wantErr) {
			t.Errorf("%s: err = %v, want %v", name, err, wantErr)
		}
	case wantField != "":
		var ve *domain.ValidationError
		if !errors.As(err, &ve) || ve.Fields[wantField] == "" {
			t.Errorf("%s: err = %v, want validation error on %q", name, err, wantField)
		}
	case err != nil:
		t.Errorf("%s: unexpected error %v", name, err)
	default:
		return true
	}
	return false
}

func TestDecideReport(t *testing.T) {
	open := &domain.Record{ID: 1, Kind: domain.KindCorrective, Status: domain.StatusOpen}
	tests := []struct {
		name      string
		state     domain.ItemState
		req       domain.ReportRequest
		wantErr   error
		wantField string
		status    domain.ItemStatus
		at        time.Time
	}{
		{name: "defaults to inoperative and now", state: asset, req: domain.ReportRequest{Description: "Motor burnt"}, status: domain.ItemInoperative, at: now},
		{name: "in repair with explicit date", state: asset, req: domain.ReportRequest{Description: "x", ItemStatus: domain.ItemInRepair, ReportedAt: tp(now.Add(-48 * time.Hour))}, status: domain.ItemInRepair, at: now.Add(-48 * time.Hour)},
		{name: "reported exactly now", state: asset, req: domain.ReportRequest{Description: "x", ReportedAt: tp(now)}, status: domain.ItemInoperative, at: now},
		{name: "future date", state: asset, req: domain.ReportRequest{Description: "x", ReportedAt: tp(now.Add(time.Second))}, wantField: "reported_at"},
		{name: "blank description", state: asset, req: domain.ReportRequest{Description: "   "}, wantField: "description"},
		{name: "description too long", state: asset, req: domain.ReportRequest{Description: string(make([]rune, 1001))}, wantField: "description"},
		{name: "destination too long", state: asset, req: domain.ReportRequest{Description: "x", Destination: sp(string(make([]byte, 121)))}, wantField: "destination"},
		{name: "functional is not a failure status", state: asset, req: domain.ReportRequest{Description: "x", ItemStatus: domain.ItemFunctional}, wantField: "item_status"},
		{name: "unknown status", state: asset, req: domain.ReportRequest{Description: "x", ItemStatus: "broken"}, wantField: "item_status"},
		{name: "consumable", state: domain.ItemState{Kind: domain.ItemConsumable}, req: domain.ReportRequest{Description: "x"}, wantErr: domain.ErrNotApplicable},
		{name: "open failure exists", state: domain.ItemState{Kind: domain.ItemAsset, Open: open}, req: domain.ReportRequest{Description: "x"}, wantErr: domain.ErrOpenFailureExists},
		{name: "validation wins over open failure", state: domain.ItemState{Kind: domain.ItemAsset, Open: open}, req: domain.ReportRequest{}, wantField: "description"},
	}
	for _, tc := range tests {
		out, err := domain.DecideReport(tc.state, tc.req, now)
		if !check(t, tc.name, err, tc.wantErr, tc.wantField) {
			continue
		}
		r := out.Record
		if out.ItemStatus != tc.status || !r.ReportedAt.Equal(tc.at) || r.Kind != domain.KindCorrective || r.Status != domain.StatusOpen || r.ClosedAt != nil {
			t.Errorf("%s: %+v status=%s", tc.name, r, out.ItemStatus)
		}
	}
}

func TestReportTrimsAndNormalizesDestination(t *testing.T) {
	out, err := domain.DecideReport(asset, domain.ReportRequest{Description: "  Motor burnt \n", Destination: sp("  Taller de Reparación Interno ")}, now)
	if err != nil || out.Record.Description != "Motor burnt" || *out.Record.Destination != "Taller de Reparación Interno" {
		t.Fatalf("%+v %v", out.Record, err)
	}
	out, err = domain.DecideReport(asset, domain.ReportRequest{Description: "x", Destination: sp("  ")}, now)
	if err != nil || out.Record.Destination != nil {
		t.Errorf("blank destination must become nil: %+v %v", out.Record.Destination, err)
	}
}

func TestDecidePreventive(t *testing.T) {
	last := now.Add(-24 * time.Hour)
	withInterval := domain.ItemState{Kind: domain.ItemAsset, IntervalDays: ip(30), LastMaintenanceAt: &last}
	performed := now.Add(-2 * time.Hour)
	tests := []struct {
		name      string
		state     domain.ItemState
		req       domain.PreventiveRequest
		wantErr   error
		wantField string
		next      *time.Time
		last      time.Time
	}{
		{name: "defaults to now, no interval", state: asset, req: domain.PreventiveRequest{Description: "Oil change"}, last: now},
		{name: "next due derived from interval", state: withInterval, req: domain.PreventiveRequest{Description: "x", PerformedAt: &performed}, next: tp(domain.AddDays(performed, 30)), last: performed},
		{name: "explicit next due wins", state: withInterval, req: domain.PreventiveRequest{Description: "x", PerformedAt: &performed, NextDueAt: tp(now.Add(time.Hour))}, next: tp(now.Add(time.Hour)), last: performed},
		{name: "older service never moves last maintenance back", state: withInterval, req: domain.PreventiveRequest{Description: "x", PerformedAt: tp(now.Add(-72 * time.Hour))}, next: tp(domain.AddDays(now.Add(-72*time.Hour), 30)), last: last},
		{name: "cost rounded to cents", state: asset, req: domain.PreventiveRequest{Description: "x", Cost: fp(12.345678)}, last: now},
		{name: "future performed_at", state: asset, req: domain.PreventiveRequest{Description: "x", PerformedAt: tp(now.Add(time.Minute))}, wantField: "performed_at"},
		{name: "next due not after performed", state: asset, req: domain.PreventiveRequest{Description: "x", PerformedAt: &performed, NextDueAt: &performed}, wantField: "next_due_at"},
		{name: "negative cost", state: asset, req: domain.PreventiveRequest{Description: "x", Cost: fp(-1)}, wantField: "cost"},
		{name: "cost too large", state: asset, req: domain.PreventiveRequest{Description: "x", Cost: fp(1e12)}, wantField: "cost"},
		{name: "blank description", state: asset, req: domain.PreventiveRequest{}, wantField: "description"},
		{name: "consumable", state: domain.ItemState{Kind: domain.ItemConsumable}, req: domain.PreventiveRequest{Description: "x"}, wantErr: domain.ErrNotApplicable},
	}
	for _, tc := range tests {
		out, err := domain.DecidePreventive(tc.state, tc.req, now)
		if !check(t, tc.name, err, tc.wantErr, tc.wantField) {
			continue
		}
		r := out.Record
		if r.Kind != domain.KindPreventive || r.Status != domain.StatusClosed || r.ClosedAt == nil || !r.ClosedAt.Equal(r.ReportedAt) {
			t.Errorf("%s: record %+v", tc.name, r)
		}
		if (tc.next == nil) != (r.NextDueAt == nil) || (tc.next != nil && !tc.next.Equal(*r.NextDueAt)) {
			t.Errorf("%s: next due = %v, want %v", tc.name, r.NextDueAt, tc.next)
		}
		if !out.LastMaintenanceAt.Equal(tc.last) {
			t.Errorf("%s: last maintenance = %v, want %v", tc.name, out.LastMaintenanceAt, tc.last)
		}
	}
	out, _ := domain.DecidePreventive(asset, domain.PreventiveRequest{Description: "x", Cost: fp(12.345678)}, now)
	if *out.Record.Cost != 12.35 {
		t.Errorf("cost = %v", *out.Record.Cost)
	}
}

func openRecord() domain.Record {
	return domain.Record{ID: 7, Kind: domain.KindCorrective, Status: domain.StatusOpen, ReportedAt: now.Add(-72 * time.Hour), Description: "Motor burnt"}
}

func TestDecideUpdate(t *testing.T) {
	closed := openRecord()
	closed.Status = domain.StatusClosed
	tests := []struct {
		name      string
		state     domain.ItemState
		rec       domain.Record
		req       domain.UpdateRequest
		wantErr   error
		wantField string
		item      domain.ItemStatus
		desc      string
		dest      *string
	}{
		{name: "move to repair with destination", state: asset, rec: openRecord(), req: domain.UpdateRequest{SetDestination: true, Destination: sp("Taller de Reparación Interno"), ItemStatus: domain.ItemInRepair}, item: domain.ItemInRepair, desc: "Motor burnt", dest: sp("Taller de Reparación Interno")},
		{name: "back to inoperative", state: asset, rec: openRecord(), req: domain.UpdateRequest{ItemStatus: domain.ItemInoperative}, item: domain.ItemInoperative, desc: "Motor burnt"},
		{name: "edit description only leaves item", state: asset, rec: openRecord(), req: domain.UpdateRequest{Description: sp(" Bearing seized ")}, desc: "Bearing seized"},
		{name: "null destination clears it", state: asset, rec: func() domain.Record { r := openRecord(); r.Destination = sp("Taller"); return r }(), req: domain.UpdateRequest{SetDestination: true}, desc: "Motor burnt"},
		{name: "empty patch", state: asset, rec: openRecord(), req: domain.UpdateRequest{}, wantField: "body"},
		{name: "blank description", state: asset, rec: openRecord(), req: domain.UpdateRequest{Description: sp(" ")}, wantField: "description"},
		{name: "functional is closed's job", state: asset, rec: openRecord(), req: domain.UpdateRequest{ItemStatus: domain.ItemFunctional}, wantField: "item_status"},
		{name: "closed record is immutable", state: asset, rec: closed, req: domain.UpdateRequest{Description: sp("x")}, wantErr: domain.ErrRecordClosed},
		{name: "consumable", state: domain.ItemState{Kind: domain.ItemConsumable}, rec: openRecord(), req: domain.UpdateRequest{Description: sp("x")}, wantErr: domain.ErrNotApplicable},
	}
	for _, tc := range tests {
		out, err := domain.DecideUpdate(tc.state, tc.rec, tc.req)
		if !check(t, tc.name, err, tc.wantErr, tc.wantField) {
			continue
		}
		d := out.Record.Destination
		if out.ItemStatus != tc.item || out.Record.Description != tc.desc || (d == nil) != (tc.dest == nil) || (d != nil && *d != *tc.dest) {
			t.Errorf("%s: %+v item=%q", tc.name, out.Record, out.ItemStatus)
		}
	}
}

func TestDecideClose(t *testing.T) {
	closed := openRecord()
	closed.Status = domain.StatusClosed
	last := now.Add(-24 * time.Hour)
	tests := []struct {
		name      string
		state     domain.ItemState
		rec       domain.Record
		req       domain.CloseRequest
		wantErr   error
		wantField string
		item      domain.ItemStatus
		closedAt  time.Time
		lastMaint *time.Time
	}{
		{name: "defaults: functional now", state: asset, rec: openRecord(), req: domain.CloseRequest{}, item: domain.ItemFunctional, closedAt: now, lastMaint: tp(now)},
		{name: "explicit date, cost and note", state: asset, rec: openRecord(), req: domain.CloseRequest{ClosedAt: tp(now.Add(-time.Hour)), Cost: fp(80), ResolutionNote: " Replaced motor "}, item: domain.ItemFunctional, closedAt: now.Add(-time.Hour), lastMaint: tp(now.Add(-time.Hour))},
		{name: "closed on the reported instant", state: asset, rec: openRecord(), req: domain.CloseRequest{ClosedAt: tp(openRecord().ReportedAt)}, item: domain.ItemFunctional, closedAt: openRecord().ReportedAt, lastMaint: tp(openRecord().ReportedAt)},
		{name: "stays inoperative: no last maintenance", state: asset, rec: openRecord(), req: domain.CloseRequest{ItemStatusAfter: domain.ItemInoperative}, item: domain.ItemInoperative, closedAt: now},
		{name: "last maintenance never moves back", state: domain.ItemState{Kind: domain.ItemAsset, LastMaintenanceAt: &last}, rec: func() domain.Record { r := openRecord(); r.ReportedAt = now.Add(-96 * time.Hour); return r }(), req: domain.CloseRequest{ClosedAt: tp(now.Add(-48 * time.Hour))}, item: domain.ItemFunctional, closedAt: now.Add(-48 * time.Hour), lastMaint: &last},
		{name: "before report", state: asset, rec: openRecord(), req: domain.CloseRequest{ClosedAt: tp(openRecord().ReportedAt.Add(-time.Second))}, wantField: "closed_at"},
		{name: "future", state: asset, rec: openRecord(), req: domain.CloseRequest{ClosedAt: tp(now.Add(time.Second))}, wantField: "closed_at"},
		{name: "negative cost", state: asset, rec: openRecord(), req: domain.CloseRequest{Cost: fp(-0.01)}, wantField: "cost"},
		{name: "bad item status", state: asset, rec: openRecord(), req: domain.CloseRequest{ItemStatusAfter: domain.ItemInRepair}, wantField: "item_status_after"},
		{name: "note too long", state: asset, rec: openRecord(), req: domain.CloseRequest{ResolutionNote: string(make([]byte, 1001))}, wantField: "resolution_note"},
		{name: "already closed", state: asset, rec: closed, req: domain.CloseRequest{}, wantErr: domain.ErrRecordClosed},
		{name: "consumable", state: domain.ItemState{Kind: domain.ItemConsumable}, rec: openRecord(), req: domain.CloseRequest{}, wantErr: domain.ErrNotApplicable},
	}
	for _, tc := range tests {
		out, err := domain.DecideClose(tc.state, tc.rec, tc.req, now)
		if !check(t, tc.name, err, tc.wantErr, tc.wantField) {
			continue
		}
		r := out.Record
		if out.ItemStatus != tc.item || r.Status != domain.StatusClosed || r.ClosedAt == nil || !r.ClosedAt.Equal(tc.closedAt) {
			t.Errorf("%s: %+v item=%s", tc.name, r, out.ItemStatus)
		}
		if (tc.lastMaint == nil) != (out.LastMaintenanceAt == nil) || (tc.lastMaint != nil && !tc.lastMaint.Equal(*out.LastMaintenanceAt)) {
			t.Errorf("%s: last maintenance = %v, want %v", tc.name, out.LastMaintenanceAt, tc.lastMaint)
		}
	}
}

func TestCloseClampsClockSkew(t *testing.T) {
	rec := openRecord()
	rec.ReportedAt = now.Add(time.Minute) // reported by an instance whose clock is ahead
	out, err := domain.DecideClose(asset, rec, domain.CloseRequest{}, now)
	if err != nil || !out.Record.ClosedAt.Equal(rec.ReportedAt) {
		t.Errorf("%v %v", out.Record.ClosedAt, err)
	}
}

func TestCreateRequestSplitRejectsFieldsOfTheOtherKind(t *testing.T) {
	tests := []struct {
		name  string
		req   domain.CreateRequest
		field string // empty: accepted
	}{
		{name: "corrective ok", req: domain.CreateRequest{Kind: domain.KindCorrective, Description: "x", Destination: sp("Taller"), ItemStatus: domain.ItemInRepair}},
		{name: "preventive ok", req: domain.CreateRequest{Kind: domain.KindPreventive, Description: "x", Cost: fp(1), NextDueAt: tp(now)}},
		{name: "corrective with cost", req: domain.CreateRequest{Kind: domain.KindCorrective, Cost: fp(1)}, field: "cost"},
		{name: "corrective with performed_at", req: domain.CreateRequest{Kind: domain.KindCorrective, PerformedAt: tp(now)}, field: "performed_at"},
		{name: "preventive with item_status", req: domain.CreateRequest{Kind: domain.KindPreventive, ItemStatus: domain.ItemInRepair}, field: "item_status"},
		{name: "preventive with reported_at", req: domain.CreateRequest{Kind: domain.KindPreventive, ReportedAt: tp(now)}, field: "reported_at"},
		{name: "preventive with destination", req: domain.CreateRequest{Kind: domain.KindPreventive, Destination: sp("x")}, field: "destination"},
		{name: "missing kind", req: domain.CreateRequest{}, field: "kind"},
	}
	for _, tc := range tests {
		_, _, err := tc.req.Split()
		check(t, tc.name, err, nil, tc.field)
	}
}

func TestDueItemOverdue(t *testing.T) {
	d := domain.DueItem{DueAt: now}
	if d.Overdue(now) {
		t.Error("due exactly now is not overdue yet")
	}
	if !d.Overdue(now.Add(time.Nanosecond)) {
		t.Error("past due must be overdue")
	}
}

func TestNormalizePage(t *testing.T) {
	if p, err := domain.NormalizePage(0, 0); err != nil || p.Limit != domain.DefaultLimit {
		t.Errorf("%+v %v", p, err)
	}
	for _, c := range [][2]int{{101, 0}, {-1, 0}, {10, -1}} {
		if _, err := domain.NormalizePage(c[0], c[1]); !errors.Is(err, domain.ErrInvalidInput) {
			t.Errorf("%v: %v", c, err)
		}
	}
}
