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
	gasless    bool
	chainArg   uint64

	fundAmount = big.NewInt(0)
)

func init() {
	fundAmount.SetString("90000000000000000000", 10)
}

type Wallet struct {
	Address    *common.Address
	PrivateKey *ecdsa.PrivateKey
}

func main() {
	flag.StringVar(&privateKey, "private-key", "", "main account private key used to fund accounts for the jobs")
	flag.StringVar(&rpcURL, "rpc-url", "", "rpc url")
	flag.BoolVar(&gasless, "gasless", false, "use gasless transactions")
	flag.Uint64Var(&chainArg, "chain-id", 0, "chain id")
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

	ctx, cancelJobs := context.WithCancel(context.Background())

	parentNonce, err := client.PendingNonceAt(ctx, parentAddress)
	if err != nil {
		log.Error("failed to get parent nonce", "error", err)
		return
	}

	gasPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		log.Error("failed to suggest gas price", "error", err)
		return
	}
	log.Info("suggested gas price", "price", gasPrice)

	allJobs, err := createJobs(parentKey, &parentAddress)
	if err != nil {
		log.Error("failed to create jobs", "error", err)
		return
	}

	var chainID *big.Int
	if chainArg == 0 {
		chainID, err = client.NetworkID(ctx)
		if err != nil {
			log.Error("failed to get chain id", "error", err)
			return
		}
	} else {
		chainID = big.NewInt(int64(chainArg))
	}

	fundingHashes, walletKeys, err := fundWallets(ctx, client, parentNonce, &parentAddress, parentKey, len(allJobs), fundAmount, chainID, gasPrice, log)
	if err != nil {
		log.Error("failed to fund wallets", "error", err)
		return
	}

	if !gasless {
		allMined, err := waitUntilMined(ctx, client, fundingHashes)
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
	}

	for i, job := range allJobs {
		wallet := walletKeys[i]
		job.SetWallet(wallet.Address, wallet.PrivateKey, chainID, gasPrice)
		go func(j jobs.Job, wallet Wallet) {
			log.Info("starting job", "name", j.Name(), "address", wallet.Address.Hex())
			err = j.Run(ctx, client, log)
			if err != nil {
				log.Error("problem running job", "name", j.Name(), "error", err)
			}
		}(job, wallet)
	}

	// now wait for sigint or sigterm
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1)
	sig := <-ch
	log.Info("received signal", "signal", sig.String())

	cancelJobs()

	timeout, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	for _, job := range allJobs {
		log.Info("waiting for job to finish", "name", job.Name(), "instance", job.Instance())
		select {
		case <-timeout.Done():
			log.Error("timed out waiting for jobs to finish")
			return
		case <-job.WaitForStop():
			log.Info("job finished", "name", job.Name())
		}
	}
}

func createJobs(parentKey *ecdsa.PrivateKey, parentAddress *common.Address) ([]jobs.Job, error) {
	result := []jobs.Job{}

	for i := 0; i < 5; i++ {
		alreadyExists, err := jobs.NewAlreadyExists(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, alreadyExists)
	}

	for i := 0; i < 20; i++ {
		goodSender, err := jobs.NewGoodSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, goodSender)
	}

	for i := 0; i < 20; i++ {
		noWaitSender, err := jobs.NewNoWaitSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, noWaitSender)
	}

	for i := 0; i < 5; i++ {
		nonceOverlapSender, err := jobs.NewNonceOverlapSender(uint64(i), parentKey, parentAddress)
		if err != nil {
			return result, err
		}
		result = append(result, nonceOverlapSender)
	}

	for i := 0; i < 5; i++ {
		nonceGapSender, err := jobs.NewNonceGapSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, nonceGapSender)
	}

	for i := 0; i < 10; i++ {
		multiSender, err := jobs.NewMultiSender(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, multiSender)
	}

	for i := 0; i < 5; i++ {
		stateFiller, err := jobs.NewStateFiller(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, stateFiller)
	}

	for i := 0; i < 5; i++ {
		txReplacerWithGaps, err := jobs.NewTxReplacerWithGaps(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, txReplacerWithGaps)
	}

	for i := 0; i < 5; i++ {
		gasPriceReader, err := jobs.NewGasPriceReader(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, gasPriceReader)
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
	ctx context.Context,
	client *ethclient.Client,
	nonce uint64,
	parentAddress *common.Address,
	parentKey *ecdsa.PrivateKey,
	destAddress *common.Address,
	value *big.Int,
	chainID *big.Int,
	gasPrice *big.Int,
) (common.Hash, error) {
	tx := types.NewTransaction(nonce, *destAddress, value, 21000, gasPrice, []byte{})

	tx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), parentKey)
	if err != nil {
		return common.Hash{}, err
	}

	err = client.SendTransaction(ctx, tx)
	if err != nil {
		return common.Hash{}, err
	}

	return tx.Hash(), nil
}

func fundWallets(
	ctx context.Context,
	client *ethclient.Client,
	nonce uint64,
	parentAddress *common.Address,
	parentKey *ecdsa.PrivateKey,
	count int,
	amount *big.Int,
	chainID *big.Int,
	gasPrice *big.Int,
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

		if !gasless {
			hash, err := fundWallet(ctx, client, nonce, parentAddress, parentKey, address, amount, chainID, gasPrice)
			if err != nil {
				return nil, nil, err
			}
			log.Info("funded wallet", "address", address.Hex(), "hash", hash.Hex())
			hashes[i] = hash
			nonce++
		}
	}

	return hashes, wallets, nil
}

func waitUntilMined(ctx context.Context, client *ethclient.Client, hashes []common.Hash) (bool, error) {
	allMined := false
	killSwitch := 0
	for _, hash := range hashes {
		for {
			receipt, err := client.TransactionReceipt(ctx, hash)
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
