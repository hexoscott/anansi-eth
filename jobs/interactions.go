package jobs

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

func deployContractWithEstimates(
	ctx context.Context,
	from common.Address,
	bytecode []byte,
	client *ethclient.Client,
	log hclog.Logger,
	privateKey *ecdsa.PrivateKey,
	chainID *big.Int,
) (*types.Transaction, *types.Receipt, error) {
	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		return nil, nil, err
	}

	gasEstimate, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From: from,
		To:   nil,
		Data: bytecode,
	})
	if err != nil {
		log.Error("failed to estimate gas", "error", err)
		return nil, nil, err
	}

	// now add 10% to the gas estimate
	gasEstimate = gasEstimate * 110 / 100

	price, err := client.SuggestGasPrice(ctx)
	if err != nil {
		log.Error("failed to suggest gas price", "error", err)
		return nil, nil, err
	}

	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       nil,
		Value:    big.NewInt(0),
		Gas:      gasEstimate,
		GasPrice: price,
		Data:     bytecode,
	})

	tx, err = types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		return nil, nil, err
	}

	err = client.SendTransaction(ctx, tx)
	if err != nil {
		log.Error("failed to deploy contract", "error", err)
		return nil, nil, err
	}

	var receipt *types.Receipt
	for i := 0; i < 10; i++ {
		receipt, err = client.TransactionReceipt(ctx, tx.Hash())
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				time.Sleep(1 * time.Second)
				continue
			}

			log.Error("failed to get transaction receipt", "error", err)
			return nil, nil, err
		}

		if receipt.Status == types.ReceiptStatusSuccessful {
			break
		}
	}

	return tx, receipt, nil
}

func makeContractCall(
	ctx context.Context,
	client *ethclient.Client,
	log hclog.Logger,
	from common.Address,
	to common.Address,
	data []byte,
	chainId *big.Int,
	key *ecdsa.PrivateKey,
	waitOnReceipt bool,
) (*types.Transaction, *types.Receipt, error) {

	nonce, err := client.PendingNonceAt(ctx, from)
	if err != nil {
		log.Error("failed to get nonce", "error", err)
		return nil, nil, err
	}

	gasEstimate, err := client.EstimateGas(ctx, ethereum.CallMsg{
		From: from,
		To:   &to,
		Data: data,
	})
	if err != nil {
		log.Error("failed to estimate gas", "error", err)
		return nil, nil, err
	}

	suggestedPrice, err := client.SuggestGasPrice(ctx)
	if err != nil {
		log.Error("failed to suggest gas price", "error", err)
		return nil, nil, err
	}

	// now make the mint call
	tx := types.NewTx(&types.LegacyTx{
		Nonce:    nonce,
		To:       &to,
		Value:    big.NewInt(0),
		Gas:      gasEstimate,
		GasPrice: suggestedPrice,
		Data:     data,
	})

	tx, err = types.SignTx(tx, types.NewEIP155Signer(chainId), key)
	if err != nil {
		return nil, nil, err
	}

	err = client.SendTransaction(ctx, tx)
	if err != nil {
		return nil, nil, err
	}

	var receipt *types.Receipt
	if waitOnReceipt {
		for i := 0; i < 10; i++ {
			receipt, err = client.TransactionReceipt(ctx, tx.Hash())
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					time.Sleep(1 * time.Second)
					continue
				}

				return nil, nil, err
			}

			if receipt.Status == types.ReceiptStatusSuccessful {
				break
			}
		}
	}

	return tx, receipt, nil
}

func waitOnReceipts(ctx context.Context, client *ethclient.Client, transactions []*types.Transaction) ([]*types.Receipt, error) {
	result := make([]*types.Receipt, 0)
	for _, transaction := range transactions {
		for i := 0; i < 10; i++ {
			receipt, err := client.TransactionReceipt(ctx, transaction.Hash())
			if err != nil {
				if errors.Is(err, ethereum.NotFound) {
					time.Sleep(1 * time.Second)
					continue
				}

				return nil, err
			}

			if receipt.Status == types.ReceiptStatusSuccessful {
				result = append(result, receipt)
				break
			}
		}
	}
	return result, nil
}

func randomAddress() common.Address {
	randomBytes := make([]byte, 20)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return common.Address{}
	}
	return common.BytesToAddress(randomBytes)
}
