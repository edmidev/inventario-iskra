package importer

import (
	"strings"
	"testing"
	"time"
)

func TestAssetsSheetHeaderOffsetAndCategoryNoise(t *testing.T) {
	p := parseSynthetic(t, Options{DefaultLocation: "Almacén principal"}, assetsSheet(
		[]any{nil, "ISK-AAA-001", "  taladro   verde ", "maquinaria/", "funcional", "disponible ", nil, nil},
		[]any{nil, "isk-bbb-002", "martillo", "Herramientas ", "funcional", "disponible"},
	))
	if len(p.Items) != 2 || p.Sheets[0].HeaderRow != 1 || p.Sheets[0].RowsRead != 2 {
		t.Fatalf("sheets=%+v items=%d", p.Sheets, len(p.Items))
	}
	a := byRow(p, sheetAssets, 3)
	if a.Code != "ISK-AAA-001" || a.Description != "taladro verde" || a.Category != CatMachinery || a.Kind != "asset" || a.Stock != 1 {
		t.Fatalf("row 3 = %+v", a)
	}
	if a.Location != "Almacén principal" || !hasFlag(a, FlagCategoryNormalized) {
		t.Fatalf("location/flags = %q %v", a.Location, a.Flags)
	}
	b := byRow(p, sheetAssets, 4)
	if b.Code != "ISK-BBB-002" || b.Category != CatTools || hasFlag(b, FlagCategoryNormalized) {
		t.Fatalf("row 4 = %+v", b)
	}
	if a.Attributes["import_source"] != "xlsx:Activos y Disponibles:r3" {
		t.Fatalf("import_source = %v", a.Attributes["import_source"])
	}
}

func TestEmptyDefaultLocationMeansNoLocation(t *testing.T) {
	p := parseSynthetic(t, Options{DefaultLocation: ""}, assetsSheet(
		[]any{nil, "ISK-AAA-001", "x", "herramientas", "funcional", "disponible"}))
	if p.Items[0].Location != "" || len(p.Locations) != 0 {
		t.Fatalf("location = %q, locations = %+v", p.Items[0].Location, p.Locations)
	}
}

func TestStatusMappingAndCylinderLevel(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet(
		[]any{nil, "ISK-A-001", "Cilindro argon", "maquinaria", "LLENO", "disponible"},
		[]any{nil, "ISK-A-002", "Cilindro argon", "maquinaria", "VACIO", "disponible"},
		[]any{nil, "ISK-A-003", "taladro de banco alto", "maquinaria/", "VACIO", "disponible"},
		[]any{nil, "ISK-A-004", "cortadora", "maquinaria", "inoperativo", "no disponible"},
		[]any{nil, "ISK-A-005", "algo", "maquinaria", "roto?", "disponible"},
	))
	full, empty, drill, broken, odd := p.Items[0], p.Items[1], p.Items[2], p.Items[3], p.Items[4]
	if full.Status != "functional" || full.Attributes["nivel"] != "lleno" || empty.Attributes["nivel"] != "vacio" {
		t.Fatalf("cylinders: %+v %+v", full.Attributes, empty.Attributes)
	}
	if _, has := drill.Attributes["nivel"]; has || drill.Status != "functional" || !hasFlag(drill, FlagSuspicious) || !strings.Contains(drill.Notes, "VACIO") {
		t.Fatalf("suspicious drill = %+v", drill)
	}
	if broken.Status != "inoperative" || !hasFlag(broken, FlagNoFailureRecord) || broken.Failure != nil || hasFlag(broken, FlagInconsistency) {
		t.Fatalf("broken = %+v", broken)
	}
	if odd.Status != "functional" || !hasFlag(odd, FlagSuspicious) {
		t.Fatalf("unknown estado = %+v", odd)
	}
}

func TestAvailabilityInconsistenciesAndBorrowedNote(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet(
		[]any{nil, "ISK-A-001", "uno", "herramientas", "funcional", "no disponible "},
		[]any{nil, "ISK-A-002", "dos", "herramientas", "inoperativo", "disponible"},
		[]any{nil, "ISK-A-003", "taladro electrico verde(prestamo)", "maquinaria", "funcional", "disponible"},
		[]any{nil, "ISK-A-004", "cuatro", "herramientas", "funcional", "disponible"},
	))
	if !hasFlag(p.Items[0], FlagInconsistency) || !hasFlag(p.Items[1], FlagInconsistency) {
		t.Fatalf("inconsistencies not flagged: %+v %+v", p.Items[0].Flags, p.Items[1].Flags)
	}
	if hasFlag(p.Items[3], FlagInconsistency) {
		t.Fatal("consistent row flagged")
	}
	if !strings.Contains(p.Items[2].Notes, "prestamo") || !hasFlag(p.Items[2], FlagSuspicious) {
		t.Fatalf("borrowed = %+v", p.Items[2])
	}
}

