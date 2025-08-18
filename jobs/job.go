package jobs

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

var logInterval = 5 * time.Second

type Job interface {
	Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error
	Name() string
	Instance() uint64
	SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int)
	WaitForStop() <-chan struct{}
	NeedsFunding() bool
	WalletAddress() *common.Address
	GiveLoadStats() map[string]uint64
	UpdateLoad(indicator LoadIndicator)
}

type JobConfig struct {
	Address  *common.Address
	Key      *ecdsa.PrivateKey
	ChainID  *big.Int
	GasPrice *big.Int
}

type JobFile struct {
	Jobs                []JobJsonDefinition  `json:"jobs"`
	StabilizationConfig *StabilizationConfig `json:"stabilization_config,omitempty"`
}

type JobJsonDefinition struct {
	Count int    `json:"count"`
	Type  string `json:"type"`
}

type StabilizationConfig struct {
	MinSamples                int     `json:"min_samples"`                 // Minimum samples needed before checking stability
	StabilityThresholdPercent float64 `json:"stability_threshold_percent"` // Standard deviation should be within this % of mean for stability
	StabilityRequiredDuration int     `json:"stability_required_duration"` // Number of consecutive stable measurements required
}

type LoadIndicator uint64

var (
	LoadIncrease LoadIndicator = 1
	LoadDecrease LoadIndicator = 2
	LoadSteady   LoadIndicator = 3
)
