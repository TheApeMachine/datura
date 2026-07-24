package server

import (
	"context"
	"encoding/binary"
	"errors"
	"sync"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
)

/*
ForestServer implements the Cap'n Proto RPC interface for the spatial
index. It delegates all storage to a dmt.Forest, which provides persistence
(WAL), distribution (Merkle sync), and read routing (applied-index + latency).

Keys are Morton-packed uint64 values, stored as 8-byte big-endian keys
in the radix tree to preserve sort order for prefix queries.

In-process clients receive the local Cap'n Proto capability directly; there is
no net.Pipe multiplexing.
*/
type ForestServer struct {
	ctx    context.Context
	cancel context.CancelFunc
	client Server
	mu     sync.Mutex
	forest *dmt.Forest
}

type serverOpts func(*ForestServer)

/*
NewForestServer creates a new ForestServer backed by a dmt.Forest.
*/
func NewForestServer(opts ...serverOpts) (*ForestServer, error) {
	idx := &ForestServer{}

	for _, opt := range opts {
		opt(idx)
	}

	if idx.ctx == nil {
		return nil, errors.New("dmt/server: context is required")
	}

	if idx.forest == nil {
		forest, err := dmt.NewForest(dmt.ForestConfig{})

		if err != nil {
			return nil, err
		}

		idx.forest = forest
	}

	idx.client = Server_ServerToClient(idx)

	return idx, nil
}

/*
Client returns the local Cap'n Proto capability for in-process use.
*/
func (idx *ForestServer) Client(clientID string) Server {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	_ = clientID

	return idx.client
}

/*
Close shuts down the forest and cancels the server context.
*/
func (idx *ForestServer) Close() error {
	var closeErr error

	if idx.cancel != nil {
		idx.cancel()
	}

	if idx.forest != nil {
		closeErr = errnie.Combine(closeErr, idx.forest.Close())
	}

	return closeErr
}

/*
Done implements the Forest RPC done method.
*/
func (idx *ForestServer) Done(ctx context.Context, call Server_done) error {
	return idx.Close()
}

/*
EvaluateClassification routes a context probe through the classification engine.
*/
func (idx *ForestServer) EvaluateClassification(sequence []byte) (dmt.ClassificationResult, error) {
	tree := idx.forest.GetFastestTree()

	if tree == nil {
		return dmt.ClassificationResult{}, errors.New("no active trees available")
	}

	var scratch dmt.ClassificationScratch

	return tree.Classify(sequence, &scratch)
}

/*
IntelligentIngestPipeline stores data and evaluates branch curiosity for peer sync.
*/
func (idx *ForestServer) IntelligentIngestPipeline(key, value []byte) error {
	if err := idx.forest.Insert(key, value); err != nil {
		return err
	}

	idx.forest.EvaluateCuriosityAndTriggerSync(key)

	return nil
}

/*
ExactLookup resolves a key without analogical fallback.
*/
func (idx *ForestServer) ExactLookup(key []byte) ([]byte, bool) {
	return idx.forest.Get(key)
}

/*
AnalogLookup resolves a key through structural analog mapping and returns the
matched key when fallback occurs.
*/
func (idx *ForestServer) AnalogLookup(key []byte) (matchedKey, value []byte, found bool) {
	if value, ok := idx.forest.Get(key); ok {
		return append([]byte(nil), key...), value, true
	}

	tree := idx.forest.GetFastestTree()

	if tree == nil {
		return nil, nil, false
	}

	match, ok := tree.FindStructuralAnalog(key)

	if !ok {
		return nil, nil, false
	}

	value, found = tree.Get(match.ClosestKey)

	if !found {
		return nil, nil, false
	}

	return append([]byte(nil), match.ClosestKey...), value, true
}

/*
Write stores a Morton-packed key in the forest. The key is encoded as
8-byte big-endian to preserve radix tree sort order.
*/
func (idx *ForestServer) Write(
	ctx context.Context, call Server_write,
) error {
	args := call.Args()
	key := args.Key()

	if !args.HasValue() {
		return errors.New("dmt/server write requires value")
	}

	value, err := args.Value()
	if err != nil {
		return errnie.Error(err, "rpc_input_value_failed")
	}
	if len(value) == 0 {
		return errors.New("dmt/server write requires non-empty value")
	}

	var keyBytes [8]byte
	binary.BigEndian.PutUint64(keyBytes[:], key)

	return idx.IntelligentIngestPipeline(
		append([]byte(nil), keyBytes[:]...),
		append([]byte(nil), value...),
	)
}

/*
Lookup retrieves exact values from the forest for the given Morton-packed keys.
Missing keys leave a found=false style empty artifact rather than analog fallback.
*/
func (idx *ForestServer) Lookup(
	ctx context.Context,
	call Server_lookup,
) error {
	args := call.Args()

	keys := errnie.Does(func() (capnp.UInt64List, error) {
		return args.Keys()
	})

	if keys.Err() != nil {
		return keys.Err()
	}

	results := errnie.Does(func() (Server_lookup_Results, error) {
		return call.AllocResults()
	})

	if results.Err() != nil {
		return results.Err()
	}

	out := errnie.Does(func() (datura.Artifact_List, error) {
		return results.Value().NewValues(int32(keys.Value().Len()))
	})

	if out.Err() != nil {
		return out.Err()
	}

	var keyBytes [8]byte

	for index := range keys.Value().Len() {
		binary.BigEndian.PutUint64(keyBytes[:], keys.Value().At(index))

		value, exists := idx.ExactLookup(keyBytes[:])

		if !exists || len(value) == 0 {
			continue
		}

		element := &datura.Artifact{}

		if _, err := element.Unpack(value); err != nil {
			return errnie.Error(err, "rpc_output_population_failed")
		}

		if err := out.Value().Set(index, *element); err != nil {
			return errnie.Error(err, "rpc_output_population_failed")
		}
	}

	return nil
}

/*
Forest returns the underlying dmt.Forest for direct access by
components that need the raw store (e.g. sequence storage).
*/
func (idx *ForestServer) Forest() *dmt.Forest {
	return idx.forest
}

/*
WithContext sets the context for the server.
*/
func WithContext(ctx context.Context) serverOpts {
	return func(idx *ForestServer) {
		idx.ctx, idx.cancel = context.WithCancel(ctx)
	}
}

/*
WithForest injects a pre-created dmt.Forest.
*/
func WithForest(forest *dmt.Forest) serverOpts {
	return func(idx *ForestServer) {
		idx.forest = forest
	}
}

/*
SpatialIndexError is a typed error for SpatialIndex failures.
*/
type SpatialIndexError string

const (
	ErrForestInit SpatialIndexError = "spatial-index: forest init failed"
)

/*
Error implements the error interface for SpatialIndexError.
*/
func (err SpatialIndexError) Error() string {
	return string(err)
}
