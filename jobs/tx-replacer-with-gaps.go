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

type TxReplacerWithGaps struct {
	Config   TxReplacerWithGapsConfig
	quit     chan struct{}
	done     chan struct{}
	instance uint64
}

type TxReplacerWithGapsConfig struct {
	JobConfig
}

func NewTxReplacerWithGaps(instance uint64) (*TxReplacerWithGaps, error) {
	return &TxReplacerWithGaps{
		Config:   TxReplacerWithGapsConfig{},
		quit:     make(chan struct{}),
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (n *TxReplacerWithGaps) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	n.Config.Address = address
	n.Config.Key = privateKey
	n.Config.ChainID = chainID
	n.Config.GasPrice = gasPrice
}

func (n *TxReplacerWithGaps) Run(client *ethclient.Client, log hclog.Logger) error {
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

			// get the current network nonce
			nonce, err := client.PendingNonceAt(context.Background(), *n.Config.Address)
			if err != nil {
				return err
			}

			log.Info("current nonce", "nonce", nonce)

			// create an initial gap
			gapNonce := nonce + 1

			// now spam 5k transactions into the queued pool
			for i := 0; i < 10; i++ {
				select {
				case <-n.quit:
					log.Info("received signal, stopping")
					close(n.done)
					return nil
				default:
				}

				randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

				tx := types.NewTransaction(gapNonce, randomAddress, big.NewInt(1000), 21000, n.Config.GasPrice, nil)

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
				gapNonce++
				totalSent++
			}

			time.Sleep(200 * time.Millisecond) // wait a little moment before continuing

			// now send another 5k transactions in without the gap and let it start replacing the previous ones whilst doing so
			for i := 0; i < 10; i++ {
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

			// now wait a moment before starting the process again
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func (n *TxReplacerWithGaps) Stop() <-chan struct{} {
	close(n.quit)
	return n.done
}

func (n *TxReplacerWithGaps) Name() string {
	return "tx-replacer-with-gaps"
}
