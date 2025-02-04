package jobs

import (
	"crypto/ecdsa"
	"encoding/json"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

type Monitor struct {
	Job
	quit chan struct{}
	done chan struct{}
}

func NewMonitor() (*Monitor, error) {
	return &Monitor{
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}, nil
}

func (j *Monitor) Name() string {
	return "monitor"
}

func (j *Monitor) Run(client *ethclient.Client, log hclog.Logger) error {
	log = log.With("job", j.Name())
	for {
		select {
		case <-j.quit:
			j.done <- struct{}{}
			return nil
		default:
			var result json.RawMessage
			err := client.Client().Call(&result, "txpool_status")
			if err != nil {
				log.Error("failed to call txpool_status", "error", err)
				time.Sleep(3 * time.Second)
				continue
			}
			var status PoolStatus
			if err := json.Unmarshal(result, &status); err != nil {
				log.Error("failed to unmarshal txpool_status", "error", err)
				time.Sleep(3 * time.Second)
				continue
			}
			baseFee, pending, queued, err := decodePoolStatus(status)
			if err != nil {
				log.Error("failed to decode pool status", "error", err)
				time.Sleep(3 * time.Second)
				continue
			}
			log.Info("txpool_status", "baseFee", baseFee, "pending", pending, "queued", queued)
			time.Sleep(3 * time.Second)
		}
	}
}

func (j *Monitor) Stop() <-chan struct{} {
	close(j.quit)
	return j.done
}

func (j *Monitor) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	// no op
}

type PoolStatus struct {
	BaseFee string `json:"baseFee"`
	Pending string `json:"pending"`
	Queued  string `json:"queued"`
}

func decodePoolStatus(status PoolStatus) (uint64, uint64, uint64, error) {
	baseFee, err := hexutil.DecodeUint64(status.BaseFee)
	if err != nil {
		return 0, 0, 0, err
	}
	pending, err := hexutil.DecodeUint64(status.Pending)
	if err != nil {
		return 0, 0, 0, err
	}
	queued, err := hexutil.DecodeUint64(status.Queued)
	if err != nil {
		return 0, 0, 0, err
	}
	return baseFee, pending, queued, nil
}