func TestLastMaintenanceDatesFromSerialRealDateAndText(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet(
		[]any{nil, "ISK-A-001", "serial", "herramientas", "funcional", "disponible", nil, 46283},
		[]any{nil, "ISK-A-002", "real", "herramientas", "funcional", "disponible", nil, time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
		[]any{nil, "ISK-A-003", "text", "herramientas", "funcional", "disponible", nil, "05/06/2026"},
		[]any{nil, "ISK-A-004", "junk", "herramientas", "funcional", "disponible", nil, "pronto"},
	))
	want := []string{"2026-09-18", "2026-03-04", "2026-06-05", ""}
	for i, w := range want {
		got := ""
		if lm := p.Items[i].LastMaintenanceAt; lm != nil {
			got = lm.Format("2006-01-02")
		}
		if got != w {
			t.Errorf("row %d last maintenance = %q, want %q", i, got, w)
		}
	}
	if !hasFlag(p.Items[0], FlagDateConverted) || !hasFlag(p.Items[2], FlagDateConverted) || !hasFlag(p.Items[3], FlagSuspicious) {
		t.Fatalf("flags: %v / %v / %v", p.Items[0].Flags, p.Items[2].Flags, p.Items[3].Flags)
	}
}

func TestDatesUseTheConfiguredZone(t *testing.T) {
	bogota := time.FixedZone("UTC-5", -5*3600)
	p := parseSynthetic(t, Options{Location: bogota}, assetsSheet(
		[]any{nil, "ISK-A-001", "serial", "herramientas", "funcional", "disponible", nil, 46283}))
	got := *p.Items[0].LastMaintenanceAt
	if got.Hour() != 0 || got.UTC().Hour() != 5 || p.Timezone != "UTC-5" {
		t.Fatalf("got %v tz %q", got, p.Timezone)
	}
}

func TestCaseSheetToolsFusedCodeAndShiftedConsumables(t *testing.T) {
	p := parseSynthetic(t, Options{DefaultLocation: "Almacén principal"}, caseSheet(
		[]any{"ISK-M1-LLTU", "llave de tubo ", nil, nil, nil, 1},                                       // 3 tool
		[]any{nil, "repuestos de segueta", nil, nil, nil, 3},                                           // 4 uncoded, tool block
		[]any{"ISK-M16-PAR                    pinza para anillos de retencion", nil, nil, nil, nil, 1}, // 5 fused
		[]any{"ISK-M35-DESP", "destornillador pequeno", nil, nil, nil, nil},                            // 6 tool, no stock
		nil,                        // 7
		[]any{nil, "CONSUMIBLES "}, // 8 marker
		[]any{"ISK-MD1-PT", "PASTA TERMICA", nil, nil, nil, 2},     // 9 coded consumable
		[]any{"capuchones amarillos", "electricidad", nil, nil, 6}, // 10 shifted
		[]any{"terminales azul", nil, nil, nil, 74},                // 11 shifted, no category
	))
	if p.Sheets[0].HeaderRow != 2 || p.Sheets[0].RowsRead != 7 {
		t.Fatalf("summary = %+v", p.Sheets[0])
	}
	tool := byRow(p, sheetCase, 3)
	if tool.Kind != "asset" || tool.Category != CatTools || tool.Stock != 1 || tool.Location != LocationCase || tool.Description != "llave de tubo" {
		t.Fatalf("tool = %+v", tool)
	}
	spare := byRow(p, sheetCase, 4)
	if spare.Kind != "consumable" || spare.Stock != 3 || spare.Code != "" || spare.Unit != "unidad" || !hasFlag(spare, FlagNoCode) {
		t.Fatalf("spare = %+v", spare)
	}
	fused := byRow(p, sheetCase, 5)
	if fused.Code != "ISK-M16-PAR" || fused.Description != "pinza para anillos de retencion" || !hasFlag(fused, FlagFused) || fused.Kind != "asset" {
		t.Fatalf("fused = %+v", fused)
	}
	if nostock := byRow(p, sheetCase, 6); nostock.Kind != "asset" || nostock.Stock != 1 || len(nostock.Flags) != 0 {
		t.Fatalf("no stock tool = %+v", nostock)
	}
	if c := byRow(p, sheetCase, 9); c.Kind != "consumable" || c.Code != "ISK-MD1-PT" || c.Stock != 2 || c.Category != CatConsumables {
		t.Fatalf("coded consumable = %+v", c)
	}
	cap := byRow(p, sheetCase, 10)
	if cap.Kind != "consumable" || cap.Category != CatElectrical || cap.Stock != 6 || cap.Code != "" || cap.Location != LocationCase {
		t.Fatalf("shifted = %+v", cap)
	}
	if t11 := byRow(p, sheetCase, 11); t11.Category != CatConsumables || t11.Stock != 74 {
		t.Fatalf("shifted no category = %+v", t11)
	}
}

