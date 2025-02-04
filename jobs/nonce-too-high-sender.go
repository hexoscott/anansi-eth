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

type NonceTooHighSender struct {
	Config   NonceTooHighSenderConfig
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type NonceTooHighSenderConfig struct {
	JobConfig
}

func NewNonceTooHighSender(instance uint64) (*NonceTooHighSender, error) {
	return &NonceTooHighSender{
		Config:   NonceTooHighSenderConfig{},
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (n *NonceTooHighSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
}

func (n *NonceTooHighSender) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", n.Name(), "instance", n.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

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

			nonce, err := client.PendingNonceAt(context.Background(), *n.Config.Address)
			if err != nil {
				return err
			}

			nonce++

			for i := 0; i < 100; i++ {
				select {
				case <-n.quit:
					log.Info("received signal, stopping")
					close(n.done)
					return nil
				default:
				}

				randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

				tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, n.Config.GasPrice, nil)

				tx, err = types.SignTx(tx, types.NewEIP155Signer(n.Config.ChainID), n.Config.Key)
				if err != nil {
					return err
				}

				err = client.SendTransaction(context.Background(), tx)
				if err != nil {
					log.Error("failed to send transaction", "error", err)
					time.Sleep(1 * time.Second)
					continue
				}
				nonce++
				totalSent++
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

func (n *NonceTooHighSender) Stop() <-chan struct{} {
	close(n.quit)
	return n.done
}

func (n *NonceTooHighSender) Name() string {
	return "nonce-too-high-sender"
}
