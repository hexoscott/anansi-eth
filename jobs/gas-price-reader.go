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

type GasPriceReader struct {
	Config   GasPriceReaderConfig
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type GasPriceReaderConfig struct {
	JobConfig
}

func NewGasPriceReader(instance uint64) (*GasPriceReader, error) {
	var config GasPriceReaderConfig
	// contents, err := os.ReadFile("config/gas-price-reader.json")
	// if err != nil {
	// 	return nil, err
	// }
	// if err := json.Unmarshal(contents, &config); err != nil {
	// 	return nil, err
	// }
	return &GasPriceReader{
		Config:   config,
		instance: instance,
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

func (a *GasPriceReader) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	a.Config.Address = address
	a.Config.Key = privateKey
	a.Config.ChainID = chainID
	a.Config.GasPrice = gasPrice
}

func (a *GasPriceReader) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", a.Name(), "instance", a.instance)

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	totalRead := 0

	for {
		select {
		case <-a.quit:
			log.Info("received signal, stopping")
			close(a.done)
			return nil
		case <-ticker.C:
			log.Info("read gas price", "total", totalRead)
		default:
		}

		func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, err := client.SuggestGasPrice(ctx)
			if err != nil {
				log.Error("failed to get gas price", "error", err)
				time.Sleep(1 * time.Second)
				return
			}
		}()

		totalRead++

		time.Sleep(5 * time.Millisecond)
	}
}

func (a *GasPriceReader) Stop() <-chan struct{} {
	close(a.quit)
	return a.done
}

func (a *GasPriceReader) Name() string {
	return "gas-price-reader"
}
