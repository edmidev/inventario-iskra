// Command import-xlsx reads the legacy inventory workbook and produces a
// review report (dry-run, the default) or imports it (--apply).
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	_ "time/tzdata" // zone database for minimal images

	"github.com/edgar/inventario/backend/internal/importer"
	importerpg "github.com/edgar/inventario/backend/internal/importer/adapters/postgres"
	"github.com/edgar/inventario/backend/internal/platform/db"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv)) }

type config struct {
	file, report, format, defaultLocation, firstBlockLocation, repeatLocation, timezone, userEmail string
	apply                                                                                          bool
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	var cfg config
	fs := flag.NewFlagSet("import-xlsx", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.file, "file", "", "path of the .xlsx workbook (required)")
	fs.StringVar(&cfg.report, "report", "", "write the report to this file instead of stdout")
	fs.StringVar(&cfg.format, "format", "md", "report format: md or json")
	fs.StringVar(&cfg.defaultLocation, "default-location", importer.DefaultRepeatLocation, "location of the items of sheets 1 and 3: an assumption, the workbook does not say (empty: no location)")
	fs.StringVar(&cfg.firstBlockLocation, "first-block-location", importer.LocationCase, "location of the first block of sheet 2 (first occurrence of each consumable, and its tools)")
	fs.StringVar(&cfg.repeatLocation, "repeat-location", importer.DefaultRepeatLocation, "location of the repeated block of sheet 2 (second occurrence of each consumable)")
	fs.BoolVar(&cfg.apply, "apply", false, "write to the database (default is a dry-run that writes nothing)")
	fs.StringVar(&cfg.userEmail, "user-email", "", "email of the admin/operator the import acts as (required with --apply)")
	fs.StringVar(&cfg.timezone, "timezone", "", "IANA time zone for dates (default: the local zone)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if cfg.file == "" {
		fmt.Fprintln(stderr, "import-xlsx: --file is required")
		return 2
	}
	if cfg.format != "md" && cfg.format != "json" {
		fmt.Fprintf(stderr, "import-xlsx: unknown --format %q (use md or json)\n", cfg.format)
		return 2
	}
	loc := time.Local
	if cfg.timezone != "" {
		l, err := time.LoadLocation(cfg.timezone)
		if err != nil {
			fmt.Fprintf(stderr, "import-xlsx: unknown --timezone %q\n", cfg.timezone)
			return 2
		}
		loc = l
	}

	dbURL := getenv("DATABASE_URL")
	if cfg.apply && (cfg.userEmail == "" || dbURL == "") {
		fmt.Fprintln(stderr, "import-xlsx: --apply needs --user-email and DATABASE_URL")
		return 2
	}

	if importer.Fold(cfg.firstBlockLocation) == importer.Fold(cfg.repeatLocation) {
		fmt.Fprintln(stderr, "import-xlsx: --first-block-location and --repeat-location must differ")
		return 2
	}
	plan, err := importer.Parse(cfg.file, importer.Options{DefaultLocation: cfg.defaultLocation, FirstBlockLocation: cfg.firstBlockLocation, RepeatLocation: cfg.repeatLocation, Location: loc})
	if err != nil {
		fmt.Fprintf(stderr, "import-xlsx: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	var result *importer.Result
	if dbURL != "" {
		pool, err := db.NewPool(ctx, dbURL)
		if err != nil {
			fmt.Fprintf(stderr, "import-xlsx: database unavailable: %v\n", err)
			return 1
		}
		defer pool.Close()
		deps := importerpg.NewDeps(pool)
		if cfg.apply {
			actor, err := importerpg.ResolveActor(ctx, pool, cfg.userEmail)
			if err != nil {
				fmt.Fprintf(stderr, "import-xlsx: %v\n", err)
				return 1
			}
			result, err = importer.Apply(ctx, plan, deps, actor)
			if err != nil {
				fmt.Fprintf(stderr, "import-xlsx: apply failed: %v\n", err)
				return 1
			}
		} else if err := importer.Inspect(ctx, plan, deps); err != nil {
			fmt.Fprintf(stderr, "import-xlsx: %v (is the database migrated? start the API once)\n", err)
			return 1
		}
	}
	code := emit(cfg, plan, result, stdout, stderr)
	if result != nil {
		fmt.Fprintf(stderr, "applied: %d items created (%d assets, %d consumables, %d with two stock rows), %d stock rows added to existing items, %d already imported, %d conflicts, %d maintenance records opened, %d errors\n",
			result.ItemsCreated, result.AssetsCreated, result.ConsumablesCreated, result.SplitItems, result.StockRowsAdded, result.AlreadyImported, result.Conflicts, result.MaintenanceOpened, len(result.Errors))
		if len(result.Errors) > 0 && code == 0 {
			code = 1
		}
	}
	return code
}

// emit writes the report to --report or stdout.
func emit(cfg config, plan *importer.Plan, result *importer.Result, stdout, stderr io.Writer) int {
	out := stdout
	if cfg.report != "" {
		f, err := os.Create(cfg.report)
		if err != nil {
			fmt.Fprintf(stderr, "import-xlsx: create report: %v\n", err)
			return 1
		}
		defer f.Close()
		out = f
	}
	if err := importer.WriteReport(out, plan, result, importer.Format(cfg.format)); err != nil {
		fmt.Fprintf(stderr, "import-xlsx: write report: %v\n", err)
		return 1
	}
	if cfg.report != "" {
		fmt.Fprintf(stderr, "report written to %s\n", cfg.report)
	}
	return 0
}
