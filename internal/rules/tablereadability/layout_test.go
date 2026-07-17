package tablereadability

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestTable_PointerFieldsPrecedeScalars and
// TestTableRow_PointerFieldsPrecedeScalars guard the GC ptrdata
// region of table/tableRow, per
// docs/development/high-performance-go.md#struct-layout. parseTables
// allocates one table (and one tableRow per source row) for every
// table MDS026 scans — the same per-row hot path the sibling
// tablefmt package already reordered for this reason (commit
// 9284744f, "GC scan reduction for ... table, row structs"); this
// package's local copy was never updated to match.
func TestTable_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(table{}))
}

func TestTableRow_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(tableRow{}))
}
