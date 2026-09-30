package cmd

import "github.com/spf13/cobra"

// Exit-code class symbols for the kit/exit-codes annotation. Each leaf
// declares the classes it can produce; `aim spec` projects the CSV into
// the manifest's exit_codes field so agents can branch on process exit
// without parsing stderr. Numeric mapping (shared taxonomy): OK=0,
// GENERIC=1, USAGE=2, NOT_FOUND=3, CONFLICT=4, TRANSIENT=6.
//
// Every leaf can produce USAGE (flag validation, --api-version guard),
// CONFLICT (cache lock) and TRANSIENT (network / upstream) via the
// shared cache + refresh path; NOT_FOUND is added only where a lookup
// can miss.
const (
	// exitCodesRead covers the cache-backed read leaves (list,
	// providers, query) and refresh.
	exitCodesRead = "OK,GENERIC,USAGE,CONFLICT,TRANSIENT"
	// exitCodesLookup extends the read set with NOT_FOUND for leaves
	// that resolve a caller-supplied identifier (show).
	exitCodesLookup = "OK,GENERIC,USAGE,NOT_FOUND,CONFLICT,TRANSIENT"
)

// setExitCodes stamps the kit/exit-codes annotation on a leaf so the
// spec manifest publishes its exit-code contract.
func setExitCodes(cmd *cobra.Command, symbols string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations["kit/exit-codes"] = symbols
}
