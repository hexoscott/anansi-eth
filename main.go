package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"flag"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"

	"github.com/hexoscott/anansi-eth/jobs"
)

var (
	privateKey string
	rpcURL     string

	fundAmount = big.NewInt(1000000000000000000)
)

type Wallet struct {
	Address    *common.Address
	PrivateKey *ecdsa.PrivateKey
}

func main() {
	flag.StringVar(&privateKey, "private-key", "", "main account private key used to fund accounts for the jobs")
	flag.StringVar(&rpcURL, "rpc-url", "", "rpc url")
	flag.Parse()

	log := hclog.New(&hclog.LoggerOptions{
		Level: hclog.LevelFromString("INFO"),
	})

	if privateKey == "" {
		log.Error("private-key is required")
		return
	}

	if rpcURL == "" {
		log.Error("rpc-url is required")
		return
	}

	parentKey, err := crypto.HexToECDSA(strings.TrimPrefix(privateKey, "0x"))
	if err != nil {
		log.Error("failed to convert private key", "error", err)
		return
	}
	parentAddress := crypto.PubkeyToAddress(parentKey.PublicKey)

	log.Info("parent address", "address", parentAddress.Hex())

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Error("failed to dial rpc", "error", err)
		return
	}
	defer client.Close()

	parentNonce, err := client.PendingNonceAt(context.Background(), parentAddress)
	if err != nil {
		log.Error("failed to get parent nonce", "error", err)
		return
	}

	allJobs, err := createJobs()
	if err != nil {
		log.Error("failed to create jobs", "error", err)
		return
	}

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		log.Error("failed to get chain id", "error", err)
		return
	}

	fundingHashes, walletKeys, err := fundWallets(client, parentNonce, &parentAddress, parentKey, len(allJobs), fundAmount, chainID, log)
	if err != nil {
		log.Error("failed to fund wallets", "error", err)
		return
	}

	allMined, err := waitUntilMined(client, fundingHashes)
	if err != nil {
		log.Error("failed to wait for funding transactions to be mined", "error", err)
		return
	}

	if allMined {
		log.Info("all funding transactions mined")
	} else {
		log.Error("failed to mine all funding transactions")
		return
	}

	for i, job := range allJobs {
		wallet := walletKeys[i]
		job.SetWallet(wallet.Address, wallet.PrivateKey, chainID)
		go func(j jobs.Job, wallet Wallet) {
			log.Info("starting job", "name", j.Name(), "address", wallet.Address.Hex())
			err = j.Run(client, log)
			if err != nil {
				log.Error("problem running job", "name", j.Name(), "error", err)
			}
		}(job, wallet)
	}

	// now wait for sigint or sigterm
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doneChans := make([]<-chan struct{}, len(allJobs))
	for i, job := range allJobs {
		doneChans[i] = job.Stop()
	}

	for i, doneChan := range doneChans {
		select {
		case <-timeout.Done():
			log.Error("timed out waiting for jobs to finish")
			return
		case <-doneChan:
			log.Info("job finished", "name", allJobs[i].Name())
		}
	}
}

func createJobs() ([]jobs.Job, error) {
	result := []jobs.Job{}

	// create 10 already exists jobs
	for i := 0; i < 50; i++ {
		alreadyExists, err := jobs.NewAlreadyExists(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, alreadyExists)
	}

	// create 10 good sender jobs
	for i := 0; i < 600; i++ {
		goodSender, err := jobs.NewGoodSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, goodSender)
	}

	for i := 0; i < 50; i++ {
		noWaitSender, err := jobs.NewNoWaitSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, noWaitSender)
	}

	for i := 0; i < 50; i++ {
		nonceGapSender, err := jobs.NewNonceGapSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, nonceGapSender)
	}

	monitor, err := jobs.NewMonitor()
	if err != nil {
		return result, err
	}
	result = append(result, monitor)

	return result, nil
}

func createWallet() (*common.Address, *ecdsa.PrivateKey, error) {
	// generate a random private key
	privateKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, err
	}

	address := crypto.PubkeyToAddress(privateKey.PublicKey)

	return &address, privateKey, nil
}

func fundWallet(
	client *ethclient.Client,
	nonce uint64,
	parentAddress *common.Address,
	parentKey *ecdsa.PrivateKey,
	destAddress *common.Address,
	value *big.Int,
	chainID *big.Int,
) (common.Hash, error) {
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return common.Hash{}, err
	}

	tx := types.NewTransaction(nonce, *destAddress, value, 21000, gasPrice, []byte{})

	tx, err = types.SignTx(tx, types.NewEIP155Signer(chainID), parentKey)
	if err != nil {
		return common.Hash{}, err
	}

	err = client.SendTransaction(context.Background(), tx)
	if err != nil {
		return common.Hash{}, err
	}

	return tx.Hash(), nil
}

func fundWallets(
	client *ethclient.Client,
	nonce uint64,
	parentAddress *common.Address,
	parentKey *ecdsa.PrivateKey,
	count int,
	amount *big.Int,
	chainID *big.Int,
	log hclog.Logger,
) ([]common.Hash, []Wallet, error) {
	wallets := make([]Wallet, count)
	hashes := make([]common.Hash, count)
	for i := 0; i < count; i++ {
		address, privateKey, err := createWallet()
		if err != nil {
			return nil, nil, err
		}
		wallets[i] = Wallet{
			Address:    address,
			PrivateKey: privateKey,
		}
		log.Info("created wallet", "address", address.Hex())
		hash, err := fundWallet(client, nonce, parentAddress, parentKey, address, amount, chainID)
		if err != nil {
			return nil, nil, err
		}
		log.Info("funded wallet", "address", address.Hex(), "hash", hash.Hex())
		hashes[i] = hash
		nonce++
	}

	return hashes, wallets, nil
}

func waitUntilMined(client *ethclient.Client, hashes []common.Hash) (bool, error) {
	allMined := false
	killSwitch := 0
	for _, hash := range hashes {
		for {
			receipt, err := client.TransactionReceipt(context.Background(), hash)
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					time.Sleep(1 * time.Second)
					killSwitch++
					if killSwitch > 10 {
						return false, errors.New("failed to mine all funding transactions")
					}
					continue
				} else {
					return false, err
				}
			}
			if receipt.Status == types.ReceiptStatusSuccessful {
				allMined = true
				break
			} else {
				return false, errors.New("last mining transaction failed")
			}
		}
	}
	return allMined, nil
}
