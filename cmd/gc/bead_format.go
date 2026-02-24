package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
)

// parseBeadFormat extracts --format/--json flags from raw args (needed because
// DisableFlagParsing is true). Returns the format ("text", "json", or "toon")
// and the remaining positional args with the flag removed.
func parseBeadFormat(args []string) (string, []string) {
	format := "text"
	var rest []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--format" && i+1 < len(args):
			format = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--format="):
			format = strings.TrimPrefix(args[i], "--format=")
		case args[i] == "--json":
			format = "json"
		default:
			rest = append(rest, args[i])
		}
	}
	return format, rest
}

// writeBeadJSON writes a single bead as indented JSON.
func writeBeadJSON(b beads.Bead, stdout io.Writer) {
	data, _ := json.MarshalIndent(b, "", "  ")
	fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
}

// writeBeadsJSON writes a slice of beads as a JSON array.
func writeBeadsJSON(bs []beads.Bead, stdout io.Writer) {
	data, _ := json.MarshalIndent(bs, "", "  ")
	fmt.Fprintln(stdout, string(data)) //nolint:errcheck // best-effort stdout
}

// writeBeadTOON writes a single bead in TOON (token-optimized) format,
// matching br's compact header+row style.
func writeBeadTOON(b beads.Bead, stdout io.Writer) {
	fmt.Fprintln(stdout, "[1]{id,title,status,type,created_at,assignee}:") //nolint:errcheck // best-effort stdout
	assignee := b.Assignee
	if assignee == "" {
		assignee = "\u2014"
	}
	fmt.Fprintf(stdout, "  %s,%s,%s,%s,%s,%s\n", //nolint:errcheck // best-effort stdout
		toonVal(b.ID), toonVal(b.Title), b.Status, b.Type,
		b.CreatedAt.Format(time.RFC3339), toonVal(assignee))
}

// writeBeadListTOON writes a bead list in TOON format with the given fields.
func writeBeadListTOON(bs []beads.Bead, fields string, rowFn func(beads.Bead) string, stdout io.Writer) {
	fmt.Fprintf(stdout, "[%d]{%s}:\n", len(bs), fields) //nolint:errcheck // best-effort stdout
	for _, b := range bs {
		fmt.Fprintf(stdout, "  %s\n", rowFn(b)) //nolint:errcheck // best-effort stdout
	}
}

// toonVal quotes a TOON value if it contains commas, quotes, or newlines.
func toonVal(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return s
}
