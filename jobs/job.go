package jobs

import (
	"crypto/ecdsa"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

var logInterval = 5 * time.Second

type Job interface {
	Run(client *ethclient.Client, log hclog.Logger) error
	Name() string
	SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int)
	Stop() <-chan struct{}
}

type JobConfig struct {
	Address  *common.Address
	Key      *ecdsa.PrivateKey
	ChainID  *big.Int
	GasPrice *big.Int
}
