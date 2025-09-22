package state_native

import (
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state/state-native/types"
	"github.com/pkg/errors"
)

// UpdateExecutionPayloadAvailabilityAtIndex updates the execution payload availability at a specific index.
func (b *BeaconState) UpdateExecutionPayloadAvailabilityAtIndex(idx uint64, val byte) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if idx >= uint64(len(b.executionPayloadAvailability)) {
		return errors.Errorf("index %d out of range for execution payload availability length %d", idx, len(b.executionPayloadAvailability))
	}

	b.executionPayloadAvailability[idx] = val
	b.markFieldAsDirty(types.ExecutionPayloadAvailability)
	return nil
}
