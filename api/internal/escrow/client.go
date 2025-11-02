package escrow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// Client abstracts the on-chain escrow interaction.
type Client interface {
	SubmitIntent(ctx context.Context, req SubmitIntentRequest) (SubmitIntentResponse, error)
}

type SubmitIntentRequest struct {
	UserAddress string
	Amount      string
	CountryCode string
	TxRef       string
}

type SubmitIntentResponse struct {
	IntentID string
}

// FakeClient is a temporary placeholder that mimics the escrow submit flow by hashing the payload.
type FakeClient struct{}

func (FakeClient) SubmitIntent(_ context.Context, req SubmitIntentRequest) (SubmitIntentResponse, error) {
	if req.UserAddress == "" {
		return SubmitIntentResponse{}, fmt.Errorf("missing user address")
	}
	hash := sha256.Sum256([]byte(req.UserAddress + req.Amount + req.CountryCode + req.TxRef))
	return SubmitIntentResponse{
		IntentID: "0x" + hex.EncodeToString(hash[:]),
	}, nil
}
