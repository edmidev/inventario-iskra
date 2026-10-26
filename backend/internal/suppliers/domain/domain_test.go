package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/edgar/inventario/backend/internal/suppliers/domain"
)

func TestInputNormalize(t *testing.T) {
	got, err := domain.Input{Name: "  Ferretería Sur ", Email: " ventas@sur.test ", Phone: " 555 ", Active: true}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ferretería Sur" || got.Email != "ventas@sur.test" || got.Phone != "555" || !got.Active {
		t.Fatalf("not trimmed: %+v", got)
	}

	cases := map[string]domain.Input{
		"name":         {Name: "   "},
		"email":        {Name: "A", Email: "not-an-email"},
		"contact_name": {Name: "A", ContactName: strings.Repeat("x", domain.MaxContactLen+1)},
		"phone":        {Name: "A", Phone: strings.Repeat("1", domain.MaxPhoneLen+1)},
		"notes":        {Name: "A", Notes: strings.Repeat("n", domain.MaxNotesLen+1)},
	}
	for field, in := range cases {
		_, err := in.Normalize()
		var ve *domain.ValidationError
		if !errors.As(err, &ve) || !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%s: want ValidationError, got %v", field, err)
		}
		if _, ok := ve.Fields[field]; !ok {
			t.Fatalf("%s: field error missing in %v", field, ve.Fields)
		}
	}
}

func TestFilterNormalize(t *testing.T) {
	f, err := domain.Filter{Q: "  tor "}.Normalize()
	if err != nil || f.Limit != domain.DefaultLimit || f.Q != "tor" {
		t.Fatalf("defaults: %+v %v", f, err)
	}
	for _, bad := range []domain.Filter{{Limit: -1}, {Limit: domain.MaxLimit + 1}, {Offset: -1}} {
		if _, err := bad.Normalize(); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("%+v: want invalid input, got %v", bad, err)
		}
	}
}
