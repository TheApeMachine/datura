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
	"errors"
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

const maxWALFieldBytes = 64 << 20

type persistenceFatal struct {
	message string
}

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
	fatal     atomic.Pointer[persistenceFatal]
	snapCount uint64
	lastSnap  atomic.Int64
}

func (ps *PersistentStore) fatalError() error {
	fatal := ps.fatal.Load()
	if fatal == nil {
		return nil
	}

	return errors.New(fatal.message)
}

func (ps *PersistentStore) markFatal(err error) error {
	if err == nil {
		return nil
	}

	if existing := ps.fatalError(); existing != nil {
		return existing
	}

	fatal := &persistenceFatal{
		message: fmt.Sprintf("persistent store fatal: %v", err),
	}

	if ps.fatal.CompareAndSwap(nil, fatal) {
		return errors.New(fatal.message)
	}

	return ps.fatalError()
}

func (ps *PersistentStore) requireWritable() error {
	if err := ps.fatalError(); err != nil {
		return err
	}

	if ps.closed.Load() {
		return fmt.Errorf("persistent store is closed")
	}

	return nil
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
	if err := ps.requireWritable(); err != nil {
		return err
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

			if err := writeFull(ps.walWriter, frame[:]); err != nil {
				return err
			}

			if err := writeFull(ps.walWriter, key); err != nil {
				return err
			}

			if err := writeFull(ps.walWriter, value); err != nil {
				return err
			}

			return nil
		})

		guardStep(ps.state, ps.walWriter.Flush)

		return ps.state.Err()
	})

	if err != nil {
		return ps.markFatal(err)
	}

	ps.lastTerm.Store(term)
	ps.lastIndex.Store(index)

	return nil
}

func (ps *PersistentStore) LogInserts(entries []WALEntry) error {
	if err := ps.requireWritable(); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	err := ps.runWal("insert-batch", func() error {
		ps.state.Reset()

		for _, entry := range entries {
			entry := entry

			guardStep(ps.state, func() error {
				var frame [25]byte

				frame[0] = opInsert
				binary.LittleEndian.PutUint64(frame[1:9], entry.Term)
				binary.LittleEndian.PutUint64(frame[9:17], entry.Index)
				binary.LittleEndian.PutUint32(frame[17:21], uint32(len(entry.Key)))
				binary.LittleEndian.PutUint32(frame[21:25], uint32(len(entry.Value)))

				if err := writeFull(ps.walWriter, frame[:]); err != nil {
					return err
				}

				if err := writeFull(ps.walWriter, entry.Key); err != nil {
					return err
				}

				if err := writeFull(ps.walWriter, entry.Value); err != nil {
					return err
				}

				return nil
			})
		}

		guardStep(ps.state, ps.walWriter.Flush)

		return ps.state.Err()
	})

	if err != nil {
		return ps.markFatal(err)
	}

	last := entries[len(entries)-1]
	ps.lastTerm.Store(last.Term)
	ps.lastIndex.Store(last.Index)

	return nil
}

