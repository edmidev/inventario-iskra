package postgres_test

import (
	"database/sql"
	"errors"
	"fmt"
	executiondomain "github.com/edgar/inventario/backend/internal/executions/domain"
	"github.com/edgar/inventario/backend/internal/platform/quantity"
	"github.com/edgar/inventario/backend/migrations"
	"github.com/pressly/goose/v3"
	"os"
	"strings"
	"testing"
	"time"
)

func TestIndependentIncidentHistory(t *testing.T) {
	f := setupSupplementalMaterialFixture(t, quantity.Quantity(1000), quantity.Quantity(1000))
	e := f.create(1)
	db, err := sql.Open("pgx", os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.DownTo(f.ctx, 24); err != nil {
		t.Fatalf("empty event migration rollback: %v", err)
	}
	if _, err = provider.Up(f.ctx); err != nil {
		t.Fatal(err)
	}
	hash := func() string {
		var value string
		err := f.pool.QueryRow(f.ctx, `SELECT md5((SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]')::text FROM manufacturing_executions e)||(SELECT coalesce(jsonb_agg(to_jsonb(e) ORDER BY id),'[]')::text FROM manufacturing_execution_events e))`).Scan(&value)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	before := time.Now().Add(-time.Second)
	in := executiondomain.IncidentInput{RequestID: "00000000-0000-4000-8000-000000000301", Type: "positive", Message: "Milestone reached"}
	first, err := f.service.RecordIncident(f.ctx, e.ID, f.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	if first.Type != "positive" || first.ActorID != f.userID || first.ActorName == "" || first.OccurredAt.Before(before) || first.OccurredAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("bad actor/timestamp: %+v", first)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE users SET name='Renamed actor' WHERE id=$1`, f.userID); err != nil {
		t.Fatal(err)
	}
	replay, err := f.service.RecordIncident(f.ctx, e.ID, f.userID, in)
	if err != nil || replay.ID != first.ID || replay.ActorName != first.ActorName || !replay.OccurredAt.Equal(first.OccurredAt) {
		t.Fatalf("snapshot replay: %+v %v", replay, err)
	}
	in.RequestID = "00000000-0000-4000-8000-000000000302"
	in.Type = "negative"
	in.Message = "Material missing"
	type result struct {
		id  int64
		err error
	}
	out := make(chan result, 12)
	start := make(chan struct{})
	for i := 0; i < 12; i++ {
		go func() {
			<-start
			event, err := f.service.RecordIncident(f.ctx, e.ID, f.userID, in)
			out <- result{event.ID, err}
		}()
	}
	close(start)
	var id int64
	for i := 0; i < 12; i++ {
		r := <-out
		if r.err != nil {
			t.Fatal(r.err)
		}
		if id == 0 {
			id = r.id
		}
		if id != r.id {
			t.Fatalf("duplicate replay IDs: %d != %d", id, r.id)
		}
	}
	state := hash()
	conflicts := []executiondomain.IncidentInput{in, in, in}
	conflicts[0].Message = "different"
	conflicts[1].Type = "positive"
	conflicts[2].RequestID = "00000000-0000-4000-8000-000000000301"
	for _, c := range conflicts {
		if _, err = f.service.RecordIncident(f.ctx, e.ID, f.userID, c); !errors.Is(err, executiondomain.ErrIncidentReplay) {
			t.Fatalf("conflict accepted: %v", err)
		}
		if hash() != state {
			t.Fatal("conflict changed persisted state")
		}
	}
	for i, c := range []executiondomain.IncidentInput{{RequestID: "bad", Type: "positive", Message: "valid"}, {RequestID: "00000000-0000-4000-8000-000000000303", Type: "neutral", Message: "valid"}, {RequestID: "00000000-0000-4000-8000-000000000304", Type: "negative", Message: " "}, {RequestID: "00000000-0000-4000-8000-000000000305", Type: "positive", Message: strings.Repeat("a", 2001)}} {
		if _, err = f.service.RecordIncident(f.ctx, e.ID, f.userID, c); err == nil {
			t.Fatalf("invalid case %d accepted", i)
		}
		if hash() != state {
			t.Fatal("invalid input changed persisted state")
		}
	}
	if _, err = f.service.RecordIncident(f.ctx, 999999, f.userID, executiondomain.IncidentInput{RequestID: "00000000-0000-4000-8000-000000000306", Type: "positive", Message: "missing"}); !errors.Is(err, executiondomain.ErrNotFound) {
		t.Fatalf("nonexistent execution: %v", err)
	}
	if hash() != state {
		t.Fatal("nonexistent execution changed persisted state")
	}
	if _, err = provider.DownTo(f.ctx, 24); err == nil {
		t.Fatal("rollback discarded incident history")
	}
	if hash() != state {
		t.Fatal("rollback rejection changed persisted state")
	}
	events, err := f.service.ListEvents(f.ctx, e.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("timeline: %+v %v", events, err)
	}
	if events[0].ID != first.ID || events[1].ID != id || events[1].ActorName != "Renamed actor" {
		t.Fatalf("incorrect ordered timeline: %+v", events)
	}
	fmt.Printf("independent timeline IDs=%d,%d; 12 replay calls; preserved hash=%s\n", first.ID, id, state)
}
