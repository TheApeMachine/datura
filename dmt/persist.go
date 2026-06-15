/*
package dmt implements persistence functionality for the radix tree.
The persistence layer uses a Write-Ahead Log (WAL) to ensure data durability
and provides mechanisms for recovery in case of failures.
*/
package dmt

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/theapemachine/qpool"
)

/*
Operation types for the WAL. These define the possible operations that can be
recorded in the write-ahead log for persistence and recovery.
*/
const (
	opInsert byte = iota
	opDelete
	opSnapshot
	opTermUpdate
)

/*
WALEntry represents a single write-ahead log entry. Each entry contains the
operation type and the associated key-value pair, allowing for replay during
recovery operations.
*/
type WALEntry struct {
	Op    byte
	Term  uint64
	Index uint64
	Key   []byte
	Value []byte
}

/*
PersistentStore handles the persistence layer for the radix tree.
It manages write-ahead logging and provides mechanisms for durable storage
and recovery of tree data.
*/
type PersistentStore struct {
	state      *batch
	walFile    *os.File
	walWriter  *bufio.Writer
	walPath    string
	snapPath   string
	ctx        context.Context
	cancel     context.CancelFunc
	pool       *qpool.Q[any]
	writeMutex sync.Mutex
	syncChan   chan struct{}
	lastIndex  uint64
	lastTerm   uint64
	closed     bool
	// Snapshot control
	snapCount uint64    // Number of entries before triggering snapshot
	lastSnap  time.Time // Time of last snapshot
	snapMutex sync.RWMutex
}

