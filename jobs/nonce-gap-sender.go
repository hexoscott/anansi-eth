package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
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
	done     chan struct{}
	instance uint64
}

type NonceGapSenderConfig struct {
	JobConfig
}

func NewNonceGapSender(instance uint64) (*NonceGapSender, error) {
	return &NonceGapSender{
		Config:   NonceGapSenderConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (n *NonceGapSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
	n.Config.GasPrice = gasPrice
}

func (n *NonceGapSender) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(n.done)
	log = log.With("job", n.Name(), "instance", n.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	masterNonce, err := client.PendingNonceAt(ctx, *n.Config.Address)
	if err != nil {
		return err
	}

	// create the gap
	masterNonce++

	randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		case <-ticker.C:
			log.Info("sent transactions", "from", n.Config.Address.Hex(), "total", totalSent)
		default:
		}

		nonce := masterNonce

		for i := 0; i < 200; i++ {
			select {
			case <-ctx.Done():
				log.Info("received signal, stopping")
				return nil
			default:
			}

			tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, n.Config.GasPrice, nil)

			tx, err = types.SignTx(tx, types.NewEIP155Signer(n.Config.ChainID), n.Config.Key)
			if err != nil {
				return err
			}

			err = client.SendTransaction(ctx, tx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Error("failed to send transaction", "error", err)
					return nil
				}
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

func (n *NonceGapSender) WaitForStop() <-chan struct{} {
	return n.done
}

func (n *NonceGapSender) Name() string {
	return "nonce-gap-sender"
}

func (n *NonceGapSender) Instance() uint64 {
	return n.instance
}

func (n *NonceGapSender) NeedsFunding() bool {
	return true
}

func (n *NonceGapSender) WalletAddress() *common.Address {
	return n.Config.Address
}
