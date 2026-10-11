// Command import-xlsx reads the legacy inventory workbook and produces a
// review report (dry-run, the default) or imports it (--apply).
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	_ "time/tzdata" // zone database for minimal images

	"github.com/edgar/inventario/backend/internal/importer"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv)) }

type config struct {
	file, report, format, defaultLocation, timezone string
}

func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	var cfg config
	fs := flag.NewFlagSet("import-xlsx", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&cfg.file, "file", "", "path of the .xlsx workbook (required)")
	fs.StringVar(&cfg.report, "report", "", "write the report to this file instead of stdout")
	fs.StringVar(&cfg.format, "format", "md", "report format: md or json")
	fs.StringVar(&cfg.defaultLocation, "default-location", "Almacén principal", "location of the items of sheets 1 and 3 (empty: no location)")
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

	plan, err := importer.Parse(cfg.file, importer.Options{DefaultLocation: cfg.defaultLocation, Location: loc})
	if err != nil {
		fmt.Fprintf(stderr, "import-xlsx: %v\n", err)
		return 1
	}
	return emit(cfg, plan, nil, stdout, stderr)
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