func TestInvalidQuantityAndMissingDescriptionAreSkippedNotGuessed(t *testing.T) {
	p := parseSynthetic(t, Options{}, caseSheet(
		[]any{"ISK-MD1-X", "algo", nil, nil, nil, "mucho"},
		[]any{"ISK-MD2-X", "otro", nil, nil, nil, 1.5},
		[]any{nil, nil, "huerfana", nil, nil, 4},
		[]any{nil, "CONSUMIBLES"},
	))
	for _, row := range []int{3, 4, 5} {
		if it := byRow(p, sheetCase, row); it.Action != ActionSkipInvalid || !hasFlag(it, FlagInvalid) {
			t.Errorf("row %d = %+v, want skip_invalid", row, it)
		}
	}
}

// twoBlocks builds a sheet 2 with a first block, a middle row and a repeat.
func consumableRows(first, repeat [][]any) [][]any {
	rows := [][]any{{nil, "CONSUMIBLES"}}
	rows = append(rows, first...)
	return append(rows, repeat...)
}

func stockOf(it PlannedItem, loc string) int {
	q, _ := it.stockAt(loc)
	return q
}

func TestRepeatedBlockBecomesASecondStockRowAtTheRepeatLocation(t *testing.T) {
	// header r2, CONSUMIBLES r3; first block r4-r8 (r7 blank); repeat r9-r13.
	block := func(qtyAmarras, qtyTerminales int) [][]any {
		return [][]any{
			{"ISK-MD7-APT", "AMARRAS TRASPARENTE ", nil, nil, nil, qtyAmarras},
			{"capuchones amarillos", "electricidad", nil, nil, 6},
			nil,
			{"terminales azul", nil, nil, nil, qtyTerminales},
			{"ISK-MD8-CG", "CINTA GRIS", nil, nil, nil, 1},
		}
	}
	first := append([][]any{{"ISK-MD1-PT", "PASTA", nil, nil, nil, 2}}, block(100, 74)...)
	repeat := block(100, 70) // same item, a different quantity in the workshop
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows(first, repeat)...))

	amarras := byRow(p, sheetCase, 5)
	if amarras.Action != ActionCreate || len(amarras.Stocks) != 2 || stockOf(amarras, LocationCase) != 100 || stockOf(amarras, "Taller") != 100 || amarras.Stock != 200 {
		t.Fatalf("amarras = %+v", amarras)
	}
	if amarras.Attributes["import_source"] != "xlsx:"+sheetCase+":r5" || amarras.Attributes["import_source_repeat"] != "xlsx:"+sheetCase+":r10" {
		t.Fatalf("provenance = %v", amarras.Attributes)
	}
	rep := byRow(p, sheetCase, 10)
	if rep.Action != ActionStockRow || rep.MergedInto != amarras.Ref() {
		t.Fatalf("repeat row = %+v", rep)
	}
	var terminales PlannedItem
	for _, it := range p.Items {
		if it.Description == "terminales azul" && it.Action == ActionCreate {
			terminales = it
		}
	}
	if stockOf(terminales, LocationCase) != 74 || stockOf(terminales, "Taller") != 70 || terminales.Stock != 144 || terminales.Code != "" {
		t.Fatalf("terminales (different quantities) = %+v", terminales)
	}
	// Only the first block item has one row; nothing is skipped as duplicate.
	pasta := byRow(p, sheetCase, 4)
	if len(pasta.Stocks) != 1 || pasta.Stocks[0].Location != LocationCase {
		t.Fatalf("pasta = %+v", pasta)
	}
	if p.Count(ActionStockRow) != 4 || p.Count(ActionCreate) != 5 || p.Count(ActionSkipConflict) != 0 {
		t.Fatalf("stock rows=%d create=%d conflicts=%d", p.Count(ActionStockRow), p.Count(ActionCreate), p.Count(ActionSkipConflict))
	}
	st := p.splitStats()
	if st.items != 5 || st.two != 4 || st.onlyFirst != 1 || st.onlySecond != 0 || st.extra != 0 {
		t.Fatalf("stats = %+v", st)
	}
	if len(p.Locations) != 2 || p.Locations[0].Name != LocationCase || p.Locations[0].Kind != "container" || p.Locations[1].Name != "Taller" || p.Locations[1].Kind != "storage" {
		t.Fatalf("locations = %+v", p.Locations)
	}
}

