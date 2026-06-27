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

	if ps.state.Err() == nil {
		go ps.backgroundSyncer()
	}

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
			var frame [25]byte

			frame[0] = opInsert
			binary.LittleEndian.PutUint64(frame[1:9], term)
			binary.LittleEndian.PutUint64(frame[9:17], index)
			binary.LittleEndian.PutUint32(frame[17:21], uint32(len(key)))
			binary.LittleEndian.PutUint32(frame[21:25], uint32(len(value)))

			if _, err := ps.walWriter.Write(frame[:]); err != nil {
				return err
			}

			if _, err := ps.walWriter.Write(key); err != nil {
				return err
			}

			if _, err := ps.walWriter.Write(value); err != nil {
				return err
			}

			return nil
		})

		guardStep(ps.state, ps.walWriter.Flush)

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
LogDelete logs a delete operation to the WAL through the serialized worker pool.
*/
func (ps *PersistentStore) LogDelete(key []byte, term, index uint64) error {
	if ps.closed.Load() {
		return fmt.Errorf("persistent store is closed")
	}

	err := ps.runWal("delete", func() error {
		ps.state.Reset()

		guardStep(ps.state, func() error {
			var frame [21]byte

			frame[0] = opDelete
			binary.LittleEndian.PutUint64(frame[1:9], term)
			binary.LittleEndian.PutUint64(frame[9:17], index)
			binary.LittleEndian.PutUint32(frame[17:21], uint32(len(key)))

			if _, err := ps.walWriter.Write(frame[:]); err != nil {
				return err
			}

			_, err := ps.walWriter.Write(key)

			return err
		})

		guardStep(ps.state, ps.walWriter.Flush)

		return ps.state.Err()
	})

	if err != nil {
		return err
	}

	ps.lastTerm.Store(term)
	ps.lastIndex.Store(index)

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
		if ps.walFile != nil {
			_ = ps.walFile.Sync()
		}
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
			var frame [9]byte

			frame[0] = opTermUpdate
			binary.LittleEndian.PutUint64(frame[1:9], term)

			_, err := ps.walWriter.Write(frame[:])

			return err
		})

		guardStep(ps.state, ps.walWriter.Flush)

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
			var header [24]byte

			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, header[:])

				return err
			})

			if ps.state.Failed() {
				break
			}

			term := binary.LittleEndian.Uint64(header[0:8])
			index := binary.LittleEndian.Uint64(header[8:16])
			keyLen := binary.LittleEndian.Uint32(header[16:20])
			valLen := binary.LittleEndian.Uint32(header[20:24])

			key := make([]byte, keyLen)
			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, key)

				return err
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

		case opDelete:
			var header [20]byte

			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, header[:])

				return err
			})

			if ps.state.Failed() {
				break
			}

			term := binary.LittleEndian.Uint64(header[0:8])
			index := binary.LittleEndian.Uint64(header[8:16])
			keyLen := binary.LittleEndian.Uint32(header[16:20])

			key := make([]byte, keyLen)
			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, key)

				return err
			})

			entries = append(entries, WALEntry{
				Op:    opDelete,
				Term:  term,
				Index: index,
				Key:   key,
			})

			ps.lastTerm.Store(term)
			ps.lastIndex.Store(index)

		case opTermUpdate:
			var termBuf [8]byte

			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, termBuf[:])

				return err
			})

			if ps.state.Failed() {
				break
			}

			ps.lastTerm.Store(binary.LittleEndian.Uint64(termBuf[:]))

		case opSnapshot:
			var snapshotHeader [16]byte

			guardStep(ps.state, func() error {
				_, err := io.ReadFull(reader, snapshotHeader[:])

				return err
			})

			if ps.state.Failed() {
				break
			}

			term := binary.LittleEndian.Uint64(snapshotHeader[0:8])
			index := binary.LittleEndian.Uint64(snapshotHeader[8:16])

			ps.lastTerm.Store(term)
			ps.lastIndex.Store(index)
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
			var stateFrame [16]byte

			binary.LittleEndian.PutUint64(stateFrame[0:8], ps.lastTerm.Load())
			binary.LittleEndian.PutUint64(stateFrame[8:16], ps.lastIndex.Load())

			_, err := file.Write(stateFrame[:])

			return err
		})

		guardStep(ps.state, func() error {
			var walFrame [17]byte

			walFrame[0] = opSnapshot
			binary.LittleEndian.PutUint64(walFrame[1:9], ps.lastTerm.Load())
			binary.LittleEndian.PutUint64(walFrame[9:17], ps.lastIndex.Load())

			if _, err := ps.walWriter.Write(walFrame[:]); err != nil {
				return err
			}

			return nil
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
		var walFrame [17]byte

		walFrame[0] = opSnapshot
		binary.LittleEndian.PutUint64(walFrame[1:9], ps.lastTerm.Load())
		binary.LittleEndian.PutUint64(walFrame[9:17], ps.lastIndex.Load())

		_, err := writer.Write(walFrame[:])

		return err
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

func (ps *PersistentStore) backgroundSyncer() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ps.ctx.Done():
			return
		case <-ticker.C:
			if ps.closed.Load() {
				return
			}
			_ = ps.runWal("sync", func() error {
				if ps.walFile != nil {
					return ps.walFile.Sync()
				}
				return nil
			})
		}
	}
}
