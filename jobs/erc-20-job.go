package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"os"
	"time"

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

		// nonce, err := client.PendingNonceAt(ctx, *g.Config.Address)
		// if err != nil {
		// 	return err
		// }

		// randomAddress := common.HexToAddress(fmt.Sprintf("0x%x", rand.Intn(1000000000000000000)))

		time.Sleep(1 * time.Second)
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