/*
LogDelete logs a delete operation to the WAL through the serialized worker pool.
*/
func (ps *PersistentStore) LogDelete(key []byte, term, index uint64) error {
	if err := ps.requireWritable(); err != nil {
		return err
	}

	err := ps.runWal("delete", func() error {
		ps.state.Reset()

		guardStep(ps.state, func() error {
			var frame [21]byte

			frame[0] = opDelete
			binary.LittleEndian.PutUint64(frame[1:9], term)
			binary.LittleEndian.PutUint64(frame[9:17], index)
			binary.LittleEndian.PutUint32(frame[17:21], uint32(len(key)))

			if err := writeFull(ps.walWriter, frame[:]); err != nil {
				return err
			}

			return writeFull(ps.walWriter, key)
		})

		guardStep(ps.state, ps.walWriter.Flush)

		return ps.state.Err()
	})

	if err != nil {
		return ps.markFatal(err)
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

	err := ps.runWal("close", func() error {
		ps.state.Reset()

		if ps.walWriter != nil {
			guardStep(ps.state, ps.walWriter.Flush)
		}
		if ps.walFile != nil {
			_ = ps.walFile.Sync()
			guardStep(ps.state, ps.walFile.Close)
		}

		ps.walFile = nil
		ps.walWriter = nil

		return ps.state.Err()
	})

	if ps.cancel != nil {
		ps.cancel()
	}

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
	if err := ps.requireWritable(); err != nil {
		return err
	}

	err := ps.runWal("term", func() error {
		ps.state.Reset()

		guardStep(ps.state, func() error {
			var frame [9]byte

			frame[0] = opTermUpdate
			binary.LittleEndian.PutUint64(frame[1:9], term)

			return writeFull(ps.walWriter, frame[:])
		})

		guardStep(ps.state, ps.walWriter.Flush)

		return ps.state.Err()
	})

	if err != nil {
		return ps.markFatal(err)
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
			if errors.Is(err, io.EOF) {
				break
			}

			return entries, ps.markFatal(err)
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

			if err := validateWALLength("key", keyLen); err != nil {
				return entries, ps.markFatal(err)
			}

			if err := validateWALLength("value", valLen); err != nil {
				return entries, ps.markFatal(err)
			}

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

			if err := validateWALLength("key", keyLen); err != nil {
				return entries, ps.markFatal(err)
			}

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
			return entries, ps.markFatal(fmt.Errorf("invalid wal operation: %d", op))
		}
	}

	if ps.state.Failed() {
		return entries, ps.markFatal(ps.state.Err())
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

func (ps *PersistentStore) CreateSnapshot(
	iterator func(yield func(key, value []byte) bool),
) error {
	if err := ps.requireWritable(); err != nil {
		return err
	}

	if iterator == nil {
		return fmt.Errorf("persistent snapshot requires active tree iterator")
	}

	now := time.Now().UnixNano()
	lastSnap := ps.lastSnap.Load()

	if now-lastSnap < int64(time.Minute) {
		return nil
	}

	if !ps.lastSnap.CompareAndSwap(lastSnap, now) {
		return nil
	}

	err := ps.persistWal(func() error {
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

			return writeFull(file, stateFrame[:])
		})

		guardStep(ps.state, func() error {
			return ps.truncateWAL(iterator)
		})

		return ps.state.Err()
	})

	if err != nil {
		return ps.markFatal(err)
	}

	return nil
}

func (ps *PersistentStore) truncateWAL(
	iterator func(yield func(key, value []byte) bool),
) error {
	if iterator == nil {
		return fmt.Errorf("persistent wal truncation requires active tree iterator")
	}

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

		return writeFull(writer, walFrame[:])
	})

	var writeErr error
	term := ps.lastTerm.Load()
	index := ps.lastIndex.Load()

	iterator(func(key, value []byte) bool {
		if writeErr != nil {
			return false
		}

		var frame [25]byte

		frame[0] = opInsert
		binary.LittleEndian.PutUint64(frame[1:9], term)
		binary.LittleEndian.PutUint64(frame[9:17], index)
		binary.LittleEndian.PutUint32(frame[17:21], uint32(len(key)))
		binary.LittleEndian.PutUint32(frame[21:25], uint32(len(value)))

		if writeErr = writeFull(writer, frame[:]); writeErr != nil {
			return false
		}
		if writeErr = writeFull(writer, key); writeErr != nil {
			return false
		}
		if writeErr = writeFull(writer, value); writeErr != nil {
			return false
		}

		return true
	})

	if writeErr != nil {
		guardStep(ps.state, func() error {
			return writeErr
		})
	}

	guardStep(ps.state, writer.Flush)
	guardStep(ps.state, newFile.Sync)
	guardStep(ps.state, newFile.Close)

	if ps.walWriter != nil {
		guardStep(ps.state, ps.walWriter.Flush)
	}
	if ps.walFile != nil {
		_ = ps.walFile.Sync()
	}

	guardStep(ps.state, func() error {
		return os.Rename(newPath, ps.walPath)
	})

	if ps.walFile != nil {
		guardStep(ps.state, ps.walFile.Close)
	}

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

func (ps *PersistentStore) TruncateWAL(
	iterator func(yield func(key, value []byte) bool),
) error {
	if err := ps.requireWritable(); err != nil {
		return err
	}

	return ps.runWal("truncate", func() error {
		ps.state.Reset()

		if err := ps.truncateWAL(iterator); err != nil {
			return ps.markFatal(err)
		}

		return nil
	})
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

func writeFull(writer io.Writer, data []byte) error {
	written, err := writer.Write(data)
	if err != nil {
		return err
	}

	if written != len(data) {
		return io.ErrShortWrite
	}

	return nil
}

func validateWALLength(field string, length uint32) error {
	if length > maxWALFieldBytes {
		return fmt.Errorf(
			"invalid wal %s length %d exceeds max %d",
			field,
			length,
			maxWALFieldBytes,
		)
	}

	return nil
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
