package state_native

import (
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/runtime/version"
)

// LatestBlockHash returns the hash of the latest execution block.
func (b *BeaconState) LatestBlockHash() ([32]byte, error) {
	if b.version < version.Gloas {
		return [32]byte{}, errNotSupported("LatestBlockHash", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.latestBlockHash == nil {
		return [32]byte{}, nil
	}

	return [32]byte(b.latestBlockHash), nil
}

// BuilderPendingPayments returns the builder pending payments.
func (b *BeaconState) BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingPayments", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.builderPendingPayments == nil {
		return make([]*ethpb.BuilderPendingPayment, 0), nil
	}

	return ethpb.CopyBuilderPendingPaymentSlice(b.builderPendingPayments), nil
}

// BuilderPendingWithdrawals returns the builder pending withdrawals.
func (b *BeaconState) BuilderPendingWithdrawals() ([]*ethpb.BuilderPendingWithdrawal, error) {
	if b.version < version.Gloas {
		return nil, errNotSupported("BuilderPendingWithdrawals", b.version)
	}

	b.lock.RLock()
	defer b.lock.RUnlock()

	if b.builderPendingWithdrawals == nil {
		return make([]*ethpb.BuilderPendingWithdrawal, 0), nil
	}

	return ethpb.CopyBuilderPendingWithdrawalSlice(b.builderPendingWithdrawals), nil
}
