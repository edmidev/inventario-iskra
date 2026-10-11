package importer

import (
	"strings"
)

// item defaults shared by the row parsers.
func (p *parseCtx) base(desc string, kind, category string) PlannedItem {
	return PlannedItem{Description: Clean(desc), Kind: kind, Category: category, Status: "functional", Attributes: map[string]any{}}
}

func (p *parseCtx) invalid(it PlannedItem, format string, args ...any) PlannedItem {
	it.Action = ActionSkipInvalid
	it.flag(FlagInvalid, format, args...)
	return it
}

// checkDescription flags an empty or oversized description.
func (p *parseCtx) checkDescription(it PlannedItem) (PlannedItem, bool) {
	switch {
	case it.Description == "":
		return p.invalid(it, "row has no description; skipped"), false
	case len([]rune(it.Description)) > 500:
		return p.invalid(it, "description is longer than 500 characters; skipped"), false
	}
	return it, true
}

// resolveCategory maps a raw category cell, defaulting (and flagging) when it
// is empty or unknown. A raw value that differs from the canonical lowercase
// name is reported as normalized.
func (p *parseCtx) resolveCategory(it *PlannedItem, raw, fallback string) {
	name, ok := CanonicalCategory(raw)
	if !ok {
		it.Category = fallback
		if strings.TrimSpace(raw) == "" {
			it.flag(FlagSuspicious, "no category in the workbook; defaulted to %s", fallback)
		} else {
			it.flag(FlagSuspicious, "unknown category %q; defaulted to %s", raw, fallback)
		}
		return
	}
	it.Category = name
	if Fold(raw) != Fold(name) {
		it.flag(FlagCategoryNormalized, "%q normalized to %s", raw, name)
	}
}

func (p *parseCtx) defaultLocation(it *PlannedItem) { it.Location = p.plan.DefaultLocation }

// resolveDate converts a date cell and flags the conversion.
func (p *parseCtx) resolveDate(it *PlannedItem, raw, what string) (ok bool) {
	t, via, ok := parseDate(raw, p.loc)
	if !ok {
		it.flag(FlagSuspicious, "%s %q is not a recognizable date; ignored", what, raw)
		return false
	}
	switch {
	case strings.HasPrefix(via, "excel serial"):
		it.flag(FlagDateConverted, "%s %s converted to %s", what, via, t.Format("2006-01-02"))
	case via != "iso text":
		it.flag(FlagDateConverted, "%s %q (%s) converted to %s", what, raw, via, t.Format("2006-01-02"))
	}
	it.LastMaintenanceAt = &t
	return true
}

// ---- sheet "Activos y Disponibles" ----

func (p *parseCtx) assetRow(v rowView, c columns) PlannedItem {
	it := p.base(v.get(c.desc), "asset", CatTools)
	it.Stock = 1
	p.defaultLocation(&it)
	if raw := v.get(c.code); raw != "" {
		code, rest, ok := SplitCode(raw)
		switch {
		case !ok:
			return p.invalid(it, "code %q does not look like ISK-ABC-001; skipped", raw)
		case rest != "":
			it.flag(FlagFused, "code cell also contained text %q; used as description only if the description cell is empty", rest)
			if it.Description == "" {
				it.Description = rest
			}
			it.Code = code
		default:
			it.Code = code
		}
	} else {
		it.flag(FlagNoCode, "no code; one will be generated from the category prefix")
	}
	it, ok := p.checkDescription(it)
	if !ok {
		return it
	}
	p.resolveCategory(&it, v.get(c.category), CatTools)
	if q, present, err := parseQuantity(v.get(c.stock)); err != nil {
		it.flag(FlagSuspicious, "stock cell: %v; assets always have stock 1", err)
	} else if present && q != 1 {
		it.flag(FlagSuspicious, "stock cell says %d but assets always have stock 1", q)
	}
	it.rawQty = v.get(c.stock)

	state := Fold(v.get(c.state))
	switch state {
	case "funcional":
	case "inoperativo", "no funcional":
		it.Status = "inoperative"
		it.flag(FlagNoFailureRecord, "inoperative without a failure record; consider reporting a failure")
	case "lleno", "vacio":
		if strings.Contains(Fold(it.Description), "cilindro") {
			it.Attributes["nivel"] = state
		} else {
			it.Notes = joinNotes(it.Notes, "Excel Estado = "+strings.ToUpper(state))
			it.flag(FlagSuspicious, "Estado %s on something that is not a cylinder; imported as functional without a level (Excel Estado kept in notes)", strings.ToUpper(state))
		}
	case "":
		it.flag(FlagSuspicious, "no Estado in the workbook; imported as functional")
	default:
		it.flag(FlagSuspicious, "unknown Estado %q; imported as functional", v.get(c.state))
	}
	if strings.Contains(Fold(it.Description), "prestamo") {
		it.Notes = joinNotes(it.Notes, "Excel description says it was borrowed (prestamo)")
		it.flag(FlagSuspicious, "description mentions \"prestamo\" (possibly a borrowed tool); kept in notes, not recorded as a loan")
	}
	p.checkAvailability(&it, v.get(c.availability))
	if raw := v.get(c.lastMaint); raw != "" {
		p.resolveDate(&it, raw, "last maintenance")
	}
	return it
}

