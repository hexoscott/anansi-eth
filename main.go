package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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
)

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

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Error("failed to dial rpc", "error", err)
		return
	}
	defer client.Close()

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

	for _, job := range allJobs {
		address, privateKey, err := createWallet()
		if err != nil {
			log.Error("failed to create wallet", "error", err)
			return
		}
		err = fundWallet(client, &parentAddress, parentKey, address, big.NewInt(1000000000000000000), chainID)
		if err != nil {
			log.Error("failed to fund wallet", "error", err)
			return
		}
		job.SetWallet(address, privateKey, chainID)
		go func(j jobs.Job, address *common.Address, privateKey *ecdsa.PrivateKey) {
			log.Info("starting job", "name", j.Name(), "address", address.Hex())
			err = j.Run(client, log)
			if err != nil {
				log.Error("problem running job", "name", j.Name(), "error", err)
			}
		}(job, address, privateKey)
	}

	// now wait for sigint or sigterm
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch

	timeout, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, job := range allJobs {
		s := job.Stop()
		select {
		case <-timeout.Done():
			log.Error("timed out waiting for job to finish", "name", job.Name())
		case <-s:
			log.Info("job finished", "name", job.Name())
		}
	}
}

func createJobs() ([]jobs.Job, error) {
	result := []jobs.Job{}

	// create 10 already exists jobs
	for i := 0; i < 10; i++ {
		alreadyExists, err := jobs.NewAlreadyExists(uint64(i))
		if err != nil {
			return result, err
		}
		result = append(result, alreadyExists)
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
	parentAddress *common.Address,
	parentKey *ecdsa.PrivateKey,
	destAddress *common.Address,
	value *big.Int,
	chainID *big.Int,
) error {
	nonce, err := client.PendingNonceAt(context.Background(), *parentAddress)
	if err != nil {
		return err
	}

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return err
	}

	tx := types.NewTransaction(nonce, *destAddress, value, 21000, gasPrice, []byte{})

	tx, err = types.SignTx(tx, types.NewEIP155Signer(chainID), parentKey)
	if err != nil {
		return err
	}

	return client.SendTransaction(context.Background(), tx)
}