func TestItemOnlyInTheRepeatedBlockGetsASingleRowAtTheRepeatLocation(t *testing.T) {
	first := [][]any{{"ISK-MD1-A", "uno", nil, nil, nil, 1}, {"dos", "electricidad", nil, nil, 2}}
	repeat := [][]any{{"ISK-MD1-A", "uno", nil, nil, nil, 5}, {"solo taller", nil, nil, nil, 9}}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows(first, repeat)...))
	only := byRow(p, sheetCase, 7)
	if only.Action != ActionCreate || len(only.Stocks) != 1 || only.Stocks[0].Location != "Taller" || only.Stock != 9 || !hasFlag(only, FlagStockSplit) {
		t.Fatalf("second-only = %+v", only)
	}
	if two := byRow(p, sheetCase, 4); stockOf(two, LocationCase) != 1 || stockOf(two, "Taller") != 5 {
		t.Fatalf("two rows = %+v", two)
	}
	if byRow(p, sheetCase, 5).Stocks[0].Location != LocationCase {
		t.Fatal("only-first item must stay in the first location")
	}
	if st := p.splitStats(); st.two != 1 || st.onlyFirst != 1 || st.onlySecond != 1 {
		t.Fatalf("stats = %+v", st)
	}
}

func TestThirdOccurrenceIsAnIssueAndNeverMergedSilently(t *testing.T) {
	row := []any{"ISK-MD1-A", "uno", nil, nil, nil, 1}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows([][]any{row}, [][]any{row, row})...))
	third := byRow(p, sheetCase, 6)
	if third.Action != ActionSkipConflict || !hasFlag(third, FlagExtraOccurrence) {
		t.Fatalf("third = %+v", third)
	}
	if o := byRow(p, sheetCase, 4); o.Stock != 2 || len(o.Stocks) != 2 {
		t.Fatalf("owner must keep only two stock rows: %+v", o)
	}
	if st := p.splitStats(); st.extra != 1 {
		t.Fatalf("stats = %+v", st)
	}
	// An uncoded item listed three times is reported the same way.
	u := []any{"cables", "electricidad", nil, nil, 3}
	p = parseSynthetic(t, Options{}, caseSheet(consumableRows([][]any{u}, [][]any{u, u})...))
	if !hasFlag(byRow(p, sheetCase, 6), FlagExtraOccurrence) {
		t.Fatal("uncoded third occurrence not reported")
	}
}

func TestRepeatedCodeWithADifferentDescriptionIsAConflictNotAStockRow(t *testing.T) {
	first := [][]any{{"ISK-MD1-A", "uno", nil, nil, nil, 1}}
	repeat := [][]any{{"ISK-MD1-A", "otra cosa", nil, nil, nil, 4}}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows(first, repeat)...))
	c := byRow(p, sheetCase, 5)
	if c.Action != ActionSkipConflict || !hasFlag(c, FlagConflict) || p.Count(ActionStockRow) != 0 {
		t.Fatalf("conflict row = %+v", c)
	}
	if o := byRow(p, sheetCase, 4); len(o.Stocks) != 1 {
		t.Fatalf("owner = %+v", o)
	}
}