// checkAvailability compares the workbook's Disponibilidad with the status the
// importer derived. Availability itself is derived by the system.
func (p *parseCtx) checkAvailability(it *PlannedItem, raw string) {
	switch Fold(raw) {
	case "disponible":
		if it.Status != "functional" {
			it.flag(FlagInconsistency, "Excel says disponible but Estado is not functional")
		}
	case "no disponible":
		if it.Status == "functional" {
			it.flag(FlagInconsistency, "Excel says no disponible but Estado is functional (availability is derived; check for a loan or a missing failure)")
		}
	case "":
	default:
		it.flag(FlagSuspicious, "unknown Disponibilidad %q (not imported; the system derives availability)", raw)
	}
}

func joinNotes(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// ---- sheet "Maleta y caja de ..." ----

func (p *parseCtx) caseRow(v rowView, c columns, inConsumables bool) PlannedItem {
	a, b := v.get(c.code), v.get(c.desc)
	var it PlannedItem
	qtyRaw := v.get(c.stock)
	code, rest, isCode := SplitCode(a)
	switch {
	case isCode:
		desc := b
		fused := false
		if rest != "" && b == "" {
			desc, fused = rest, true
		}
		kind, cat := "asset", CatTools
		if inConsumables {
			kind, cat = "consumable", CatConsumables
		}
		it = p.base(desc, kind, cat)
		it.Code = code
		if fused {
			it.flag(FlagFused, "code and description were in one cell (%q); split into %s and %q", a, code, it.Description)
		} else if rest != "" {
			it.flag(FlagSuspicious, "code cell also contained text %q; the description cell was used", rest)
		}
	case a != "":
		// Columns shifted: description in A, optional category in B, quantity
		// in the first numeric cell after that (column E in the workbook).
		it = p.base(a, "consumable", CatConsumables)
		qtyRaw = ""
		for col := c.desc + 1; col < len(v.s.rows[v.r-1]); col++ {
			if _, present, err := parseQuantity(v.get(col)); present && err == nil {
				qtyRaw = v.get(col)
				break
			}
		}
		if cat := b; cat != "" {
			if name, ok := CanonicalCategory(cat); ok {
				it.Category = name
				if Fold(cat) != Fold(name) {
					it.flag(FlagCategoryNormalized, "%q normalized to %s", cat, name)
				}
			} else {
				it.flag(FlagSuspicious, "unknown category %q; defaulted to %s", cat, CatConsumables)
			}
		}
		it.flag(FlagNoCode, "no code; one will be generated from the category prefix")
	case b != "":
		it = p.base(b, "consumable", CatConsumables)
		it.flag(FlagNoCode, "no code; one will be generated from the category prefix")
		if !inConsumables {
			it.flag(FlagSuspicious, "uncoded row inside the tool block; treated as a consumable")
		}
	default:
		it = p.base("", "consumable", CatConsumables)
	}
	it.Location = LocationCase
	it.rawQty = qtyRaw
	it, ok := p.checkDescription(it)
	if !ok {
		return it
	}
	q, present, err := parseQuantity(qtyRaw)
	if err != nil {
		return p.invalid(it, "%v; skipped", err)
	}
	switch it.Kind {
	case "asset":
		it.Stock = 1
		if present && q != 1 {
			it.flag(FlagSuspicious, "quantity %d on a tool; assets always have stock 1", q)
		}
	default:
		it.Unit = "unidad"
		it.Stock = q
		if !present {
			it.flag(FlagSuspicious, "no quantity; imported with stock 0")
		}
	}
	return it
}

// ---- sheet "Inactivos y Reparación" ----

func (p *parseCtx) inactiveRow(v rowView, c columns) PlannedItem {
	it := p.base(v.get(c.desc), "asset", CatTools)
	it.Stock = 1
	p.defaultLocation(&it)
	raw := v.get(c.code)
	if raw != "" {
		code, rest, ok := SplitCode(raw)
		if !ok {
			return p.invalid(it, "code %q does not look like ISK-ABC-001; skipped", raw)
		}
		it.Code = code
		if rest != "" && it.Description == "" {
			it.Description = rest
			it.flag(FlagFused, "code and description were in one cell; split")
		}
	} else {
		it.flag(FlagNoCode, "no code; one will be generated from the category prefix")
	}
	it, ok := p.checkDescription(it)
	if !ok {
		return it
	}
	p.resolveCategory(&it, v.get(c.category), CatTools)

	dest := Clean(v.get(c.destination))
	it.Status = "inoperative"
	if strings.Contains(Fold(dest), "taller") {
		it.Status = "in_repair"
	}
	if st := Fold(v.get(c.state)); st != "no funcional" && st != "" {
		it.flag(FlagSuspicious, "unexpected Estado %q on the inactive sheet; treated as not functional", v.get(c.state))
	}
	f := &Failure{Destination: dest, ItemStatus: it.Status, Description: it.Description}
	holder := PlannedItem{}
	if rawDate := v.get(c.failDate); rawDate == "" {
		it.flag(FlagSuspicious, "no failure date; the maintenance record will use the import time")
	} else if p.resolveDate(&holder, rawDate, "failure date") {
		f.ReportedAt = holder.LastMaintenanceAt
		it.Flags = append(it.Flags, holder.Flags...)
	} else {
		it.Flags = append(it.Flags, holder.Flags...)
	}
	it.Failure = f
	return it
}