/*
NewPersistentStore creates a new persistent store instance.
It initializes the WAL file and sets up background syncing to ensure
data durability. The store will create necessary directories if they
don't exist.
*/
func NewPersistentStore(dir string) (*PersistentStore, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ps := &PersistentStore{
		state:     newBatch("dmt/persist"),
		walPath:   filepath.Join(dir, "wal.log"),
		snapPath:  filepath.Join(dir, "snapshot"),
		ctx:       ctx,
		cancel:    cancel,
		pool:      qpool.NewQ[any](ctx, 2, max(2, runtime.NumCPU()), workerPoolConfig()),
		syncChan:  make(chan struct{}, 100),
		snapCount: 1000,
	}

	guardStep(ps.state, func() error {
		return os.MkdirAll(dir, 0755)
	})

	ps.walFile = guardValue(ps.state, func() (*os.File, error) {
		return os.OpenFile(ps.walPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	})

	ps.walWriter = bufio.NewWriter(ps.walFile)

	go ps.backgroundSync()

	// Load last term/index from WAL
	guardStep(ps.state, ps.loadLastState)

	return ps, ps.state.Err()
}

/*
LogInsert asynchronously logs an insert operation to the WAL.
It writes the operation type, key, and value to the WAL buffer and
signals the background sync goroutine to flush to disk.
*/
func (ps *PersistentStore) LogInsert(key, value []byte, term, index uint64) error {
	ps.writeMutex.Lock()
	defer ps.writeMutex.Unlock()

	if ps.closed {
		return fmt.Errorf("persistent store is closed")
	}

	guardStep(ps.state, func() error {
		if err := ps.walWriter.WriteByte(opInsert); err != nil {
			return err
		}

		if err := binary.Write(ps.walWriter, binary.LittleEndian, term); err != nil {
			return err
		}

		if err := binary.Write(ps.walWriter, binary.LittleEndian, index); err != nil {
			return err
		}

		if err := binary.Write(ps.walWriter, binary.LittleEndian, uint32(len(key))); err != nil {
			return err
		}

		if _, err := ps.walWriter.Write(key); err != nil {
			return err
		}

		if err := binary.Write(ps.walWriter, binary.LittleEndian, uint32(len(value))); err != nil {
			return err
		}

		if _, err := ps.walWriter.Write(value); err != nil {
			return err
		}

		return nil
	})

	guardStep(ps.state, ps.walWriter.Flush)

	// Update last term/index
	ps.lastTerm = term
	ps.lastIndex = index

	// Signal for background sync
	select {
	case ps.syncChan <- struct{}{}:
	default:
	}

	// Check if snapshot needed
	if index%ps.snapCount == 0 {
		ps.schedule("snapshot", func(ctx context.Context) (any, error) {
			return nil, ps.createSnapshot()
		})
	}

	return ps.state.Err()
}

/*
backgroundSync periodically flushes the WAL to disk.
It listens on the sync channel and ensures that buffered writes are
persisted to stable storage.
*/
func (ps *PersistentStore) backgroundSync() {
	for range ps.syncChan {
		if ps.ctx.Err() != nil {
			return
		}

		ps.writeMutex.Lock()
		ps.walWriter.Flush()
		ps.walFile.Sync()
		ps.writeMutex.Unlock()
	}
}

/*
Close closes the persistent store, ensuring all buffered data is
written to disk and resources are properly released.
*/
func (ps *PersistentStore) Close() error {
	ps.writeMutex.Lock()

	if ps.closed {
		ps.writeMutex.Unlock()

		return nil
	}

	ps.closed = true

	if ps.cancel != nil {
		ps.cancel()
	}

	close(ps.syncChan)

	guardStep(ps.state, ps.walWriter.Flush)
	guardStep(ps.state, ps.walFile.Close)

	workerPool := ps.pool
	ps.pool = nil

	ps.writeMutex.Unlock()

	if workerPool != nil {
		workerPool.Close()
	}

	return ps.state.Err()
}

/*
LogTerm writes a term-update entry to the WAL so it survives restart.
*/
func (ps *PersistentStore) LogTerm(term uint64) error {
	ps.writeMutex.Lock()
	defer ps.writeMutex.Unlock()

	if ps.closed {
		return fmt.Errorf("persistent store is closed")
	}

	guardStep(ps.state, func() error {
		if err := ps.walWriter.WriteByte(opTermUpdate); err != nil {
			return err
		}

		return binary.Write(ps.walWriter, binary.LittleEndian, term)
	})

	guardStep(ps.state, ps.walWriter.Flush)

	ps.lastTerm = term

	select {
	case ps.syncChan <- struct{}{}:
	default:
	}

	return ps.state.Err()
}

/*
Replay reads all entries from the WAL and returns them for reinsertion
into the tree. Also restores lastTerm and lastIndex.
*/
func (ps *PersistentStore) Replay() ([]WALEntry, error) {
	file := guardValue(ps.state, func() (*os.File, error) {
		f, err := os.Open(ps.walPath)
		if os.IsNotExist(err) {
			return nil, nil
		}
		return f, err
	})

	if ps.state.Failed() || file == nil {
		return nil, ps.state.Err()
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	var entries []WALEntry

	for {
		op, err := reader.ReadByte()
		if err != nil {
			break
		}

		switch op {
		case opInsert:
			var term, index uint64
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &term)
			})

			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &index)
			})

			var keyLen uint32
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &keyLen)
			})

			key := make([]byte, keyLen)
			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, key)
				return err
			})

			var valLen uint32
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &valLen)
			})

			value := make([]byte, valLen)
			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, value)
				return err
			})

			entries = append(entries, WALEntry{
				Op:    opInsert,
				Term:  term,
				Index: index,
				Key:   key,
				Value: value,
			})

			ps.lastTerm = term
			ps.lastIndex = index

		case opTermUpdate:
			var term uint64
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &term)
			})

			ps.lastTerm = term

		case opSnapshot:
			var term, index uint64
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &term)
			})

			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &index)
			})

			ps.lastTerm = term
			ps.lastIndex = index

			if ps.state.Failed() {
				return entries, ps.state.Err()
			}
		default:
			return entries, fmt.Errorf("invalid wal operation: %d", op)
		}
	}

	return entries, nil
}