func TestUncodedItemsMatchByNormalizedDescriptionAndGenerateOneCode(t *testing.T) {
	first := [][]any{{"Terminales  AZUL", nil, nil, nil, 7}}
	repeat := [][]any{{"terminales azúl ", nil, nil, nil, 8}}
	p := parseSynthetic(t, Options{}, caseSheet(consumableRows(first, repeat)...))
	o := byRow(p, sheetCase, 4)
	if o.Code != "" || len(o.Stocks) != 2 || o.Stock != 15 || p.Count(ActionStockRow) != 1 {
		t.Fatalf("owner = %+v", o)
	}
	if got := p.Count(ActionCreate); got != 1 {
		t.Fatalf("items to create = %d: a code is generated once per item, so one row is created", got)
	}
}

func TestLocationOptionsAndDefaults(t *testing.T) {
	first := [][]any{{"cables", "electricidad", nil, nil, 3}}
	p := parseSynthetic(t, Options{FirstBlockLocation: "Maleta", RepeatLocation: "Bodega"}, caseSheet(consumableRows(first, first)...))
	if o := byRow(p, sheetCase, 4); stockOf(o, "Maleta") != 3 || stockOf(o, "Bodega") != 3 {
		t.Fatalf("owner = %+v", o)
	}
	if p.Locations[0].Name != "Bodega" || p.Locations[0].Kind != "storage" || p.Locations[1].Name != "Maleta" || p.Locations[1].Kind != "container" {
		t.Fatalf("locations = %+v", p.Locations)
	}
	d := parseSynthetic(t, Options{}, caseSheet(consumableRows(first, first)...))
	if d.FirstBlockLocation != LocationCase || d.RepeatLocation != "Taller" {
		t.Fatalf("defaults = %q %q", d.FirstBlockLocation, d.RepeatLocation)
	}
	if _, err := Parse(writeWorkbook(t, caseSheet(consumableRows(first, first)...)), Options{FirstBlockLocation: "Taller", RepeatLocation: " taller "}); err == nil {
		t.Fatal("equal locations must be rejected")
	}
}

func TestWorkshopIsSharedBetweenTheRepeatAndTheDefaultLocation(t *testing.T) {
	first := [][]any{{"cables", "electricidad", nil, nil, 3}}
	p := parseSynthetic(t, Options{DefaultLocation: "Taller"},
		assetsSheet([]any{nil, "ISK-A-001", "uno", "herramientas", "funcional", "disponible"}),
		caseSheet(consumableRows(first, first)...))
	n := 0
	for _, l := range p.Locations {
		if l.Name == "Taller" {
			n++
		}
	}
	if n != 1 || len(p.Locations) != 2 {
		t.Fatalf("locations = %+v", p.Locations)
	}
}

func TestSameConsumableRepeatedInsideTheFirstBlockStartsTheRepeat(t *testing.T) {
	// Documented heuristic: the first repeating row starts the repeated block.
	rows := consumableRows([][]any{{"a", "electricidad", nil, nil, 1}, {"a", "electricidad", nil, nil, 2}, {"b", nil, nil, nil, 3}}, nil)
	p := parseSynthetic(t, Options{}, caseSheet(rows...))
	if a := byRow(p, sheetCase, 4); stockOf(a, "Taller") != 2 {
		t.Fatalf("a = %+v", a)
	}
	if b := byRow(p, sheetCase, 6); b.Stocks[0].Location != "Taller" {
		t.Fatalf("b (after the repeat starts) = %+v", b)
	}
}

func TestSameCodeWithDifferentContentIsAConflictNotADuplicate(t *testing.T) {
	p := parseSynthetic(t, Options{}, assetsSheet(
		[]any{nil, "ISK-A-001", "primero", "herramientas", "funcional", "disponible"},
		[]any{nil, "ISK-A-001", "segundo", "herramientas", "funcional", "disponible"},
	), inactiveSheet(
		[]any{"ISK-A-001", "tercero", "Herramientas", "No Funcional", "28/08/2026", "Taller"},
	))
	if p.Items[0].Action != ActionCreate || p.Items[1].Action != ActionSkipConflict || p.Items[2].Action != ActionSkipConflict {
		t.Fatalf("actions: %s %s %s", p.Items[0].Action, p.Items[1].Action, p.Items[2].Action)
	}
	if !hasFlag(p.Items[1], FlagConflict) || !strings.Contains(p.Items[1].Flags[0].Message, "ISK-A-001") {
		t.Fatalf("flags = %+v", p.Items[1].Flags)
	}
}

