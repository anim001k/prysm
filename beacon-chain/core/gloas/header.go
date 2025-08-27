package gloas

import (
	"fmt"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/helpers"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/blocks"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/crypto/bls"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

// ProcessExecutionPayloadBid processes a signed execution payload bid in the Gloas fork.
func ProcessExecutionPayloadBid(st state.BeaconState, block interfaces.ReadOnlyBeaconBlock) error {
	signedBid, err := block.Body().SignedExecutionPayloadBid()
	if err != nil {
		return fmt.Errorf("failed to get signed execution payload bid: %w", err)
	}

	wrappedBid, err := blocks.WrappedROSignedExecutionPayloadBid(signedBid)
	if err != nil {
		return fmt.Errorf("failed to wrap signed bid: %w", err)
	}

	if err := validatePayloadBidSignature(st, wrappedBid); err != nil {
		return fmt.Errorf("bid signature validation failed: %w", err)
	}

	bid, err := wrappedBid.Bid()
	if err != nil {
		return fmt.Errorf("failed to get bid from wrapped bid: %w", err)
	}

	if err := validateBuilder(st, bid, block.ProposerIndex()); err != nil {
		return fmt.Errorf("builder validation failed: %w", err)
	}

	if err := validateBidConsistency(st, bid, block); err != nil {
		return fmt.Errorf("bid consistency validation failed: %w", err)
	}

	// Cache the pending payment
	feeRecipient := bid.FeeRecipient()
	pendingPayment := &ethpb.BuilderPendingPayment{
		Weight: 0,
		Withdrawal: &ethpb.BuilderPendingWithdrawal{
			FeeRecipient: feeRecipient[:],
			Amount:       bid.Value(),
			BuilderIndex: bid.BuilderIndex(),
		},
	}
	slotIndex := params.BeaconConfig().SlotsPerEpoch + (bid.Slot() % params.BeaconConfig().SlotsPerEpoch)
	if err := st.SetBuilderPendingPayment(slotIndex, pendingPayment); err != nil {
		return fmt.Errorf("failed to set pending payment: %w", err)
	}

	// Cache the signed execution payload bid
	if err := st.SetExecutionPayloadBid(bid); err != nil {
		return fmt.Errorf("failed to cache execution payload bid: %w", err)
	}

	return nil
}

// validateBuilder checks if the builder is eligible to submit execution payload bids.
// This includes self-build validation, withdrawal credential checks, and enough balance.
func validateBuilder(st state.BeaconState, bid interfaces.ROExecutionPayloadBid, proposerIndex primitives.ValidatorIndex) error {
	builderIndex := bid.BuilderIndex()
	builder, err := st.ValidatorAtIndex(builderIndex)
	if err != nil {
		return fmt.Errorf("failed to get builder validator: %w", err)
	}

	currentEpoch := slots.ToEpoch(st.Slot())
	if !helpers.IsActiveValidator(builder, currentEpoch) {
		return fmt.Errorf("builder %d is not active in epoch %d", builderIndex, currentEpoch)
	}

	if builder.Slashed {
		return fmt.Errorf("builder %d is slashed", builderIndex)
	}

	amount := bid.Value()

	// Self-build validation: amount must be zero when builder == proposer
	fmt.Println(builderIndex, proposerIndex, amount)
	if builderIndex == proposerIndex {
		if amount != 0 {
			return fmt.Errorf("self-build amount must be zero, got %d", amount)
		}
	} else {
		// Non-self builds require builder withdrawal credential
		if err := validateBuilderWithdrawalCredential(builder); err != nil {
			return fmt.Errorf("builder withdrawal credential validation failed: %w", err)
		}
	}

	if err := validateBuilderHasEnoughBalance(st, builderIndex, amount); err != nil {
		return fmt.Errorf("builder financial capacity validation failed: %w", err)
	}

	return nil
}

// validateBidConsistency checks that the bid is consistent with the current beacon state.
func validateBidConsistency(st state.BeaconState, bid interfaces.ROExecutionPayloadBid, block interfaces.ReadOnlyBeaconBlock) error {
	// Verify that the bid is for the current slot
	if bid.Slot() != block.Slot() {
		return fmt.Errorf("bid slot %d does not match block slot %d", bid.Slot(), block.Slot())
	}

	// Verify that the bid is for the right parent block hash
	latestBlockHash, err := st.LatestBlockHash()
	if err != nil {
		return fmt.Errorf("failed to get latest block hash: %w", err)
	}
	if bid.ParentBlockHash() != latestBlockHash {
		return fmt.Errorf("bid parent block hash mismatch: got %x, expected %x",
			bid.ParentBlockHash(), latestBlockHash)
	}

	// Verify that the bid is for the right parent block root
	if bid.ParentBlockRoot() != block.ParentRoot() {
		return fmt.Errorf("bid parent block root mismatch: got %x, expected %x",
			bid.ParentBlockRoot(), block.ParentRoot())
	}

	return nil
}

// validateBuilderWithdrawalCredential checks if the builder has the correct withdrawal credential prefix.
func validateBuilderWithdrawalCredential(validator *ethpb.Validator) error {
	// Check if withdrawal credential has the builder prefix (0x02)
	if len(validator.WithdrawalCredentials) != 32 {
		return fmt.Errorf("invalid withdrawal credential length: %d", len(validator.WithdrawalCredentials))
	}

	if validator.WithdrawalCredentials[0] != params.BeaconConfig().BuilderWithdrawalPrefixByte {
		return fmt.Errorf("builder must have withdrawal credential prefix 0x%02x, got 0x%02x",
			params.BeaconConfig().BuilderWithdrawalPrefixByte,
			validator.WithdrawalCredentials[0])
	}

	return nil
}

// validateBuilderHasEnoughBalance checks if the builder has sufficient funds for the bid.
func validateBuilderHasEnoughBalance(st state.BeaconState, builderIndex primitives.ValidatorIndex, amount primitives.Gwei) error {
	if amount == 0 {
		return nil // No payment required
	}

	builderBalance, err := st.BalanceAtIndex(builderIndex)
	if err != nil {
		return fmt.Errorf("failed to get builder balance: %w", err)
	}

	// Sum pending payments for this builder
	pendingPayments, err := calculatePendingPayments(st, builderIndex)
	if err != nil {
		return fmt.Errorf("failed to calculate pending payments: %w", err)
	}

	// Sum pending withdrawals for this builder
	pendingWithdrawals, err := calculatePendingWithdrawals(st, builderIndex)
	if err != nil {
		return fmt.Errorf("failed to calculate pending withdrawals: %w", err)
	}

	minActivationBalance := params.BeaconConfig().MinActivationBalance
	requiredBalance := uint64(amount) + pendingPayments + pendingWithdrawals + minActivationBalance

	if builderBalance < requiredBalance {
		return fmt.Errorf("builder %d has insufficient balance: has %d, needs %d (amount=%d, pending_payments=%d, pending_withdrawals=%d, min_activation=%d)",
			builderIndex, builderBalance, requiredBalance, amount, pendingPayments, pendingWithdrawals, minActivationBalance)
	}

	return nil
}

// calculatePendingPayments sums all pending payments for a given builder.
func calculatePendingPayments(st state.BeaconState, builderIndex primitives.ValidatorIndex) (uint64, error) {
	pendingPayments, err := st.BuilderPendingPayments()
	if err != nil {
		return 0, fmt.Errorf("failed to get pending payments: %w", err)
	}

	var total uint64
	for _, payment := range pendingPayments {
		if payment.Withdrawal.BuilderIndex == builderIndex {
			total += uint64(payment.Withdrawal.Amount)
		}
	}

	return total, nil
}

// calculatePendingWithdrawals sums all pending withdrawals for a given builder.
func calculatePendingWithdrawals(st state.BeaconState, builderIndex primitives.ValidatorIndex) (uint64, error) {
	pendingWithdrawals, err := st.BuilderPendingWithdrawals()
	if err != nil {
		return 0, fmt.Errorf("failed to get pending withdrawals: %w", err)
	}

	var total uint64
	for _, withdrawal := range pendingWithdrawals {
		if withdrawal.BuilderIndex == builderIndex {
			total += uint64(withdrawal.Amount)
		}
	}

	return total, nil
}

// validatePayloadBidSignature verifies the BLS signature on a signed execution payload bid.
// It validates that the signature was created by the builder specified in the bid
// using the appropriate domain for the beacon builder.
func validatePayloadBidSignature(st state.ReadOnlyBeaconState, signedBid interfaces.ROSignedExecutionPayloadBid) error {
	bid, err := signedBid.Bid()
	if err != nil {
		return fmt.Errorf("failed to get bid: %w", err)
	}

	builderPubkey := st.PubkeyAtIndex(bid.BuilderIndex())
	publicKey, err := bls.PublicKeyFromBytes(builderPubkey[:])
	if err != nil {
		return fmt.Errorf("invalid builder public key: %w", err)
	}

	signatureBytes := signedBid.Signature()
	signature, err := bls.SignatureFromBytes(signatureBytes[:])
	if err != nil {
		return fmt.Errorf("invalid signature format: %w", err)
	}

	currentEpoch := slots.ToEpoch(bid.Slot())
	domain, err := signing.Domain(
		st.Fork(),
		currentEpoch,
		params.BeaconConfig().DomainBeaconBuilder,
		st.GenesisValidatorsRoot(),
	)
	if err != nil {
		return fmt.Errorf("failed to compute signing domain: %w", err)
	}

	signingRoot, err := signedBid.SigningRoot(domain)
	if err != nil {
		return fmt.Errorf("failed to compute signing root: %w", err)
	}

	if !signature.Verify(publicKey, signingRoot[:]) {
		return fmt.Errorf("signature verification failed: %w", signing.ErrSigFailedToVerify)
	}

	return nil
}
