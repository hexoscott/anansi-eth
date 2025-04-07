package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
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
		done:     make(chan struct{}),
	}, nil
}

func (a *AlreadyExists) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	a.Config.Address = address
	a.Config.Key = privateKey
	a.Config.ChainID = chainID
	a.Config.GasPrice = gasPrice
}

func (a *AlreadyExists) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(a.done)
	log = log.With("job", a.Name(), "instance", a.instance)

	randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

	nonce, err := client.PendingNonceAt(ctx, *a.Config.Address)
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
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		default:
		}

		err := client.SendTransaction(ctx, tx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info("received signal, stopping")
				return nil
			}
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

func (a *AlreadyExists) Name() string {
	return "already-exists"
}

func (a *AlreadyExists) WaitForStop() <-chan struct{} {
	return a.done
}

func (a *AlreadyExists) Instance() uint64 {
	return a.instance
}
