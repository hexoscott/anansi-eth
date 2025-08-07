package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type ERC20Job struct {
	Config   ERC20JobConfig
	done     chan struct{}
	instance uint64
}

type ERC20JobConfig struct {
	JobConfig
}

func NewERC20Job(instance uint64) (*ERC20Job, error) {
	return &ERC20Job{
		Config:   ERC20JobConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
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
	log.Info("parsed contract ABI successfully", "methods", len(contractABI.Methods), "events", len(contractABI.Events))

	bytecode := common.FromHex(forgeCompiled.Bytecode.Object)

	tx, receipt, err := deployContractWithEstimates(ctx, *g.Config.Address, bytecode, client, log, g.Config.Key, g.Config.ChainID)
	if err != nil {
		return err
	}

	log.Info("erc 20 contract deployed", "address", receipt.ContractAddress, "tx", tx.Hash().Hex())

	for {
		select {
		case <-ctx.Done():
			log.Info("received signal, stopping")
			return nil
		default:
		}

		// Example: Call the mint function using the parsed ABI
		mintAmount := big.NewInt(1000000000000000000) // 1 token (18 decimals)

		// Pack the mint function call data
		mintData, err := contractABI.Pack("mint", mintAmount)
		if err != nil {
			log.Error("failed to pack mint function call", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		tx, receipt, err := makeContractCall(ctx, client, log, *g.Config.Address, receipt.ContractAddress, mintData, g.Config.ChainID, g.Config.Key, true)
		if err != nil {
			log.Error("failed to make contract call", "error", err)
			time.Sleep(1 * time.Second)
			continue
		}

		log.Info("mint function call successful", "tx", tx.Hash().Hex(), "receipt", receipt.Status)

		// now share this out to 5 random addresses
		for i := 0; i < 5; i++ {
			receipient := randomAddress()

			// Pack the transfer function call data
			transferAmount := big.NewInt(100000000000000000) // 0.1 token
			transferData, err := contractABI.Pack("transfer", receipient, transferAmount)
			if err != nil {
				log.Error("failed to pack transfer function call", "error", err)
				continue
			}

			// Make the transfer call
			tx, transferReceipt, err := makeContractCall(ctx, client, log, *g.Config.Address, receipt.ContractAddress, transferData, g.Config.ChainID, g.Config.Key, true)
			if err != nil {
				log.Error("failed to transfer tokens", "error", err, "to", receipient.Hex())
				continue
			}

			log.Info("transfer successful", "to", receipient.Hex(), "status", transferReceipt.Status, "tx", tx.Hash().Hex())
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
