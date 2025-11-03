package escrow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// FakeClient hashes the payload to deterministically emulate intent IDs in tests.
type FakeClient struct{}

func (FakeClient) SubmitIntent(_ context.Context, req SubmitIntentRequest) (SubmitIntentResponse, error) {
	if req.UserAddress == "" {
		return SubmitIntentResponse{}, fmt.Errorf("missing user address")
	}
	hash := sha256.Sum256([]byte(req.UserAddress + req.Amount + req.CountryCode + req.TxRef))
	return SubmitIntentResponse{
		IntentID: "0x" + hex.EncodeToString(hash[:]),
		TxHash:   "",
	}, nil
}

func (FakeClient) ExecuteMint(_ context.Context, intentID string) (ExecuteMintResponse, error) {
	return ExecuteMintResponse{TxHash: fakeHash(intentID)}, nil
}

func fakeHash(input string) string {
	sum := sha256.Sum256([]byte(input))
	return "0x" + hex.EncodeToString(sum[:])
}
