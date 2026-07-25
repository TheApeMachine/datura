package dmt

import (
	"context"
	"runtime"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
batch accumulates the first error across a sequence of guarded steps.
*/
type batch struct {
	op  string
	err error
}

func newBatch(op string) *batch {
	return &batch{op: op}
}

func (batch *batch) Reset() {
	batch.err = nil
}

func (batch *batch) Failed() bool {
	return batch.err != nil
}

func (batch *batch) Err() error {
	return batch.err
}

func guardValue[T any](batch *batch, fn func() (T, error)) T {
	if batch.err != nil {
		var zero T

		return zero
	}

	value, err := fn()

	if err != nil {
		batch.err = errnie.Err(errnie.IO, batch.op, err)
	}

	return value
}

func guardStep(batch *batch, fn func() error) {
	if batch.err != nil {
		return
	}

	if err := fn(); err != nil {
		batch.err = errnie.Err(errnie.IO, batch.op, err)
	}
}

func workerPoolConfig() *qpool.Config {
	config := qpool.NewConfig()
	config.Scaler = nil

	return config
}

func newWorkerPool(ctx context.Context) *qpool.Q[any] {
	workers := max(4, runtime.NumCPU())

	return qpool.NewQ[any](ctx, workers, workers, workerPoolConfig())
}
