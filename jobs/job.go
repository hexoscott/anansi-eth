package jobs

import (
	"crypto/ecdsa"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type Job interface {
	Run(client *ethclient.Client, log hclog.Logger) error
	Name() string
	SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int)
	Stop() <-chan struct{}
}

type JobConfig struct {
	Address *common.Address
	Key     *ecdsa.PrivateKey
	ChainID *big.Int
}
