package app_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"github.com/MANTRA-Chain/mantrachain/v8/app"
	"github.com/MANTRA-Chain/mantrachain/v8/app/precompiles/distrclaim"
)

// Every file in the repo that hardcodes active_static_precompiles, with the
// JSON path to the list inside it.
var precompileFixtures = map[string][]any{
	"networks/devnet/genesis-template.json":                      {"app_state", "evm", "params", "active_static_precompiles"},
	"networks/devnet/pystarport.json":                            {"arkdevnet_9000-1", "genesis", "app_state", "evm", "params", "active_static_precompiles"},
	"networks/devnet/proposals/activate-static-precompiles.json": {"messages", 0, "params", "active_static_precompiles"},
	"networks/mainnet/genesis-DRAFT.json":                        {"app_state", "evm", "params", "active_static_precompiles"},
	"scripts/genesis/rehearsal/base-genesis.json":                {"app_state", "evm", "params", "active_static_precompiles"},
}

// StaticPrecompileAddresses must be exactly the set registered with the EVM
// keeper: an activated-but-unregistered address panics on first call, and a
// registered-but-unactivated one silently no-ops (the devnet incident behind
// networks/devnet/proposals/activate-static-precompiles.json).
func TestStaticPrecompileAddressesMatchRegistration(t *testing.T) {
	a := app.SetupWithEmptyStore(t)

	candidates := slices.Concat(
		evmtypes.AvailableStaticPrecompiles,
		[]string{distrclaim.DistributionClaimPrecompileAddress},
		app.StaticPrecompileAddresses,
	)
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	params := evmtypes.Params{ActiveStaticPrecompiles: candidates}

	for _, addr := range candidates {
		hexAddr := common.HexToAddress(addr)
		if slices.Contains(app.StaticPrecompileAddresses, addr) {
			require.NotPanics(t, func() {
				_, found, err := a.EVMKeeper.GetStaticPrecompileInstance(&params, hexAddr)
				require.NoError(t, err)
				require.True(t, found, "%s is in StaticPrecompileAddresses but not registered", addr)
			}, "%s is in StaticPrecompileAddresses but not registered", addr)
			continue
		}
		require.Panics(t, func() {
			_, _, _ = a.EVMKeeper.GetStaticPrecompileInstance(&params, hexAddr)
		}, "%s is registered but missing from StaticPrecompileAddresses", addr)
	}
}

func TestStaticPrecompileFixtures(t *testing.T) {
	want := lowercased(app.StaticPrecompileAddresses)

	for rel, path := range precompileFixtures {
		t.Run(rel, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
			require.NoError(t, err)
			var doc any
			require.NoError(t, json.Unmarshal(raw, &doc))

			got := dig(t, doc, path...)
			list, ok := got.([]any)
			require.True(t, ok, "expected a JSON array at %v, got %T", path, got)

			addrs := make([]string, 0, len(list))
			for _, v := range list {
				s, ok := v.(string)
				require.True(t, ok, "non-string entry %v", v)
				addrs = append(addrs, s)
			}
			require.ElementsMatch(t, want, lowercased(addrs),
				"active_static_precompiles must match app.StaticPrecompileAddresses")
		})
	}
}

func dig(t *testing.T, v any, path ...any) any {
	t.Helper()
	for _, step := range path {
		switch key := step.(type) {
		case string:
			m, ok := v.(map[string]any)
			require.True(t, ok, "expected object at %q, got %T", key, v)
			v, ok = m[key]
			require.True(t, ok, "missing key %q", key)
		case int:
			arr, ok := v.([]any)
			require.True(t, ok, "expected array at index %d, got %T", key, v)
			require.Less(t, key, len(arr), "index %d out of range", key)
			v = arr[key]
		default:
			t.Fatalf("unsupported path step %v", step)
		}
	}
	return v
}

func lowercased(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}
