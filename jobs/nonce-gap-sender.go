package jobs

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type NonceGapSender struct {
	Config   NonceGapSenderConfig
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type NonceGapSenderConfig struct {
	JobConfig
}

func NewNonceGapSender(instance uint64) (*NonceGapSender, error) {
	return &NonceGapSender{
		Config:   NonceGapSenderConfig{},
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (n *NonceGapSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
}

func (n *NonceGapSender) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", n.Name(), "instance", n.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	masterNonce, err := client.PendingNonceAt(context.Background(), *n.Config.Address)
	if err != nil {
		return err
	}

	// create the gap
	masterNonce++

	randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

	for {
		select {
		case <-n.quit:
			log.Info("received signal, stopping")
			close(n.done)
			return nil
		default:
			select {
			case <-ticker.C:
				log.Info("sent transactions", "from", n.Config.Address.Hex(), "total", totalSent)
			default:
			}

			nonce := masterNonce

			for i := 0; i < 200; i++ {
				select {
				case <-n.quit:
					log.Info("received signal, stopping")
					close(n.done)
					return nil
				default:
				}

				tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, big.NewInt(2000000000), nil)

				tx, err = types.SignTx(tx, types.NewEIP155Signer(n.Config.ChainID), n.Config.Key)
				if err != nil {
					return err
				}

				err = client.SendTransaction(context.Background(), tx)
				if err != nil {
					if err.Error() != "ALREADY_EXISTS: already known" {
						log.Error("failed to send transaction", "error", err)
						time.Sleep(1 * time.Second)
						continue
					}
				}
				nonce++
				totalSent++
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (n *NonceGapSender) Stop() <-chan struct{} {
	close(n.quit)
	return n.done
}

func (n *NonceGapSender) Name() string {
	return "nonce-gap-sender"
}
