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

type NonceOverlapSender struct {
	Config        NonceOverlapSenderConfig
	done          chan struct{}
	instance      uint64
	masterKey     *ecdsa.PrivateKey
	masterAddress *common.Address
}

type NonceOverlapSenderConfig struct {
	JobConfig
}

func NewNonceOverlapSender(instance uint64, masterKey *ecdsa.PrivateKey, masterAddress *common.Address) (*NonceOverlapSender, error) {
	return &NonceOverlapSender{
		Config:        NonceOverlapSenderConfig{},
		done:          make(chan struct{}),
		instance:      instance,
		masterKey:     masterKey,
		masterAddress: masterAddress,
	}, nil
}

func (n *NonceOverlapSender) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
	n.Config.GasPrice = gasPrice
}

func (n *NonceOverlapSender) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(n.done)
	log = log.With("job", n.Name(), "instance", n.instance)

	totalSent := 0

	ticker := time.NewTicker(logInterval)
	defer ticker.Stop()

	randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		case <-ticker.C:
			log.Info("sent transactions", "from", n.masterAddress.Hex(), "total", totalSent)
		default:
		}

		nonce, err := client.PendingNonceAt(ctx, *n.masterAddress)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Info("received signal, stopping")
				return nil
			}
			return err
		}

		randomChange := rand.Intn(3)
		randomUpDown := rand.Intn(2)
		if randomUpDown == 0 {
			nonce += uint64(randomChange)
		} else {
			nonce -= uint64(randomChange)
		}

		for i := 0; i < 5; i++ {
			select {
			case <-ctx.Done():
				log.Info("received signal, stopping")
				return nil
			default:
			}

			tx := types.NewTransaction(nonce, randomAddress, big.NewInt(1000), 21000, n.Config.GasPrice, nil)

			tx, err = types.SignTx(tx, types.NewEIP155Signer(n.Config.ChainID), n.masterKey)
			if err != nil {
				return err
			}

			err = client.SendTransaction(ctx, tx)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Info("received signal, stopping")
					return nil
				}
				if err.Error() != "ALREADY_EXISTS: already known" {
					log.Error("failed to send transaction", "error", err)
					continue
				}
			}
			nonce++
			totalSent++
			randomSleep := rand.Intn(3)
			time.Sleep(time.Duration(randomSleep) * time.Millisecond)
		}
	}
}

func (n *NonceOverlapSender) WaitForStop() <-chan struct{} {
	return n.done
}

func (n *NonceOverlapSender) Name() string {
	return "nonce-overlap-sender"
}

func (n *NonceOverlapSender) Instance() uint64 {
	return n.instance
}
