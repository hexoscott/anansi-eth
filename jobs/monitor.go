package jobs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/hashicorp/go-hclog"
)

// Default stabilization detection constants
const (
	defaultMinSamplesForStabilization = 20  // Minimum samples needed before checking stability
	defaultStabilityThresholdPercent  = 5.0 // Standard deviation should be within 5% of mean for stability
	defaultStabilityRequiredDuration  = 10  // Number of consecutive stable measurements required
)

type Monitor struct {
	Job
	done                   chan struct{}
	jobs                   []Job
	lastBlock              uint64
	transactionCounts      []int
	isStabilized           bool
	consecutiveStableCount int
	lastStdDev             float64
	lastMean               float64

	// Configurable stabilization parameters
	minSamplesForStabilization int
	stabilityThresholdPercent  float64
	stabilityRequiredDuration  int
}

func NewMonitor(jobs []Job) (*Monitor, error) {
	return NewMonitorWithConfig(jobs, nil)
}

func NewMonitorWithConfig(jobs []Job, config *StabilizationConfig) (*Monitor, error) {
	monitor := &Monitor{
		done:              make(chan struct{}),
		jobs:              jobs,
		transactionCounts: make([]int, 0),
		// Set defaults
		minSamplesForStabilization: defaultMinSamplesForStabilization,
		stabilityThresholdPercent:  defaultStabilityThresholdPercent,
		stabilityRequiredDuration:  defaultStabilityRequiredDuration,
	}

	// Override with config if provided
	if config != nil {
		if config.MinSamples > 0 {
			monitor.minSamplesForStabilization = config.MinSamples
		}
		if config.StabilityThresholdPercent > 0 {
			monitor.stabilityThresholdPercent = config.StabilityThresholdPercent
		}
		if config.StabilityRequiredDuration > 0 {
			monitor.stabilityRequiredDuration = config.StabilityRequiredDuration
		}
	}

	return monitor, nil
}

func (j *Monitor) Name() string {
	return "monitor"
}

func (j *Monitor) Run(ctx context.Context, client *ethclient.Client, log hclog.Logger) error {
	defer close(j.done)
	log = log.With("job", j.Name())
	// lastPending := uint64(0)

	// get the latest block so we can start to capture some load related data as the loop runs
	latestBlock, err := client.BlockByNumber(ctx, nil)
	if err != nil {
		return err
	}
	j.lastBlock = latestBlock.NumberU64()

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var result json.RawMessage
		err := client.Client().Call(&result, "txpool_status")
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
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

		var totalTransactions uint64 = 0
		stats := make(map[string]map[string]uint64)
		for _, job := range j.jobs {
			name := job.Name()
			s := job.GiveLoadStats()
			found, ok := stats[name]
			if !ok {
				found = make(map[string]uint64)
				stats[name] = found
			}
			for k, v := range s {
				found[k] = found[k] + v
				totalTransactions += v
			}
		}

		log.Info("txpool_status", "baseFee", baseFee, "pending", pending, "queued", queued)
		log.Info("current load per run", "total", totalTransactions)

		latestBlock, err := client.BlockByNumber(ctx, nil)
		if err != nil {
			log.Error("failed to get latest block", "error", err)
			time.Sleep(3 * time.Second)
			continue
		}
		latestNumber := latestBlock.NumberU64()
		latestTxCount := len(latestBlock.Transactions())

		log.Info("load stats report", "report", stats)

		if latestNumber > j.lastBlock {
			j.transactionCounts = append(j.transactionCounts, latestTxCount)
			if len(j.transactionCounts) > j.minSamplesForStabilization {
				j.transactionCounts = j.transactionCounts[1:] // remove the oldest value
			}

			// Check for stabilization
			j.checkStabilization()
			mean, stdDev, consecutiveStable, coefficientOfVariation := j.GetStabilizationMetrics()

			if j.isStabilized && consecutiveStable > j.stabilityRequiredDuration {
				log.Info("🎯 TRANSACTION RATE STABILIZED!",
					"mean", mean,
					"stdDev", stdDev,
					"coefficientOfVariation%", coefficientOfVariation,
					"consecutiveStableMeasurements", consecutiveStable)

				log.Info("🔥 increasing load!")
				for _, job := range j.jobs {
					job.UpdateLoad(LoadIncrease)
				}
			}

			// Enhanced logging with stabilization metrics
			if len(j.transactionCounts) >= j.minSamplesForStabilization {
				log.Info("transaction analysis",
					"blockCount", len(j.transactionCounts),
					"mean", mean,
					"stdDev", stdDev,
					"coefficientOfVariation%", coefficientOfVariation,
					"isStabilized", j.isStabilized,
					"consecutiveStable", consecutiveStable)
			} else {
				log.Info("transaction analysis (collecting samples)",
					"blockCount", len(j.transactionCounts),
					"mean", mean,
					"samplesNeeded", j.minSamplesForStabilization-len(j.transactionCounts))
			}
		}

		j.lastBlock = latestNumber

		// lastPending = pending

		time.Sleep(1 * time.Second)
	}
}

