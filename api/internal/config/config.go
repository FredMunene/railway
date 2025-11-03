package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SeedConfig models the subset of values we need from seed.json.
type SeedConfig struct {
	CandidateID string `json:"candidateId"`
	Chain       struct {
		ChainID   int64  `json:"chainId"`
		RPCURL    string `json:"rpcUrl"`
		BlockTime int    `json:"blockTime"`
	} `json:"chain"`
	Tokens struct {
		Stablecoin struct {
			Symbol   string `json:"symbol"`
			Name     string `json:"name"`
			Decimals int    `json:"decimals"`
		} `json:"stablecoin"`
		Country struct {
			Symbol      string `json:"symbol"`
			Name        string `json:"name"`
			CountryCode string `json:"countryCode"`
			Decimals    int    `json:"decimals"`
		} `json:"country"`
	} `json:"tokens"`
	Secrets struct {
		HMACSalt           string `json:"hmacSalt"`
		IdempotencyKeySalt string `json:"idempotencyKeySalt"`
		MpesaWebhookSecret string `json:"mpesaWebhookSecret"`
	} `json:"secrets"`
	Compliance struct {
		MaxRiskScore       int  `json:"maxRiskScore"`
		RequireAttestation bool `json:"requireAttestation"`
		MinAttestationAge  int  `json:"minAttestationAge"`
	} `json:"compliance"`
	Limits struct {
		MinMintAmount  string `json:"minMintAmount"`
		MaxMintAmount  string `json:"maxMintAmount"`
		DailyMintLimit string `json:"dailyMintLimit"`
	} `json:"limits"`
	Retry struct {
		MaxAttempts       int `json:"maxAttempts"`
		InitialBackoffMs  int `json:"initialBackoffMs"`
		MaxBackoffMs      int `json:"maxBackoffMs"`
		BackoffMultiplier int `json:"backoffMultiplier"`
	} `json:"retry"`
	Timeouts struct {
		RPCTimeoutMs          int `json:"rpcTimeoutMs"`
		WebhookTimeoutMs      int `json:"webhookTimeoutMs"`
		IdempotencyWindowSecs int `json:"idempotencyWindowSeconds"`
	} `json:"timeouts"`
}

// DeploymentConfig represents deployments.json.
type DeploymentConfig struct {
	ChainID   int64  `json:"chainId"`
	Deployer  string `json:"deployer"`
	Admin     string `json:"admin"`
	Executor  string `json:"executor"`
	Contracts struct {
		USDStablecoin     string `json:"USDStablecoin"`
		CountryToken      string `json:"CountryToken"`
		UserRegistry      string `json:"UserRegistry"`
		ComplianceManager string `json:"ComplianceManager"`
		MintEscrow        string `json:"MintEscrow"`
	} `json:"contracts"`
}

// AppConfig ties together seed + deployment info and derived values.
type AppConfig struct {
	Seed       SeedConfig
	Deployment DeploymentConfig
	Service    ServiceConfig
	Chain      ChainConfig
}

type ServiceConfig struct {
	HTTPPort             int
	HMACClockSkew        time.Duration
	IdempotencyWindow    time.Duration
	IdempotencyStorePath string
}

type ChainConfig struct {
	RPCURL     string
	PrivateKey string
}

const (
	defaultSeedPath        = "../seed.json"
	defaultDeploymentsPath = "../deployments.json"
)

// Load aggregates configuration from disk and environment.
func Load() (*AppConfig, error) {
	seedPath := envOr("SEED_PATH", defaultSeedPath)
	deploymentsPath := envOr("DEPLOYMENTS_PATH", defaultDeploymentsPath)

	seedCfg, err := loadSeed(seedPath)
	if err != nil {
		return nil, fmt.Errorf("load seed: %w", err)
	}

	deployCfg, err := loadDeployments(deploymentsPath)
	if err != nil {
		return nil, fmt.Errorf("load deployments: %w", err)
	}

	serviceCfg := ServiceConfig{
		HTTPPort:             envOrInt("API_HTTP_PORT", 3000),
		HMACClockSkew:        time.Duration(envOrInt("HMAC_CLOCK_SKEW_SECONDS", 60)) * time.Second,
		IdempotencyWindow:    time.Duration(seedCfg.Timeouts.IdempotencyWindowSecs) * time.Second,
		IdempotencyStorePath: envOr("IDEMPOTENCY_STORE_PATH", filepath.Join(os.TempDir(), "fiatrails-idem.json")),
	}

	chainCfg := ChainConfig{
		RPCURL:     envOr("CHAIN_RPC_URL", seedCfg.Chain.RPCURL),
		PrivateKey: envOr("CHAIN_PRIVATE_KEY", ""),
	}

	return &AppConfig{
		Seed:       *seedCfg,
		Deployment: *deployCfg,
		Service:    serviceCfg,
		Chain:      chainCfg,
	}, nil
}

func loadSeed(path string) (*SeedConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg SeedConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func loadDeployments(path string) (*DeploymentConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg DeploymentConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func envOr(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		var parsed int
		if _, err := fmt.Sscanf(val, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return fallback
}
