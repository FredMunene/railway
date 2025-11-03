package escrow

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"time"

	"fiatrails/internal/contracts"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// EthClient submits transactions to MintEscrow.
type EthClient struct {
	client    *ethclient.Client
	contract  *bind.BoundContract
	abi       abi.ABI
	address   common.Address
	chainID   *big.Int
	transacts *bind.TransactOpts
}

type EthClientConfig struct {
	RPCURL             string
	PrivateKeyHex      string
	ContractMintEscrow string
}

func NewEthClient(ctx context.Context, cfg EthClientConfig) (*EthClient, error) {
	if cfg.RPCURL == "" {
		return nil, fmt.Errorf("rpc url is required")
	}
	if cfg.ContractMintEscrow == "" {
		return nil, fmt.Errorf("mint escrow address is required")
	}

	cli, err := ethclient.DialContext(ctx, cfg.RPCURL)
	// Eth client call to remote.
	if err != nil {
		return nil, fmt.Errorf("dial rpc: %w", err)
	}

	parsedABI, err := abi.JSON(strings.NewReader(string(contracts.MintEscrowABI)))
	if err != nil {
		return nil, fmt.Errorf("parse abi: %w", err)
	}

	address := common.HexToAddress(cfg.ContractMintEscrow)
	bound := bind.NewBoundContract(address, parsedABI, cli, cli, cli)

	var txOpts *bind.TransactOpts
	if cfg.PrivateKeyHex != "" {
		pk, err := parsePrivateKey(cfg.PrivateKeyHex)
		if err != nil {
			return nil, err
		}

		chainID, err := cli.ChainID(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch chain id: %w", err)
		}

		txOpts, err = bind.NewKeyedTransactorWithChainID(pk, chainID)
		if err != nil {
			return nil, fmt.Errorf("transactor: %w", err)
		}
		txOpts.Context = ctx
		txOpts.NoSend = false
		txOpts.GasLimit = 0 // let node estimate
		txOpts.GasPrice = nil
		txOpts.Nonce = nil
		return &EthClient{
			client:    cli,
			contract:  bound,
			abi:       parsedABI,
			address:   address,
			chainID:   chainID,
			transacts: txOpts,
		}, nil
	}

	return nil, fmt.Errorf("private key is required for submitting intents")
}

func parsePrivateKey(hexKey string) (*ecdsa.PrivateKey, error) {
	hexKey = strings.TrimPrefix(hexKey, "0x")
	key, err := crypto.HexToECDSA(hexKey)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return key, nil
}

func (c *EthClient) SubmitIntent(ctx context.Context, req SubmitIntentRequest) (SubmitIntentResponse, error) {
	if c.transacts == nil {
		return SubmitIntentResponse{}, fmt.Errorf("client is read-only")
	}
	if err := validateSubmitRequest(req); err != nil {
		return SubmitIntentResponse{}, err
	}

	amount, ok := new(big.Int).SetString(req.Amount, 10)
	if !ok {
		return SubmitIntentResponse{}, fmt.Errorf("invalid amount: %s", req.Amount)
	}

	countryCodeBytes := toBytes32(req.CountryCode)
	txRefBytes := toBytes32(req.TxRef)

	opts := *c.transacts
	opts.Context = ctx

	tx, err := c.contract.Transact(&opts, "submitIntent", amount, countryCodeBytes, txRefBytes)
	if err != nil {
		return SubmitIntentResponse{}, fmt.Errorf("submit intent tx: %w", err)
	}

	intentID, err := computeIntentID(req)
	if err != nil {
		return SubmitIntentResponse{}, err
	}

	return SubmitIntentResponse{
		IntentID: intentID,
		TxHash:   tx.Hash().Hex(),
	}, nil
}

func (c *EthClient) ExecuteMint(ctx context.Context, intentID string) (ExecuteMintResponse, error) {
	if c.transacts == nil {
		return ExecuteMintResponse{}, fmt.Errorf("client is read-only")
	}
	if len(intentID) != 66 || !strings.HasPrefix(intentID, "0x") {
		return ExecuteMintResponse{}, fmt.Errorf("invalid intent id")
	}

	hash := common.HexToHash(intentID)

	opts := *c.transacts
	opts.Context = ctx

	tx, err := c.contract.Transact(&opts, "executeMint", hash)
	if err != nil {
		return ExecuteMintResponse{}, fmt.Errorf("execute mint tx: %w", err)
	}

	return ExecuteMintResponse{TxHash: tx.Hash().Hex()}, nil
}

func (c *EthClient) Ping(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("rpc client not configured")
	}
	_, err := c.client.BlockNumber(ctx)
	return err
}

func validateSubmitRequest(req SubmitIntentRequest) error {
	if !common.IsHexAddress(req.UserAddress) {
		return fmt.Errorf("invalid user address")
	}
	if strings.TrimSpace(req.Amount) == "" {
		return fmt.Errorf("amount required")
	}
	if strings.TrimSpace(req.CountryCode) == "" {
		return fmt.Errorf("country code required")
	}
	if strings.TrimSpace(req.TxRef) == "" {
		return fmt.Errorf("txRef required")
	}
	return nil
}

func toBytes32(value string) [32]byte {
	var out [32]byte
	copy(out[:], []byte(value))
	return out
}

func computeIntentID(req SubmitIntentRequest) (string, error) {
	amount, ok := new(big.Int).SetString(req.Amount, 10)
	if !ok {
		return "", fmt.Errorf("invalid amount %s", req.Amount)
	}
	country := toBytes32(req.CountryCode)
	txRef := toBytes32(req.TxRef)
	trace := crypto.Keccak256Hash(
		common.HexToAddress(req.UserAddress).Bytes(),
		common.LeftPadBytes(amount.Bytes(), 32),
		country[:],
		txRef[:],
	)
	return trace.Hex(), nil
}

// WaitForReceipt polls until the transaction is mined or context cancelled.
func WaitForReceipt(ctx context.Context, client *ethclient.Client, tx *types.Transaction) (*types.Receipt, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		receipt, err := client.TransactionReceipt(ctx, tx.Hash())
		if receipt != nil {
			return receipt, nil
		}
		if err != nil && err.Error() != "not found" {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}
