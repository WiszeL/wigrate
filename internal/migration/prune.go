package migration

import (
	"fmt"
	"os"

	"github.com/wiszel/wigrate/internal/schema"
)

// pruneForeignKeys drops FKs whose referenced table isn't in the same module —
// schema.Parse derives RefTable from field name (or ref:) alone, with no
// existence check. A drop is likely either a typo or a cross-module reference,
// so it's surfaced as a warning rather than silently ignored.
func pruneForeignKeys(fks []schema.ForeignKey, moduleTables map[string]struct{}, moduleName string) []schema.ForeignKey {
	kept := make([]schema.ForeignKey, 0, len(fks))
	for _, fk := range fks {
		if _, ok := moduleTables[fk.RefTable]; !ok {
			fmt.Fprintf(os.Stderr, "warning: column %q references table %q not found in module %s — foreign key skipped. Is this intended?\n", fk.Column, fk.RefTable, moduleName)
			continue
		}
		kept = append(kept, fk)
	}
	return kept
}
