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
// Verified against precompiles/erc20/query.go's Decimals(): for an ibc/ base
// denom it matches the last '/'-separated segment of `display` ("amantra")
// against denom_units, that DOES match the single unit here, and the lookup
// correctly returns what is stored — exponent 0. There is no "not found"
// fallback in that path; a genuine non-match reverts the call. This module
// exists because 0 is what got written, not because a lookup failed.
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
//
// Note the first denom unit. banktypes.Metadata.Validate() requires it to be the
// BASE denom at exponent 0 — for an IBC asset that is the ibc/<hash>, not the
// source chain's base name. Writing {amantra, 0} first looks natural and is
// rejected, which is worth knowing before drafting a proposal.
func correctedMetadata() banktypes.Metadata {
	md := ibcSynthesisedMetadata()
	md.Display = "mantra"
	md.DenomUnits = []*banktypes.DenomUnit{
		{Denom: md.Base, Exponent: 0, Aliases: []string{"amantra"}},
		{Denom: "mantra", Exponent: 18},
	}
	return md
}

// The metadata the chain synthesises on IBC receipt is refused. Note WHICH rule
// catches it: banktypes.Validate() rejects it first, because its single denom
// unit is named "amantra" while the base is the ibc/<hash>. So the live metadata
// on this chain is not merely inconsistent about display — it would not pass
// bank's own validation if anyone tried to write it back. It exists only because
// the transfer module sets it directly, bypassing Validate().
func TestValidateBasic_RejectsTheBugItExistsToFix(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata:  []banktypes.Metadata{ibcSynthesisedMetadata()},
	}
	require.Error(t, msg.ValidateBasic(),
		"the synthesised IBC metadata must never be writable through this module")
}

// Metadata whose display names a unit that does not exist is refused. This is
// the shape that yields decimals() = 0, and bank's own Validate() already
// catches it — verified rather than assumed, which is why this module carries no
// bespoke check for it.
func TestValidateBasic_RejectsUnresolvableDisplay(t *testing.T) {
	md := correctedMetadata()
	md.Display = "mantra"
	md.DenomUnits = []*banktypes.DenomUnit{
		{Denom: md.Base, Exponent: 0},
	}

	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata:  []banktypes.Metadata{md},
	}
	err := msg.ValidateBasic()
	require.Error(t, err)
	require.Contains(t, err.Error(), "display denom 'mantra'")
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

// The chain's own native metadata must still pass, so the module cannot reject
// what the chain already relies on. Copied from the live chain.
func TestValidateBasic_AcceptsNativeMetadata(t *testing.T) {
	msg := &types.MsgSetDenomMetadata{
		Authority: govAuthority,
		Metadata: []banktypes.Metadata{{
			Base:    "esp",
			Display: "KASH",
			Name:    "KASH",
			Symbol:  "KASH",
			DenomUnits: []*banktypes.DenomUnit{
				{Denom: "esp", Exponent: 0},
				{Denom: "espees", Exponent: 9},
				{Denom: "KASH", Exponent: 18},
			},
		}},
	}
	require.NoError(t, msg.ValidateBasic())
}
