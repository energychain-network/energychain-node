package keeper

import (
	"fmt"
	"strconv"
	"strings"
)

// streamIDPrefix is the canonical prefix for sequence-derived stream
// authorisation ids. Format: "stream/<n>" where n is the sequence value.
// The format keeps ids globally sortable (lexically when the seq is
// padded by collections under the hood) and trivially debuggable.
const streamIDPrefix = "stream/"

// formatStreamID serialises the next-sequence value into the canonical
// id form.
func formatStreamID(seq uint64) string {
	return fmt.Sprintf("%s%d", streamIDPrefix, seq)
}

// parseStreamSeq is the inverse: returns 0 when the id was not minted by
// formatStreamID (e.g. a genesis import using a custom naming scheme).
func parseStreamSeq(id string) uint64 {
	if !strings.HasPrefix(id, streamIDPrefix) {
		return 0
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(id, streamIDPrefix), 10, 64)
	if err != nil {
		return 0
	}
	return n
}
