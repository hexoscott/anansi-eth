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
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type GoodSenderConfig struct {
	JobConfig
}

func NewGoodSender(instance uint64) (*GoodSender, error) {
	return &GoodSender{
		Config:   GoodSenderConfig{},
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (g *GoodSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int) {
	g.Config.Address = address
	g.Config.Key = privateKey
	g.Config.ChainID = chainID
}

func (g *GoodSender) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", g.Name(), "instance", g.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	for {
		select {
		case <-g.quit:
			log.Info("received signal, stopping")
			close(g.done)
			return nil
		default:
			nonce, err := client.PendingNonceAt(context.Background(), *g.Config.Address)
			if err != nil {
				return err
			}

			randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

			tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, big.NewInt(2000000000), nil)

			tx, err = types.SignTx(tx, types.NewEIP155Signer(g.Config.ChainID), g.Config.Key)
			if err != nil {
				return err
			}

			err = client.SendTransaction(context.Background(), tx)
			if err != nil {
				log.Error("failed to send transaction", "error", err)
				time.Sleep(1 * time.Second)
				continue
			}

			// now get the receipt
			receiptCount := 0
			for {
				select {
				case <-g.quit:
					log.Info("received signal, stopping")
					close(g.done)
					return nil
				default:
				}

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

			select {
			case <-ticker.C:
				log.Info("sent transactions", "from", g.Config.Address.Hex(), "total", totalSent)
			default:
			}
		}
	}
}

func (g *GoodSender) Stop() <-chan struct{} {
	close(g.quit)
	return g.done
}

func (g *GoodSender) Name() string {
	return "good-sender"
}
