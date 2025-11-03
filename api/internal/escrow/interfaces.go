package escrow

import (
	"context"
)

// Client abstracts the on-chain escrow interaction.
type Client interface {
	SubmitIntent(ctx context.Context, req SubmitIntentRequest) (SubmitIntentResponse, error)
	ExecuteMint(ctx context.Context, intentID string) (ExecuteMintResponse, error)
}

type HealthChecker interface {
	Ping(ctx context.Context) error
}

type SubmitIntentRequest struct {
	UserAddress string
	Amount      string // decimal string in wei
	CountryCode string
	TxRef       string
}

type SubmitIntentResponse struct {
	IntentID string
	TxHash   string
}

type ExecuteMintResponse struct {
	TxHash string
}
