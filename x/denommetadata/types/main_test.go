package types_test

import (
	"os"
	"testing"

	"github.com/MANTRA-Chain/mantrachain/v8/app/params"
)

// TestMain applies the chain's bech32 prefixes before any test runs.
//
// sdk.AccAddressFromBech32 validates against the SDK's global config, which
// defaults to "cosmos" and is only switched to "ark" during app startup. A unit
// test that never boots the app therefore rejects perfectly valid ark1
// addresses unless the prefixes are set here.
func TestMain(m *testing.M) {
	params.SetAddressPrefixes()
	os.Exit(m.Run())
}
