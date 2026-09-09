package types

const (
	// ModuleName defines the module name.
	ModuleName = "denommetadata"

	// StoreKey is intentionally NOT defined: this module owns no state.
	// It is a governance-gated writer into x/bank's metadata store, so giving
	// it a store key would create state nothing reads.
)
