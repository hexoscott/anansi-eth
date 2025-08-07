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

type NoWaitSender struct {
	Config   NoWaitSenderConfig
	done     chan struct{}
	instance uint64
}

type NoWaitSenderConfig struct {
	JobConfig
}

func NewNoWaitSender(instance uint64) (*NoWaitSender, error) {
	return &NoWaitSender{
		Config:   NoWaitSenderConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (n *NoWaitSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
	n.Config.GasPrice = gasPrice
}

func (n *NoWaitSender) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(n.done)
	log = log.With("job", n.Name(), "instance", n.instance)

	totalSent := 0

	for {
		select {
		case <-ctx.Done():
			log.Debug("received signal, stopping")
			return nil
		default:
		}

		nonce, err := client.PendingNonceAt(ctx, *n.Config.Address)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Debug("received signal, stopping")
				return nil
			}
			return err
		}

		for i := 0; i < 100; i++ {
			select {
			case <-ctx.Done():
				log.Debug("received signal, stopping")
				return nil
			default:
			}

			randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

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
				log.Error("failed to send transaction", "error", err)
				continue
			}
			nonce++
			totalSent++
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func (n *NoWaitSender) WaitForStop() <-chan struct{} {
	return n.done
}

func (n *NoWaitSender) Name() string {
	return "no-wait-sender"
}

func (n *NoWaitSender) Instance() uint64 {
	return n.instance
}

func (n *NoWaitSender) NeedsFunding() bool {
	return true
}

func (n *NoWaitSender) WalletAddress() *common.Address {
	return n.Config.Address
}

func (n *NoWaitSender) GiveLoadStats() map[string]uint64 {
	return map[string]uint64{}
}

func (n *NoWaitSender) UpdateLoad(indicator LoadIndicator) {
}
