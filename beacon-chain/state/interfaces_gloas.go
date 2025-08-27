package state

import (
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
)

type writeOnlyGloasFields interface {
	SetExecutionPayloadBid(h interfaces.ROExecutionPayloadBid) error
	SetBuilderPendingPayment(index primitives.Slot, payment *ethpb.BuilderPendingPayment) error
}

type readOnlyGloasFields interface {
	LatestBlockHash() ([32]byte, error)
	BuilderPendingPayments() ([]*ethpb.BuilderPendingPayment, error)
	BuilderPendingWithdrawals() ([]*ethpb.BuilderPendingWithdrawal, error)
}
