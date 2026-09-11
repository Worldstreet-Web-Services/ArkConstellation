package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	corevm "github.com/ethereum/go-ethereum/core/vm"
	"github.com/stretchr/testify/require"

	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// genesisFilesWithPrecompiles is every committed file carrying an
// active_static_precompiles array. Paths are relative to the repo root (this
// package sits one level down).
//
// Two shapes are represented - complete genesis documents (genesis-DRAFT,
// base-genesis) and merge-patch overlays (genesis-params, genesis-template, see
// RUNBOOK.md and networks/devnet/README.md) - but both nest module state under
// "app_state", so they read the same way here.
var genesisFilesWithPrecompiles = []string{
	"networks/mainnet/genesis-params.json",
	"networks/mainnet/genesis-DRAFT.json",
	"networks/devnet/genesis-template.json",
	"scripts/genesis/rehearsal/base-genesis.json",
}

// TestArkActiveStaticPrecompilesIsValid checks the two properties
// evmtypes.ValidatePrecompiles cares about, plus sortedness explicitly.
//
// Sortedness is asserted directly rather than left to ValidatePrecompiles because
// keeper.SetParams runs slices.Sort before validating, so an unsorted list is
// silently reordered at runtime instead of rejected. That would leave this slice
// disagreeing with what the chain actually stores - and with the JSON files
// checked below.
func TestArkActiveStaticPrecompilesIsValid(t *testing.T) {
	got := ArkActiveStaticPrecompiles()

	require.NoError(t, evmtypes.ValidatePrecompiles(got))
	require.True(t, slices.IsSorted(got), "must be sorted, got %v", got)

	for _, addr := range got {
		// IsAvailableStaticPrecompile compares against common.Address.String(),
		// which is EIP-55 checksummed. Every Ark precompile address happens to
		// checksum to its lowercase form, so an uppercased literal here would
		// make the precompile silently unreachable rather than fail loudly.
		require.Equal(t, strings.ToLower(addr), addr, "must be lowercase")
		require.Equal(t, addr, ethcommon.HexToAddress(addr).String(),
			"must equal its EIP-55 checksum form, or activation will not match")
	}
}

// TestArkActiveStaticPrecompilesAllRegistered is the important one: every address
// Ark activates must actually be present in the EVM keeper's compiled-in static
// precompile map.
//
// When it is not, GetStaticPrecompileInstance panics with "precompiled contract
// not stored in memory". Nothing catches that earlier - ValidatePrecompiles only
// checks hex/uniqueness/order, and the evm module's InitGenesis just calls
// SetParams - so an unimplemented address passes genesis validation and boots
// fine, then panics the first time anyone calls it, including via an
// unauthenticated eth_call.
//
// This is not hypothetical: evmtypes.AvailableStaticPrecompiles contains
// VestingPrecompileAddress, which the fork has no implementation for. See
// TestVestingPrecompileIsNotRegistered below and app/precompiles.go.
func TestArkActiveStaticPrecompilesAllRegistered(t *testing.T) {
	// Matches the existing pattern in app/mempool_test.go: booting the app
	// dereferences the package-level EVM chain config, which is nil until this
	// is called. Safe to call from more than one test in this package - see
	// evmtypes.SetChainConfig's guard, which is idempotent for a nil argument.
	require.NoError(t, evmtypes.SetChainConfig(nil))
	app := SetupWithEmptyStore(t)

	params := evmtypes.DefaultParams()
	params.ActiveStaticPrecompiles = ArkActiveStaticPrecompiles()

	for _, addr := range ArkActiveStaticPrecompiles() {
		t.Run(addr, func(t *testing.T) {
			hexAddr := ethcommon.HexToAddress(addr)

			// The lookup itself is the thing under test for panics, so it runs
			// outside require.NotPanics's closure: require.* calls runtime.Goexit
			// on failure, which unwinds the current goroutine rather than
			// panicking, so nesting a failing require inside NotPanics would not
			// report the way it looks like it should.
			var precompile corevm.PrecompiledContract
			var found bool
			var err error
			require.NotPanics(t, func() {
				precompile, found, err = app.EVMKeeper.GetStaticPrecompileInstance(&params, hexAddr)
			})
			require.NoError(t, err)
			require.True(t, found, "not registered in the keeper")
			require.NotNil(t, precompile)
		})
	}
}

// TestVestingPrecompileIsNotRegistered pins the reason 0x...0803 is excluded from
// ArkActiveStaticPrecompiles, so that removing the exclusion fails here with an
// explanation rather than shipping a remotely triggerable panic.
//
// If this test ever starts failing because the address IS registered, the fork has
// gained a vesting precompile: add it back to ArkActiveStaticPrecompiles (in
// sorted position, between ICS20 and Bank) and delete this test.
func TestVestingPrecompileIsNotRegistered(t *testing.T) {
	require.NotContains(t, ArkActiveStaticPrecompiles(), evmtypes.VestingPrecompileAddress,
		"0x...0803 has no implementation in the fork - activating it panics on call")
	require.Contains(t, evmtypes.AvailableStaticPrecompiles, evmtypes.VestingPrecompileAddress,
		"upstream still advertises an address it does not implement; if this changed, "+
			"app/precompiles.go's warning can be simplified")

	require.NoError(t, evmtypes.SetChainConfig(nil))
	app := SetupWithEmptyStore(t)

	// Activating it is what turns a dangling constant into a panic.
	params := evmtypes.DefaultParams()
	params.ActiveStaticPrecompiles = []string{evmtypes.VestingPrecompileAddress}

	require.Panics(t, func() {
		//nolint:errcheck // the panic is the assertion
		app.EVMKeeper.GetStaticPrecompileInstance(
			&params, ethcommon.HexToAddress(evmtypes.VestingPrecompileAddress),
		)
	}, "expected the keeper to panic on an active-but-unregistered precompile")
}

// TestGenesisFilesMatchActiveStaticPrecompiles stops the genesis JSON from
// drifting away from the binary.
//
// This drift is exactly what shipped: App.DefaultGenesis() populated the list,
// but `arkd init` never calls it (it goes through genutilcli.InitCmd with the SDK
// BasicManager), so the tests ran with precompiles active while every committed
// genesis file had an empty array. Asserting against the files from Go catches
// that, which neither `genesis validate-genesis` nor the CI genesis jobs do.
func TestGenesisFilesMatchActiveStaticPrecompiles(t *testing.T) {
	want := ArkActiveStaticPrecompiles()

	for _, relPath := range genesisFilesWithPrecompiles {
		t.Run(relPath, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", relPath))
			require.NoError(t, err)

			var doc map[string]any
			require.NoError(t, json.Unmarshal(raw, &doc))

			appState, ok := doc["app_state"].(map[string]any)
			require.True(t, ok, "no app_state object")

			evm, ok := appState["evm"].(map[string]any)
			require.True(t, ok, "no evm object")
			params, ok := evm["params"].(map[string]any)
			require.True(t, ok, "no evm.params object")

			raw2, ok := params["active_static_precompiles"]
			require.True(t, ok, "evm.params.active_static_precompiles is missing")
			list, ok := raw2.([]any)
			require.True(t, ok, "active_static_precompiles is not an array")

			got := make([]string, 0, len(list))
			for _, v := range list {
				s, ok := v.(string)
				require.True(t, ok, "non-string entry %v", v)
				got = append(got, s)
			}

			require.Equal(t, want, got,
				"out of sync with ArkActiveStaticPrecompiles() - update this file")
		})
	}
}
