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

type RepeatReceiptsJob struct {
	Config   RepeatReceiptsJobConfig
	done     chan struct{}
	instance uint64
}

type RepeatReceiptsJobConfig struct {
	JobConfig
}

func NewRepeatReceiptsJob(instance uint64) (*RepeatReceiptsJob, error) {
	return &RepeatReceiptsJob{
		Config:   RepeatReceiptsJobConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (g *RepeatReceiptsJob) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	g.Config.Address = address
	g.Config.Key = privateKey
	g.Config.ChainID = chainID
	g.Config.GasPrice = gasPrice
}

func (g *RepeatReceiptsJob) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(g.done)
	log = log.With("job", g.Name(), "instance", g.instance)

	totalSent := 0

	for {
		select {
		case <-ctx.Done():
			log.Debug("received signal, stopping")
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
		repeatsCount := 0
		for {
			select {
			case <-ctx.Done():
				log.Debug("received signal, stopping")
				return nil
			default:
			}

			receipt, err := client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Debug("received signal, stopping")
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
				if repeatsCount > 50 {
					break
				}
				repeatsCount++
			} else {
				log.Warn("transaction failed", "hash", tx.Hash().Hex())
				break
			}

			rnd := rand.Intn(5)
			time.Sleep(time.Duration(rnd) * time.Millisecond)
		}

		totalSent++
	}
}

func (g *RepeatReceiptsJob) WaitForStop() <-chan struct{} {
	return g.done
}

func (g *RepeatReceiptsJob) Name() string {
	return "repeat-receipts"
}

func (g *RepeatReceiptsJob) Instance() uint64 {
	return g.instance
}

func (g *RepeatReceiptsJob) NeedsFunding() bool {
	return true
}

func (g *RepeatReceiptsJob) WalletAddress() *common.Address {
	return g.Config.Address
}
