package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
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
	jobFile    string
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
	flag.StringVar(&jobFile, "job-file", "./config/all.json", "the json file used to define the jobs to run")
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

	if jobFile == "" {
		log.Error("job-file flag is missing")
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

	allJobs, err := createJobs(jobFile, parentKey, &parentAddress)
	if err != nil {
		log.Error("failed to create jobs", "error", err)
		return
	}

	monitor, err := jobs.NewMonitor(allJobs)
	if err != nil {
		log.Error("error creating monitor job")
		return
	}
	allJobs = append(allJobs, monitor)

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

	fundingNeeded := 0
	for _, job := range allJobs {
		if job.NeedsFunding() {
			fundingNeeded++
		}
	}

	fundingHashes, walletKeys, err := fundWallets(ctx, client, parentNonce, &parentAddress, parentKey, fundingNeeded, fundAmount, chainID, gasPrice, log)
	if err != nil {
		log.Error("failed to fund wallets", "error", err)
		return
	}
	log.Info("funded wallets", "count", len(walletKeys))

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

	// monitor does not need funding

	for _, job := range allJobs {
		if job.NeedsFunding() {
			wallet := walletKeys[0]
			walletKeys = walletKeys[1:]
			job.SetWallet(wallet.Address, wallet.PrivateKey, chainID, gasPrice)
		}
		go func(j jobs.Job) {
			log.Info("starting job", "name", j.Name(), "address", j.WalletAddress().Hex())
			err = j.Run(ctx, client, log)
			if err != nil {
				log.Error("problem running job", "name", j.Name(), "error", err)
			}
		}(job)
	}

	// now wait for sigint or sigterm
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT, syscall.SIGUSR1)
	sig := <-ch
	log.Debug("received signal", "signal", sig.String())

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

func createJobs(jobFileLocation string, parentKey *ecdsa.PrivateKey, parentAddress *common.Address) ([]jobs.Job, error) {
	jobFile, err := parseJobFile(jobFileLocation)
	if err != nil {
		return nil, err
	}

	result := []jobs.Job{}
	var instance uint64 = 0

	for _, job := range jobFile.Jobs {
		switch job.Type {
		case "GoodSender":
			for i := 0; i < job.Count; i++ {
				goodSender, err := jobs.NewGoodSender(instance)
				if err != nil {
					return nil, err
				}
				result = append(result, goodSender)
				instance++
			}

		case "AlreadyExists":
			for i := 0; i < job.Count; i++ {
				alreadyExists, err := jobs.NewAlreadyExists(instance)
				if err != nil {
					return result, err
				}
				result = append(result, alreadyExists)
				instance++
			}

		case "NoWaitSender":
			for i := 0; i < job.Count; i++ {
				noWaitSender, err := jobs.NewNoWaitSender(instance)
				if err != nil {
					return result, err
				}
				result = append(result, noWaitSender)
				instance++
			}

		case "NonceOverlapSender":
			for i := 0; i < job.Count; i++ {
				nonceOverlapSender, err := jobs.NewNonceOverlapSender(instance, parentKey, parentAddress)
				if err != nil {
					return result, err
				}
				result = append(result, nonceOverlapSender)
				instance++
			}

		case "NonceGapSender":
			for i := 0; i < job.Count; i++ {
				nonceGapSender, err := jobs.NewNonceGapSender(instance)
				if err != nil {
					return result, err
				}
				result = append(result, nonceGapSender)
				instance++
			}

		case "MultiSender":
			for i := 0; i < job.Count; i++ {
				multiSender, err := jobs.NewMultiSender(instance)
				if err != nil {
					return result, err
				}
				result = append(result, multiSender)
				instance++
			}

		case "StateFiller":
			for i := 0; i < job.Count; i++ {
				stateFiller, err := jobs.NewStateFiller(instance)
				if err != nil {
					return result, err
				}
				result = append(result, stateFiller)
				instance++
			}

		case "TxReplacerWithGaps":
			for i := 0; i < job.Count; i++ {
				txReplacerWithGaps, err := jobs.NewTxReplacerWithGaps(instance)
				if err != nil {
					return result, err
				}
				result = append(result, txReplacerWithGaps)
				instance++
			}

		case "GasPriceReader":
			for i := 0; i < job.Count; i++ {
				gasPriceReader, err := jobs.NewGasPriceReader(instance)
				if err != nil {
					return result, err
				}
				result = append(result, gasPriceReader)
				instance++
			}

		case "RepeatReceiptsJob":
			for i := 0; i < job.Count; i++ {
				repeatReceiptsJob, err := jobs.NewRepeatReceiptsJob(instance)
				if err != nil {
					return result, err
				}
				result = append(result, repeatReceiptsJob)
				instance++
			}

		case "ERC20Job":
			for i := 0; i < job.Count; i++ {
				erc20Job, err := jobs.NewERC20Job(instance)
				if err != nil {
					return result, err
				}
				result = append(result, erc20Job)
				instance++
			}
		}
	}

	return result, nil
}

func parseJobFile(file string) (*jobs.JobFile, error) {
	contents, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	result := &jobs.JobFile{}
	err = json.Unmarshal(contents, &result)
	if err != nil {
		return nil, err
	}
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

		if !gasless {
			hash, err := fundWallet(ctx, client, nonce, parentAddress, parentKey, address, amount, chainID, gasPrice)
			if err != nil {
				return nil, nil, err
			}
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
					if killSwitch > 50 {
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
