package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type ERC20Job struct {
	Config    ERC20JobConfig
	done      chan struct{}
	instance  uint64
	mints     atomic.Uint64
	transfers atomic.Uint64
}

type ERC20JobConfig struct {
	JobConfig
}

func NewERC20Job(instance uint64) (*ERC20Job, error) {
	job := &ERC20Job{
		Config:   ERC20JobConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}
	job.mints.Store(1)
	job.transfers.Store(5)

	return job, nil
}

func (g *ERC20Job) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	g.Config.Address = address
	g.Config.Key = privateKey
	g.Config.ChainID = chainID
	g.Config.GasPrice = gasPrice
}

func (g *ERC20Job) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(g.done)
	log = log.With("job", g.Name(), "instance", g.instance)

	// load up the contract byte code so we can deploy it
	compiledContract, err := os.ReadFile("./contracts/out/SimpleERC20.sol/ERC20.json")
	if err != nil {
		return err
	}

	var forgeCompiled ForgeCompiled
	err = json.Unmarshal(compiledContract, &forgeCompiled)
	if err != nil {
		return err
	}

	// Extract ABI from the forge compilation output and parse it with go-ethereum
	abiBytes, err := json.Marshal(forgeCompiled.Abi)
	if err != nil {
		return err
	}

	contractABI, err := abi.JSON(strings.NewReader(string(abiBytes)))
	if err != nil {
		return err
	}

	bytecode := common.FromHex(forgeCompiled.Bytecode.Object)

	tx, receipt, err := deployContractWithEstimates(ctx, *g.Config.Address, bytecode, client, log, g.Config.Key, g.Config.ChainID)
	if err != nil {
		return err
	}

	log.Debug("erc 20 contract deployed", "address", receipt.ContractAddress, "tx", tx.Hash().Hex())

	for {
		select {
		case <-ctx.Done():
			log.Debug("received signal, stopping")
			return nil
		default:
		}

		// Example: Call the mint function using the parsed ABI
		mintAmount := big.NewInt(1000000000000000000) // 1 token (18 decimals)

		mintData, err := contractABI.Pack("mint", mintAmount)
		if err != nil {
			log.Error("failed to pack mint function call", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		transactions := make([]*types.Transaction, 0)
		for i := 0; i < int(g.mints.Load()); i++ {
			tx, _, err := makeContractCall(ctx, client, log, *g.Config.Address, receipt.ContractAddress, mintData, g.Config.ChainID, g.Config.Key, false)
			if err != nil {
				log.Error("failed to make contract call", "error", err)
				return err
			}
			transactions = append(transactions, tx)
		}

		_, err = waitOnReceipts(ctx, client, transactions)
		if err != nil {
			return err
		}

		// now share this out to some random addresses
		transactions = transactions[:0]
		for i := 0; i < int(g.transfers.Load()); i++ {
			receipient := randomAddress()

			// Pack the transfer function call data
			transferAmount := big.NewInt(100000000000000000) // 0.1 token
			transferData, err := contractABI.Pack("transfer", receipient, transferAmount)
			if err != nil {
				log.Error("failed to pack transfer function call", "error", err)
				continue
			}

			// Make the transfer call
			tx, _, err := makeContractCall(ctx, client, log, *g.Config.Address, receipt.ContractAddress, transferData, g.Config.ChainID, g.Config.Key, false)
			if err != nil {
				log.Error("failed to transfer tokens", "error", err, "to", receipient.Hex())
				continue
			}
			transactions = append(transactions, tx)
		}

		_, err = waitOnReceipts(ctx, client, transactions)
		if err != nil {
			return err
		}
	}
}

func (g *ERC20Job) WaitForStop() <-chan struct{} {
	return g.done
}

func (g *ERC20Job) Name() string {
	return "erc-20-job"
}

func (g *ERC20Job) Instance() uint64 {
	return g.instance
}

func (g *ERC20Job) NeedsFunding() bool {
	return true
}

func (g *ERC20Job) WalletAddress() *common.Address {
	return g.Config.Address
}

func (g *ERC20Job) GiveLoadStats() map[string]uint64 {
	return map[string]uint64{
		"mints":     g.mints.Load(),
		"transfers": g.transfers.Load(),
	}
}

func (g *ERC20Job) UpdateLoad(indicator LoadIndicator) {
	switch indicator {
	case LoadIncrease:
		g.mints.Add(1)
		g.transfers.Add(1)
	case LoadDecrease:
		currentMints := g.mints.Load()
		if currentMints > 1 {
			g.mints.Store(currentMints - 1)
		}
		currentTransfers := g.transfers.Load()
		if currentTransfers > 1 {
			g.transfers.Store(currentTransfers - 1)
		}
	}
}
