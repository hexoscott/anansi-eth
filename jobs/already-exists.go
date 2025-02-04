package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type AlreadyExists struct {
	Config   AlreadyExistsConfig
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type AlreadyExistsConfig struct {
	JobConfig
}

func NewAlreadyExists(instance uint64) (*AlreadyExists, error) {
	var config AlreadyExistsConfig
	contents, err := os.ReadFile("config/already-exists.json")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(contents, &config); err != nil {
		return nil, err
	}
	return &AlreadyExists{
		Config:   config,
		instance: instance,
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
	}, nil
}

func (a *AlreadyExists) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	a.Config.Address = address
	a.Config.Key = privateKey
	a.Config.ChainID = chainID
	a.Config.GasPrice = gasPrice
}

func (a *AlreadyExists) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", a.Name(), "instance", a.instance)

	randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

	nonce, err := client.PendingNonceAt(context.Background(), *a.Config.Address)
	if err != nil {
		return err
	}

	tx := types.NewTransaction(nonce+1, randomAddress, big.NewInt(1000), 21000, a.Config.GasPrice, nil)

	tx, err = types.SignTx(tx, types.NewEIP155Signer(a.Config.ChainID), a.Config.Key)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	totalSent := 0

	for {
		select {
		case <-a.quit:
			log.Info("received signal, stopping")
			close(a.done)
			return nil
		default:
			err := client.SendTransaction(context.Background(), tx)
			if err != nil {
				if err.Error() != "ALREADY_EXISTS: already known" {
					log.Error("failed to send transaction", "error", err)
					time.Sleep(3 * time.Second)
					continue
				}
			}

			totalSent++

			select {
			case <-ticker.C:
				log.Info("sent transactions", "total", totalSent)
			default:
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (a *AlreadyExists) Stop() <-chan struct{} {
	close(a.quit)
	return a.done
}

func (a *AlreadyExists) Name() string {
	return "already-exists"
}
