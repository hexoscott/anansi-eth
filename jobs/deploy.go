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
