package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type StateFiller struct {
	Config   StateFillerConfig
	done     chan struct{}
	instance uint64
}

type StateFillerConfig struct {
	JobConfig
}

// simple contract that puts 250 items into state as the constructor and allows for a call to deleteRandomState() to remove one at random
const byteCode = "0x608060405234801561001057600080fd5b50604051610687380380610687833981810160405281019061003291906100f6565b33600160006101000a81548173ffffffffffffffffffffffffffffffffffffffff021916908373ffffffffffffffffffffffffffffffffffffffff16021790555060005b818110156100b45760008190806001815401808255809150506001900390600052602060002001600090919091909150558080600101915050610076565b5050610123565b600080fd5b6000819050919050565b6100d3816100c0565b81146100de57600080fd5b50565b6000815190506100f0816100ca565b92915050565b60006020828403121561010c5761010b6100bb565b5b600061011a848285016100e1565b91505092915050565b610555806101326000396000f3fe608060405234801561001057600080fd5b50600436106100415760003560e01c80635929337c146100465780637b3118e1146100765780638da5cb5b14610080575b600080fd5b610060600480360381019061005b9190610228565b61009e565b60405161006d9190610264565b60405180910390f35b61007e6100c2565b005b6100886101c7565b60405161009591906102c0565b60405180910390f35b600081815481106100ae57600080fd5b906000526020600020016000915090505481565b6000808054905011610109576040517f08c379a000000000000000000000000000000000000000000000000000000000815260040161010090610338565b60405180910390fd5b60008080549050424433604051602001610125939291906103c1565b6040516020818303038152906040528051906020012060001c610148919061042d565b90506000600160008054905061015e919061048d565b8154811061016f5761016e6104c1565b5b90600052602060002001546000828154811061018e5761018d6104c1565b5b906000526020600020018190555060008054806101ae576101ad6104f0565b5b6001900381819060005260206000200160009055905550565b600160009054906101000a900473ffffffffffffffffffffffffffffffffffffffff1681565b600080fd5b6000819050919050565b610205816101f2565b811461021057600080fd5b50565b600081359050610222816101fc565b92915050565b60006020828403121561023e5761023d6101ed565b5b600061024c84828501610213565b91505092915050565b61025e816101f2565b82525050565b60006020820190506102796000830184610255565b92915050565b600073ffffffffffffffffffffffffffffffffffffffff82169050919050565b60006102aa8261027f565b9050919050565b6102ba8161029f565b82525050565b60006020820190506102d560008301846102b1565b92915050565b600082825260208201905092915050565b7f4e6f206d6f726520737461746520746f2064656c657465000000000000000000600082015250565b60006103226017836102db565b915061032d826102ec565b602082019050919050565b6000602082019050818103600083015261035181610315565b9050919050565b6000819050919050565b61037361036e826101f2565b610358565b82525050565b60008160601b9050919050565b600061039182610379565b9050919050565b60006103a382610386565b9050919050565b6103bb6103b68261029f565b610398565b82525050565b60006103cd8286610362565b6020820191506103dd8285610362565b6020820191506103ed82846103aa565b601482019150819050949350505050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601260045260246000fd5b6000610438826101f2565b9150610443836101f2565b925082610453576104526103fe565b5b828206905092915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b6000610498826101f2565b91506104a3836101f2565b92508282039050818111156104bb576104ba61045e565b5b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603260045260246000fd5b7f4e487b7100000000000000000000000000000000000000000000000000000000600052603160045260246000fdfea26469706673582212209181a690d99f01dfa24c398b7665a1d75805449645f4f57ecef4147edef95fb664736f6c6343000818003300000000000000000000000000000000000000000000000000000000000000fa"

// prehashed call to deleteRandomState()
const sig = "0x7b3118e1"

func NewStateFiller(instance uint64) (*StateFiller, error) {
	return &StateFiller{
		Config:   StateFillerConfig{},
		done:     make(chan struct{}),
		instance: instance,
	}, nil
}

func (g *StateFiller) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	g.Config.Address = address
	g.Config.Key = privateKey
	g.Config.ChainID = chainID
	g.Config.GasPrice = gasPrice
}

func (g *StateFiller) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(g.done)
	log = log.With("job", g.Name(), "instance", g.instance)

	for {
		select {
		case <-ctx.Done():
			log.Debug("received signal, stopping")
			return nil
		default:
		}
		nonce, err := client.PendingNonceAt(ctx, *g.Config.Address)
		if err != nil {
			return err
		}

		// first deploy the contract
		deployCode := common.FromHex(byteCode)
		deployTx := types.NewContractCreation(nonce, big.NewInt(0), 5995000, g.Config.GasPrice, deployCode)
		deployTx, err = types.SignTx(deployTx, types.NewEIP155Signer(g.Config.ChainID), g.Config.Key)
		if err != nil {
			return err
		}

		err = client.SendTransaction(ctx, deployTx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Debug("received signal, stopping")
				return nil
			}
			return err
		}

		var contractAddress common.Address
		receipt, err := waitForReceipt(ctx, client, deployTx.Hash())
		if err != nil {
			if errors.Is(err, context.Canceled) {
				log.Debug("received signal, stopping")
				return nil
			}
			return err
		}
		contractAddress = receipt.ContractAddress

		// now loop 240 times to slowly remove from the contract state and leave 10 items behind to slowly build up the state tree
		hashes := []common.Hash{}
		for i := 0; i < 240; i++ {
			nonce++
			tx := types.NewTransaction(nonce, contractAddress, big.NewInt(0), 30000, g.Config.GasPrice, common.FromHex(sig))
			tx, err = types.SignTx(tx, types.NewEIP155Signer(g.Config.ChainID), g.Config.Key)
			if err != nil {
				return err
			}

			err = client.SendTransaction(ctx, tx)
			if err != nil {
				return err
			}

			hashes = append(hashes, tx.Hash())
		}

		// now wait for all the transactions to be mined
		err = waitUntilMined(ctx, client, hashes)
		if err != nil {
			return err
		}
	}
}

func (g *StateFiller) WaitForStop() <-chan struct{} {
	return g.done
}

func (g *StateFiller) Name() string {
	return "state-filler"
}

func waitForReceipt(ctx context.Context, client *ethclient.Client, hash common.Hash) (*types.Receipt, error) {
	killswitch := time.NewTimer(1 * time.Minute)
	defer killswitch.Stop()

	for {
		select {
		case <-killswitch.C:
			return nil, errors.New("timeout waiting for receipt")
		case <-ctx.Done():
			return nil, errors.New("context cancelled")
		default:
		}

		receipt, err := client.TransactionReceipt(ctx, hash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				continue
			}
			return nil, err
		}

		return receipt, nil
	}
}

func waitUntilMined(ctx context.Context, client *ethclient.Client, hashes []common.Hash) error {
	for _, hash := range hashes {
		receipt, err := waitForReceipt(ctx, client, hash)
		if err != nil {
			return err
		}

		if receipt.Status != types.ReceiptStatusSuccessful {
			return errors.New("transaction failed")
		}
	}

	return nil
}

func (g *StateFiller) Instance() uint64 {
	return g.instance
}

func (g *StateFiller) NeedsFunding() bool {
	return true
}

func (g *StateFiller) WalletAddress() *common.Address {
	return g.Config.Address
}

func (g *StateFiller) GiveLoadStats() map[string]uint64 {
	return map[string]uint64{}
}

func (g *StateFiller) UpdateLoad(indicator LoadIndicator) {
}
