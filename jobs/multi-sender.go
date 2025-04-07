package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type MultiSender struct {
	Config   MultiSenderConfig
	done     chan struct{}
	instance uint64
}

type MultiSenderConfig struct {
	JobConfig
}

func NewMultiSender(instance uint64) (*MultiSender, error) {
	return &MultiSender{
		Config:   MultiSenderConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (m *MultiSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	m.Config.Address = address
	m.Config.Key = privateKey
	m.Config.ChainID = chainID
	m.Config.GasPrice = gasPrice
}

func (m *MultiSender) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(m.done)
	log = log.With("job", m.Name(), "instance", m.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		case <-ticker.C:
			log.Info("sent transactions", "from", m.Config.Address.Hex(), "total", totalSent)
		default:
		}

		repeatCount := 4

		wg := sync.WaitGroup{}

		for i := 0; i < repeatCount; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				nonce, err := client.PendingNonceAt(ctx, *m.Config.Address)
				if err != nil {
					log.Error("failed to get nonce", "error", err)
					return
				}

				randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

				tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, m.Config.GasPrice, nil)

				tx, err = types.SignTx(tx, types.NewEIP155Signer(m.Config.ChainID), m.Config.Key)
				if err != nil {
					log.Error("failed to sign transaction", "error", err)
					return
				}

				err = client.SendTransaction(ctx, tx)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						log.Error("failed to send transaction", "error", err)
						return
					}
					log.Error("failed to send transaction", "error", err)
					return
				}

				// now get the receipt
				receiptCount := 0
				for {
					receipt, err := client.TransactionReceipt(ctx, tx.Hash())
					if err != nil {
						if errors.Is(err, context.Canceled) {
							log.Error("context cancelled", "error", err)
							return
						}
						if errors.Is(err, ethereum.NotFound) {
							time.Sleep(500 * time.Millisecond)
							receiptCount++
							if receiptCount > 50 {
								log.Error("waited too long for receipt", "error", err)
								continue
							}
							continue
						}

						log.Error("failed to get transaction receipt", "error", err)
						continue
					}
					if receipt.Status == types.ReceiptStatusSuccessful {
						totalSent++
						break
					} else {
						log.Warn("transaction failed", "hash", tx.Hash().Hex())
						break
					}
				}
			}()
		}

		wg.Wait()
	}
}

func (m *MultiSender) WaitForStop() <-chan struct{} {
	return m.done
}

func (m *MultiSender) Name() string {
	return "multi-sender"
}

func (m *MultiSender) Instance() uint64 {
	return m.instance
}