func TestInactiveSheetBuildsOpenFailureRecords(t *testing.T) {
	p := parseSynthetic(t, Options{DefaultLocation: "Almacén principal"}, inactiveSheet(
		[]any{"ISK-HER-013", "Llave de pipa (Fisura)", "Herramientas", "No Funcional", "28/08/2026", "Taller de Reparación Interno"},
		[]any{"ISK-HER-014", "Pinza de presión", "Herramientas", "No Funcional", "02/09/2026", "Espera de Repuestos"},
		[]any{"ISK-HER-015", "Sin fecha", "Herramientas", "No Funcional", nil, "TALLER externo"},
	))
	a, b, c := p.Items[0], p.Items[1], p.Items[2]
	if a.Status != "in_repair" || a.Failure == nil || a.Failure.ItemStatus != "in_repair" || a.Failure.Destination != "Taller de Reparación Interno" {
		t.Fatalf("a = %+v %+v", a, a.Failure)
	}
	if got := a.Failure.ReportedAt.Format("2006-01-02 15:04"); got != "2026-08-28 00:00" {
		t.Fatalf("reported_at = %s", got)
	}
	if b.Status != "inoperative" || b.Failure.ItemStatus != "inoperative" || b.Failure.ReportedAt.Format("2006-01-02") != "2026-09-02" {
		t.Fatalf("b = %+v %+v", b, b.Failure)
	}
	if c.Status != "in_repair" || c.Failure.ReportedAt != nil || !hasFlag(c, FlagSuspicious) {
		t.Fatalf("c = %+v", c)
	}
	if !hasFlag(a, FlagDateConverted) || a.Location != "Almacén principal" || a.Failure.Description != "Llave de pipa (Fisura)" {
		t.Fatalf("a flags/location = %v %q", a.Flags, a.Location)
	}
}

func TestPlanNeedsAndGeneratedCodeRows(t *testing.T) {
	p := parseSynthetic(t, Options{DefaultLocation: "Almacén principal"}, assetsSheet(
		[]any{nil, "ISK-A-001", "uno", "maquinaria", "funcional", "disponible"},
		[]any{nil, nil, "sin codigo", "herramientas", "funcional", "disponible"},
	), caseSheet(
		[]any{nil, "CONSUMIBLES"},
		[]any{"cables", "electricidad", nil, nil, 3},
	))
	var cats []string
	for _, c := range p.Categories {
		cats = append(cats, c.Name+":"+c.Prefix)
	}
	if strings.Join(cats, ",") != "Maquinaria:MAQ,Herramientas:HER,Electricidad:ELE" {
		t.Fatalf("categories = %v", cats)
	}
	if len(p.Locations) != 2 || p.Locations[0].Name != "Almacén principal" || p.Locations[1].Kind != "container" {
		t.Fatalf("locations = %+v", p.Locations)
	}
	if p.Items[1].Code != "" || !hasFlag(p.Items[1], FlagNoCode) {
		t.Fatalf("uncoded = %+v", p.Items[1])
	}
}

func TestSheetRolesAreRecognizedLooselyAndMissingSheetsWarn(t *testing.T) {
	p := parseSynthetic(t, Options{}, sheetSpec{"ACTIVOS", [][]any{
		{"codigo", "DESCRIPCION", "categoria", "ESTADO"},
		{"ISK-A-001", "algo", "herramientas", "funcional"},
	}}, sheetSpec{"Notas", [][]any{{"hola"}}})
	if len(p.Items) != 1 || p.Items[0].Code != "ISK-A-001" {
		t.Fatalf("items = %+v", p.Items)
	}
	if len(p.Warnings) < 2 {
		t.Fatalf("warnings = %v", p.Warnings)
	}
}

func TestParseFailsWithoutKnownSheetsOrHeader(t *testing.T) {
	if _, err := Parse(writeWorkbook(t, sheetSpec{"Otra", [][]any{{"x"}}}), Options{}); err == nil {
		t.Fatal("expected an error for a workbook without known sheets")
	}
	if _, err := Parse(writeWorkbook(t, sheetSpec{sheetAssets, [][]any{{"foo", "bar"}}}), Options{}); err == nil {
		t.Fatal("expected an error for a sheet without a header")
	}
	if _, err := Parse("/nonexistent/file.xlsx", Options{}); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestSerialDateConversion(t *testing.T) {
	got, _, ok := parseDate("46283", time.UTC)
	if !ok || got.Format("2006-01-02") != "2026-09-18" {
		t.Fatalf("got %v %v", got, ok)
	}
}