/*
loadLastState reads the WAL to restore term and index.
*/
func (ps *PersistentStore) loadLastState() error {
	_, err := ps.Replay()
	return err
}

// createSnapshot creates a new snapshot and truncates the WAL
func (ps *PersistentStore) createSnapshot() error {
	ps.snapMutex.Lock()
	defer ps.snapMutex.Unlock()

	// Ensure minimum time between snapshots
	if time.Since(ps.lastSnap) < time.Minute {
		return nil
	}

	ps.state.Reset()

	// Create snapshot directory if not exists
	guardStep(ps.state, func() error {
		return os.MkdirAll(ps.snapPath, 0755)
	})

	// Create new snapshot file
	snapFile := filepath.Join(ps.snapPath, fmt.Sprintf("snapshot-%d-%d", ps.lastTerm, ps.lastIndex))

	file := guardValue(ps.state, func() (*os.File, error) {
		return os.Create(snapFile)
	})

	if ps.state.Failed() {
		return ps.state.Err()
	}
	defer file.Close()

	// Write snapshot metadata
	guardStep(ps.state, func() error {
		return binary.Write(file, binary.LittleEndian, ps.lastTerm)
	})

	guardStep(ps.state, func() error {
		return binary.Write(file, binary.LittleEndian, ps.lastIndex)
	})

	// Log snapshot creation in WAL
	ps.writeMutex.Lock()
	defer ps.writeMutex.Unlock()

	guardStep(ps.state, func() error {
		return ps.walWriter.WriteByte(opSnapshot)
	})

	guardStep(ps.state, func() error {
		return binary.Write(ps.walWriter, binary.LittleEndian, ps.lastTerm)
	})

	guardStep(ps.state, func() error {
		return binary.Write(ps.walWriter, binary.LittleEndian, ps.lastIndex)
	})

	// Truncate WAL
	guardStep(ps.state, ps.truncateWAL)

	if !ps.state.Failed() {
		ps.lastSnap = time.Now()
	}

	return ps.state.Err()
}

// truncateWAL creates a new WAL file with just the snapshot entry
func (ps *PersistentStore) truncateWAL() error {
	// Create new WAL file
	newPath := ps.walPath + ".new"

	newFile := guardValue(ps.state, func() (*os.File, error) {
		return os.Create(newPath)
	})

	if ps.state.Failed() {
		return ps.state.Err()
	}

	writer := bufio.NewWriter(newFile)

	// Write snapshot entry
	guardStep(ps.state, func() error {
		return writer.WriteByte(opSnapshot)
	})

	guardStep(ps.state, func() error {
		return binary.Write(writer, binary.LittleEndian, ps.lastTerm)
	})

	guardStep(ps.state, func() error {
		return binary.Write(writer, binary.LittleEndian, ps.lastIndex)
	})

	// Ensure all data is written
	guardStep(ps.state, writer.Flush)
	guardStep(ps.state, newFile.Sync)
	guardStep(ps.state, newFile.Close)

	// Replace old WAL with new one
	guardStep(ps.state, func() error {
		return os.Rename(newPath, ps.walPath)
	})

	// Update file handles
	guardStep(ps.state, ps.walFile.Close)

	ps.walFile = guardValue(ps.state, func() (*os.File, error) {
		return os.OpenFile(ps.walPath, os.O_APPEND|os.O_RDWR, 0644)
	})

	if !ps.state.Failed() {
		ps.walWriter = bufio.NewWriter(ps.walFile)
	}

	return ps.state.Err()
}

// GetLastState returns the last recorded term and index
func (ps *PersistentStore) GetLastState() (term, index uint64) {
	ps.writeMutex.Lock()
	defer ps.writeMutex.Unlock()
	return ps.lastTerm, ps.lastIndex
}

func (ps *PersistentStore) schedule(
	id string,
	fn func(ctx context.Context) (any, error),
) {
	ps.pool.Schedule(
		"dmt/persist/"+id,
		fn,
		qpool.WithTTL(time.Second),
	)
}
