package types_test

import (
	"testing"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/stretchr/testify/require"

	"github.com/MANTRA-Chain/mantrachain/v8/x/denommetadata/types"
)

const govAuthority = "ark10d07y265gmmuvt4z0w9aw880jnsr700j2cu5hn"

// ibcSynthesisedMetadata is the metadata arkdevnet_9000-1 actually produced when
// MANTRA's amantra arrived over transfer/channel-0 on 2026-09-04, copied from
// the live chain rather than invented. Its ERC-20 reports decimals() = 0.
//
// It is the exact input this module must refuse: `display` names a unit that is
// absent from denom_units, so any consumer resolving decimals by looking up the
// display unit finds nothing and falls back to 0.
func ibcSynthesisedMetadata() banktypes.Metadata {
	return banktypes.Metadata{
		Description: "IBC token from transfer/channel-0/amantra",
		Base:        "ibc/784D26186FFF0AFB5DD933EEE6E6E1C7EEC542B44566E7626E8E4A35BC66CC7E",
		Display:     "transfer/channel-0/amantra",
		Name:        "transfer/channel-0/amantra IBC token",
		Symbol:      "AMANTRA",
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: "amantra", Exponent: 0},
		},
	}
}

// correctedMetadata is what a governance proposal should submit instead: the
// display unit is present at the source chain's real exponent.
func correctedMetadata() banktypes.Metadata {
	md := ibcSynthesisedMetadata()
	md.Display = "mantra"
	md.DenomUnits = []*banktypes.DenomUnit{
		{Denom: "amantra", Exponent: 0},
		{Denom: "mantra", Exponent: 18},
	}
	return md
}

func TestValidateBasic_RejectsTheBugItExistsToFix(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata:  []banktypes.Metadata{ibcSynthesisedMetadata()},
	}
	err := msg.ValidateBasic()
	require.Error(t, err, "metadata whose display is absent from denom_units must be refused")
	require.Contains(t, err.Error(), "decimals would resolve to 0")
}

func TestValidateBasic_AcceptsCorrectedMetadata(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata:  []banktypes.Metadata{correctedMetadata()},
	}
	require.NoError(t, msg.ValidateBasic())
}

func TestValidateBasic_RejectsBadAuthority(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{
		Authority: "not-a-bech32-address",
		Metadata:  []banktypes.Metadata{correctedMetadata()},
	}
	require.ErrorIs(t, msg.ValidateBasic(), types.ErrInvalidAuthority)
}

func TestValidateBasic_RejectsEmptyMetadata(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{Authority: govAuthority}
	require.ErrorIs(t, msg.ValidateBasic(), types.ErrEmptyMetadata)
}

// A message carrying the same denom twice would apply both writes in order,
// making the result depend on ordering rather than on what governance voted for.
func TestValidateBasic_RejectsDuplicateDenoms(t *testing.T) {
	a := correctedMetadata()
	b := correctedMetadata()
	b.DenomUnits[1].Exponent = 6 // a different, conflicting claim about the same denom

	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata:  []banktypes.Metadata{a, b},
	}
	err := msg.ValidateBasic()
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate entry")
}

func TestDisplayResolvesToAUnit(t *testing.T) {
	require.Error(t, types.DisplayResolvesToAUnit(ibcSynthesisedMetadata()))
	require.NoError(t, types.DisplayResolvesToAUnit(correctedMetadata()))

	t.Run("native denom already satisfies the rule", func(t *testing.T) {
		// esp/KASH from the live chain — proof the rule does not reject
		// well-formed metadata that the chain already relies on.
		require.NoError(t, types.DisplayResolvesToAUnit(banktypes.Metadata{
			Base:    "esp",
			Display: "KASH",
			DenomUnits: []*banktypes.DenomUnit{
				{Denom: "esp", Exponent: 0},
				{Denom: "espees", Exponent: 9},
				{Denom: "KASH", Exponent: 18},
			},
		}))
	})
}
