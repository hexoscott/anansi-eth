package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type GasPriceReader struct {
	Config   GasPriceReaderConfig
	done     chan struct{}
	instance uint64
}

type GasPriceReaderConfig struct {
	JobConfig
}

func NewGasPriceReader(instance uint64) (*GasPriceReader, error) {
	var config GasPriceReaderConfig
	return &GasPriceReader{
		Config:   config,
		instance: instance,
		done:     make(chan struct{}),
	}, nil
}

func (a *GasPriceReader) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	a.Config.Address = address
	a.Config.Key = privateKey
	a.Config.ChainID = chainID
	a.Config.GasPrice = gasPrice
}

func (a *GasPriceReader) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(a.done)
	log = log.With("job", a.Name(), "instance", a.instance)

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	totalRead := 0

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		case <-ticker.C:
			log.Info("read gas price", "total", totalRead)
		default:
		}

		_, err := client.SuggestGasPrice(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info("received signal, stopping")
				return nil
			}

			log.Error("failed to get gas price", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}
		totalRead++

		time.Sleep(5 * time.Millisecond)
	}
}

func (a *GasPriceReader) Name() string {
	return "gas-price-reader"
}

func (a *GasPriceReader) WaitForStop() <-chan struct{} {
	return a.done
}

func (a *GasPriceReader) Instance() uint64 {
	return a.instance
}

func (a *GasPriceReader) NeedsFunding() bool {
	return false
}

func (a *GasPriceReader) WalletAddress() *common.Address {
	address := common.HexToAddress("0x0000000000000000000000000000000000000000")
	return &address
}
