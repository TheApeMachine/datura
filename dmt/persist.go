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
	"strconv"
	"sync/atomic"
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
	state     *batch
	walFile   *os.File
	walWriter *bufio.Writer
	walPath   string
	snapPath  string
	ctx       context.Context
	cancel    context.CancelFunc
	pool      *qpool.Q[any]
	walSeq    atomic.Uint64
	lastIndex atomic.Uint64
	lastTerm  atomic.Uint64
	closed    atomic.Bool
	snapCount uint64
	lastSnap  atomic.Int64
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
		pool:      qpool.NewQ[any](ctx, 1, 1, workerPoolConfig()),
		snapCount: 1000,
	}

	guardStep(ps.state, func() error {
		return os.MkdirAll(dir, 0755)
	})

	ps.walFile = guardValue(ps.state, func() (*os.File, error) {
		return os.OpenFile(ps.walPath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0644)
	})

	ps.walWriter = bufio.NewWriter(ps.walFile)

	guardStep(ps.state, ps.loadLastState)

	return ps, ps.state.Err()
}

/*
LogInsert logs an insert operation to the WAL through the serialized worker pool.
*/
func (ps *PersistentStore) LogInsert(key, value []byte, term, index uint64) error {
	if ps.closed.Load() {
		return fmt.Errorf("persistent store is closed")
	}

	err := ps.runWal("insert", func() error {
		ps.state.Reset()

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
		guardStep(ps.state, ps.walFile.Sync)

		return ps.state.Err()
	})

	if err != nil {
		return err
	}

	ps.lastTerm.Store(term)
	ps.lastIndex.Store(index)

	if index%ps.snapCount == 0 {
		ps.schedule("snapshot", func(ctx context.Context) (any, error) {
			return nil, ps.createSnapshot()
		})
	}

	return nil
}

/*
Close closes the persistent store, ensuring all buffered data is
written to disk and resources are properly released.
*/
func (ps *PersistentStore) Close() error {
	if !ps.closed.CompareAndSwap(false, true) {
		return nil
	}

	if ps.cancel != nil {
		ps.cancel()
	}

	err := ps.runWal("close", func() error {
		ps.state.Reset()

		guardStep(ps.state, ps.walWriter.Flush)
		guardStep(ps.state, ps.walFile.Close)

		ps.walFile = nil
		ps.walWriter = nil

		return ps.state.Err()
	})

	workerPool := ps.pool
	ps.pool = nil

	if workerPool != nil {
		workerPool.Close()
	}

	return err
}

/*
LogTerm writes a term-update entry to the WAL so it survives restart.
*/
func (ps *PersistentStore) LogTerm(term uint64) error {
	if ps.closed.Load() {
		return fmt.Errorf("persistent store is closed")
	}

	err := ps.runWal("term", func() error {
		ps.state.Reset()

		guardStep(ps.state, func() error {
			if err := ps.walWriter.WriteByte(opTermUpdate); err != nil {
				return err
			}

			return binary.Write(ps.walWriter, binary.LittleEndian, term)
		})

		guardStep(ps.state, ps.walWriter.Flush)
		guardStep(ps.state, ps.walFile.Sync)

		return ps.state.Err()
	})

	if err != nil {
		return err
	}

	ps.lastTerm.Store(term)

	return nil
}

/*
Replay reads all entries from the WAL and returns them for reinsertion
into the tree. Also restores lastTerm and lastIndex.
*/
func (ps *PersistentStore) Replay() ([]WALEntry, error) {
	ps.state.Reset()

	file := guardValue(ps.state, func() (*os.File, error) {
		fileHandle, err := os.Open(ps.walPath)
		if os.IsNotExist(err) {
			return nil, nil
		}

		return fileHandle, err
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

			ps.lastTerm.Store(term)
			ps.lastIndex.Store(index)

		case opTermUpdate:
			var term uint64
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &term)
			})

			ps.lastTerm.Store(term)

		case opSnapshot:
			var term, index uint64
			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &term)
			})

			guardStep(ps.state, func() error {
				return binary.Read(reader, binary.LittleEndian, &index)
			})

			ps.lastTerm.Store(term)
			ps.lastIndex.Store(index)

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

