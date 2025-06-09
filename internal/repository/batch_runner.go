package repository

import (
	"log/slog"

	"github.com/pkg/errors"
)

// Batch operation support

type BatchOperation struct {
	BatchSize int
	Logger    *slog.Logger
}

// ProcessInBatches processes items in batches within a transaction
func (bo *BatchOperation) ProcessInBatches(tx Transaction, items []interface{}, processFn func(tx Transaction, batch []interface{}) error) error {
	if len(items) == 0 {
		return nil
	}

	batchSize := bo.BatchSize
	if batchSize <= 0 {
		batchSize = 100 // default batch size
	}

	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]

		bo.Logger.Debug("processing batch",
			"batch_start", i,
			"batch_end", end,
			"batch_size", len(batch),
			"total_items", len(items))

		if err := processFn(tx, batch); err != nil {
			return errors.Wrapf(err, "failed to process batch %d-%d", i, end)
		}
	}

	return nil
}