func (j *Monitor) WaitForStop() <-chan struct{} {
	return j.done
}

func (j *Monitor) SetWallet(address *common.Address, privateKey *ecdsa.PrivateKey, chainID *big.Int, gasPrice *big.Int) {
	// no op
}

func (j *Monitor) NeedsFunding() bool {
	return false
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

func (j *Monitor) Instance() uint64 {
	return 0
}

func (j *Monitor) WalletAddress() *common.Address {
	address := common.HexToAddress("0x0000000000000000000000000000000000000000")
	return &address
}

// calculateStandardDeviation calculates the standard deviation of transaction counts
func (j *Monitor) calculateStandardDeviation() (mean, stdDev float64) {
	if len(j.transactionCounts) == 0 {
		return 0, 0
	}

	// Calculate mean
	sum := 0
	for _, count := range j.transactionCounts {
		sum += count
	}
	mean = float64(sum) / float64(len(j.transactionCounts))

	// Calculate variance
	variance := 0.0
	for _, count := range j.transactionCounts {
		diff := float64(count) - mean
		variance += diff * diff
	}
	variance /= float64(len(j.transactionCounts))

	// Standard deviation is the square root of variance
	stdDev = math.Sqrt(variance)

	return mean, stdDev
}

// checkStabilization determines if the transaction count has stabilized
func (j *Monitor) checkStabilization() bool {
	// Need minimum samples to determine stability
	if len(j.transactionCounts) < j.minSamplesForStabilization {
		return false
	}

	mean, stdDev := j.calculateStandardDeviation()
	j.lastMean = mean
	j.lastStdDev = stdDev

	// Calculate coefficient of variation (standard deviation as percentage of mean)
	if mean == 0 {
		return false
	}

	coefficientOfVariation := (stdDev / mean) * 100

	// Check if the coefficient of variation is within our stability threshold
	isCurrentlyStable := coefficientOfVariation <= j.stabilityThresholdPercent

	if isCurrentlyStable {
		j.consecutiveStableCount++
	} else {
		j.consecutiveStableCount = 0
	}

	// Consider stabilized if we've had enough consecutive stable measurements
	wasStabilized := j.isStabilized
	j.isStabilized = j.consecutiveStableCount >= j.stabilityRequiredDuration

	return j.isStabilized && !wasStabilized // Return true only on the transition to stabilized
}

// GetStabilizationMetrics returns current stabilization metrics for monitoring/debugging
func (j *Monitor) GetStabilizationMetrics() (mean, stdDev float64, consecutiveStable int, coefficientOfVariation float64) {
	mean, stdDev = j.calculateStandardDeviation()
	coefficientOfVariation = 0
	if mean > 0 {
		coefficientOfVariation = (stdDev / mean) * 100
	}
	return mean, stdDev, j.consecutiveStableCount, coefficientOfVariation
}