func (ps *PersistentStore) createSnapshot() error {
	now := time.Now().UnixNano()
	lastSnap := ps.lastSnap.Load()

	if now-lastSnap < int64(time.Minute) {
		return nil
	}

	if !ps.lastSnap.CompareAndSwap(lastSnap, now) {
		return nil
	}

	return ps.persistWal(func() error {
		ps.state.Reset()

		guardStep(ps.state, func() error {
			return os.MkdirAll(ps.snapPath, 0755)
		})

		snapFile := filepath.Join(
			ps.snapPath,
			fmt.Sprintf("snapshot-%d-%d", ps.lastTerm.Load(), ps.lastIndex.Load()),
		)

		file := guardValue(ps.state, func() (*os.File, error) {
			return os.Create(snapFile)
		})

		if ps.state.Failed() {
			return ps.state.Err()
		}
		defer file.Close()

		guardStep(ps.state, func() error {
			return binary.Write(file, binary.LittleEndian, ps.lastTerm.Load())
		})

		guardStep(ps.state, func() error {
			return binary.Write(file, binary.LittleEndian, ps.lastIndex.Load())
		})

		guardStep(ps.state, func() error {
			return ps.walWriter.WriteByte(opSnapshot)
		})

		guardStep(ps.state, func() error {
			return binary.Write(ps.walWriter, binary.LittleEndian, ps.lastTerm.Load())
		})

		guardStep(ps.state, func() error {
			return binary.Write(ps.walWriter, binary.LittleEndian, ps.lastIndex.Load())
		})

		guardStep(ps.state, ps.truncateWAL)

		return ps.state.Err()
	})
}

func (ps *PersistentStore) truncateWAL() error {
	newPath := ps.walPath + ".new"

	newFile := guardValue(ps.state, func() (*os.File, error) {
		return os.Create(newPath)
	})

	if ps.state.Failed() {
		return ps.state.Err()
	}

	writer := bufio.NewWriter(newFile)

	guardStep(ps.state, func() error {
		return writer.WriteByte(opSnapshot)
	})

	guardStep(ps.state, func() error {
		return binary.Write(writer, binary.LittleEndian, ps.lastTerm.Load())
	})

	guardStep(ps.state, func() error {
		return binary.Write(writer, binary.LittleEndian, ps.lastIndex.Load())
	})

	guardStep(ps.state, writer.Flush)
	guardStep(ps.state, newFile.Sync)
	guardStep(ps.state, newFile.Close)

	guardStep(ps.state, func() error {
		return os.Rename(newPath, ps.walPath)
	})

	guardStep(ps.state, ps.walFile.Close)

	ps.walFile = guardValue(ps.state, func() (*os.File, error) {
		return os.OpenFile(ps.walPath, os.O_APPEND|os.O_RDWR, 0644)
	})

	if !ps.state.Failed() {
		ps.walWriter = bufio.NewWriter(ps.walFile)
	}

	return ps.state.Err()
}

func (ps *PersistentStore) GetLastState() (term, index uint64) {
	return ps.lastTerm.Load(), ps.lastIndex.Load()
}

func (ps *PersistentStore) TruncateWAL() error {
	return ps.runWal("truncate", ps.truncateWAL)
}

func (ps *PersistentStore) runWal(op string, fn func() error) error {
	if ps.pool == nil {
		return ps.persistWal(fn)
	}

	sequence := ps.walSeq.Add(1)
	jobID := "dmt/persist/" + op + "/" + strconv.FormatUint(sequence, 10)

	wait := ps.pool.Schedule(jobID, func(ctx context.Context) (any, error) {
		return nil, ps.persistWal(fn)
	})

	_, err := wait.Get(ps.ctx)

	return err
}

func (ps *PersistentStore) persistWal(fn func() error) error {
	return fn()
}

func (ps *PersistentStore) schedule(
	id string,
	fn func(ctx context.Context) (any, error),
) {
	if ps.pool == nil {
		return
	}

	ps.pool.Schedule(
		"dmt/persist/"+id,
		fn,
		qpool.WithTTL(time.Second),
	)
}
