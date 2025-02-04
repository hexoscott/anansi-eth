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
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type MultiSenderConfig struct {
	JobConfig
}

func NewMultiSender(instance uint64) (*MultiSender, error) {
	return &MultiSender{
		Config:   MultiSenderConfig{},
		quit:     make(chan struct{}),
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

func (m *MultiSender) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", m.Name(), "instance", m.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.quit:
			log.Info("received signal, stopping")
			close(m.done)
			return nil
		default:

			select {
			case <-m.quit:
				log.Info("received signal, stopping")
				close(m.done)
				return nil
			default:
			}

			repeatCount := 4

			wg := sync.WaitGroup{}

			for i := 0; i < repeatCount; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					nonce, err := client.PendingNonceAt(context.Background(), *m.Config.Address)
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

					err = client.SendTransaction(context.Background(), tx)
					if err != nil {
						log.Error("failed to send transaction", "error", err)
						return
					}

					// now get the receipt
					receiptCount := 0
					for {

						receipt, err := client.TransactionReceipt(context.Background(), tx.Hash())
						if err != nil {
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
				}()
			}

			wg.Wait()

			select {
			case <-ticker.C:
				log.Info("sent transactions", "from", m.Config.Address.Hex(), "total", totalSent)
			default:
			}
		}
	}
}

func (m *MultiSender) Stop() <-chan struct{} {
	close(m.quit)
	return m.done
}

func (m *MultiSender) Name() string {
	return "multi-sender"
}
