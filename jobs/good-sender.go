package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type GoodSender struct {
	Config   GoodSenderConfig
	done     chan struct{}
	instance uint64
}

type GoodSenderConfig struct {
	JobConfig
}

func NewGoodSender(instance uint64) (*GoodSender, error) {
	return &GoodSender{
		Config:   GoodSenderConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (g *GoodSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	g.Config.Address = address
	g.Config.Key = privateKey
	g.Config.ChainID = chainID
	g.Config.GasPrice = gasPrice
}

func (g *GoodSender) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(g.done)
	log = log.With("job", g.Name(), "instance", g.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		default:
		}

		nonce, err := client.PendingNonceAt(ctx, *g.Config.Address)
		if err != nil {
			return err
		}

		randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

		tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, g.Config.GasPrice, nil)

		tx, err = types.SignTx(tx, types.NewEIP155Signer(g.Config.ChainID), g.Config.Key)
		if err != nil {
			return err
		}

		err = client.SendTransaction(ctx, tx)
		if err != nil {
			log.Error("failed to send transaction", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		// now get the receipt
		receiptCount := 0
		for {
			select {
			case <-ctx.Done():
				log.Info("received signal, stopping")
				return nil
			default:
			}

			receipt, err := client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Info("received signal, stopping")
					return nil
				}
				if errors.Is(err, ethereum.NotFound) {
					time.Sleep(500 * time.Millisecond)
					receiptCount++
					if receiptCount > 50 {
						log.Error("waited too long for receipt", "error", err)
						time.Sleep(3 * time.Second)
						continue
					}
					continue
				} else {
					log.Error("failed to get transaction receipt", "error", err)
					time.Sleep(1 * time.Second)
					continue
				}
			}
			if receipt.Status == types.ReceiptStatusSuccessful {
				totalSent++
				break
			} else {
				log.Warn("transaction failed", "hash", tx.Hash().Hex())
				break
			}
		}

		select {
		case <-ticker.C:
			log.Info("sent transactions", "from", g.Config.Address.Hex(), "total", totalSent)
		default:
		}
	}
}

func (g *GoodSender) WaitForStop() <-chan struct{} {
	return g.done
}

func (g *GoodSender) Name() string {
	return "good-sender"
}

func (g *GoodSender) Instance() uint64 {
	return g.instance
}
