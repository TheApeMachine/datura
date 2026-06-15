package server

import (
	"context"
	"encoding/binary"
	"net"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/rpc"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
)

/*
ForestServer implements the Cap'n Proto RPC interface for the spatial
index. It delegates all storage to a dmt.Forest, which provides persistence
(WAL), distribution (Merkle sync), and read routing (fastest tree).

Keys are Morton-packed uint64 values, stored as 8-byte big-endian keys
in the radix tree to preserve sort order for prefix queries.
*/
type ForestServer struct {
	ctx         context.Context
	cancel      context.CancelFunc
	serverSide  net.Conn
	clientSide  net.Conn
	client      Server
	serverConn  *rpc.Conn
	clientConns map[string]*rpc.Conn
	forest      *dmt.Forest
	workerPool  *qpool.Q[any]
}

type serverOpts func(*ForestServer)

/*
NewForestServer creates a new ForestServer backed by a dmt.Forest.
*/
func NewForestServer(opts ...serverOpts) *ForestServer {
	idx := &ForestServer{
		clientConns: map[string]*rpc.Conn{},
	}

	for _, opt := range opts {
		opt(idx)
	}

	if err := errnie.Require(map[string]any{
		"ctx": idx.ctx,
	}); err != nil {
		panic(err)
	}

	if idx.forest == nil {
		forest, err := dmt.NewForest(dmt.ForestConfig{
			Pool: idx.workerPool,
		})

		if err != nil {
			panic(err)
		}

		idx.forest = forest
	}

	idx.serverSide, idx.clientSide = net.Pipe()
	idx.client = Server_ServerToClient(idx)

	idx.serverConn = rpc.NewConn(rpc.NewStreamTransport(
		idx.serverSide,
	), &rpc.Options{
		BootstrapClient: capnp.Client(idx.client),
	})

	return idx
}

/*
Client returns a Cap'n Proto client connected to this ForestServer.
*/
func (idx *ForestServer) Client(clientID string) Server {
	idx.clientConns[clientID] = rpc.NewConn(rpc.NewStreamTransport(
		idx.clientSide,
	), &rpc.Options{
		BootstrapClient: capnp.Client(idx.client),
	})

	return idx.client
}

/*
Close shuts down the RPC connections, underlying net.Pipe, and the forest.
*/
func (idx *ForestServer) Close() error {
	var closeErr error

	if idx.serverConn != nil {
		closeErr = errnie.Combine(closeErr, idx.serverConn.Close())
		idx.serverConn = nil
	}

	for clientID, conn := range idx.clientConns {
		if conn != nil {
			closeErr = errnie.Combine(closeErr, conn.Close())
		}

		delete(idx.clientConns, clientID)
	}

	if idx.serverSide != nil {
		closeErr = errnie.Combine(closeErr, idx.serverSide.Close())
		idx.serverSide = nil
	}

	if idx.clientSide != nil {
		closeErr = errnie.Combine(closeErr, idx.clientSide.Close())
		idx.clientSide = nil
	}

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
	return nil
}

/*
Write stores a Morton-packed key in the forest. The key is encoded as
8-byte big-endian to preserve radix tree sort order.
*/
func (idx *ForestServer) Write(
	ctx context.Context, call Server_write,
) error {
	key := call.Args().Key()
	keyBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(keyBytes, key)

	idx.forest.Insert(keyBytes, nil)

	return nil
}

/*
Lookup retrieves values from the forest for the given Morton-packed keys.
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

	for index := range keys.Value().Len() {
		keyBytes := make([]byte, 8)
		binary.BigEndian.PutUint64(keyBytes, keys.Value().At(index))

		_, exists := idx.forest.Get(keyBytes)
		if exists {
			element := out.Value().At(index)
			_ = element
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
WithWorkerPool injects the shared worker pool for the backing forest.
*/
func WithWorkerPool(workerPool *qpool.Q[any]) serverOpts {
	return func(idx *ForestServer) {
		idx.workerPool = workerPool
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
