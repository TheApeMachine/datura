package dmt

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var errInjectedWALSync = errors.New("injected wal sync failure")

/*
faultInjectWALFile wraps a real WAL file and fails Sync on a configured call.
*/
type faultInjectWALFile struct {
	file       *os.File
	syncFailAt int
	syncCount  int
}

func (faultFile *faultInjectWALFile) Write(payload []byte) (int, error) {
	return faultFile.file.Write(payload)
}

func (faultFile *faultInjectWALFile) Sync() error {
	faultFile.syncCount++

	if faultFile.syncFailAt > 0 && faultFile.syncCount >= faultFile.syncFailAt {
		return errInjectedWALSync
	}

	return faultFile.file.Sync()
}

func (faultFile *faultInjectWALFile) Close() error {
	return faultFile.file.Close()
}

func (faultFile *faultInjectWALFile) Stat() (os.FileInfo, error) {
	return faultFile.file.Stat()
}

func (faultFile *faultInjectWALFile) Truncate(size int64) error {
	return faultFile.file.Truncate(size)
}

func (faultFile *faultInjectWALFile) Seek(offset int64, whence int) (int64, error) {
	return faultFile.file.Seek(offset, whence)
}

func withFaultInjectWAL(store *PersistentStore, syncFailAt int) func() {
	original, ok := store.walFile.(*os.File)

	if !ok {
		panic("expected underlying WAL to be *os.File")
	}

	store.walFile = &faultInjectWALFile{
		file:       original,
		syncFailAt: syncFailAt,
	}

	return func() {
		if faultFile, isFault := store.walFile.(*faultInjectWALFile); isFault {
			store.walFile = faultFile.file
		}
	}
}

func TestLogInsertSyncFailureDoesNotAdvanceIndex(t *testing.T) {
	Convey("Given a WAL whose fsync fails on the first commit", t, func() {
		tmpDir := t.TempDir()

		store, err := NewPersistentStore(tmpDir)
		So(err, ShouldBeNil)

		restore := withFaultInjectWAL(store, 1)
		defer restore()
		defer store.Close()

		term, index := store.GetLastState()
		So(term, ShouldEqual, uint64(0))
		So(index, ShouldEqual, uint64(0))

		Convey("When logging an insert", func() {
			logErr := store.LogInsert([]byte("key"), []byte("value"), 1, 1)

			Convey("Then the write should fail and last index must not advance", func() {
				So(logErr, ShouldNotBeNil)

				lastTerm, lastIndex := store.GetLastState()
				So(lastTerm, ShouldEqual, term)
				So(lastIndex, ShouldEqual, index)
			})
		})
	})
}

func TestLogInsertSyncFailureLeavesEmptyWALOnReopen(t *testing.T) {
	Convey("Given a WAL whose fsync fails on the first commit", t, func() {
		tmpDir := t.TempDir()

		store, err := NewPersistentStore(tmpDir)
		So(err, ShouldBeNil)

		restore := withFaultInjectWAL(store, 1)
		defer restore()

		So(store.LogInsert([]byte("key"), []byte("value"), 1, 1), ShouldNotBeNil)
		_ = store.Close()

		Convey("When reopening after the failed commit", func() {
			reopened, reopenErr := NewPersistentStore(tmpDir)
			So(reopenErr, ShouldBeNil)
			defer reopened.Close()

			entries, replayErr := reopened.Replay()
			So(replayErr, ShouldBeNil)
			So(len(entries), ShouldEqual, 0)

			replayTerm, replayIndex := reopened.GetLastState()
			So(replayTerm, ShouldEqual, uint64(0))
			So(replayIndex, ShouldEqual, uint64(0))
		})
	})
}

func TestLogInsertSyncFailureMarksStoreFatal(t *testing.T) {
	Convey("Given a WAL whose fsync fails", t, func() {
		tmpDir := t.TempDir()

		store, err := NewPersistentStore(tmpDir)
		So(err, ShouldBeNil)

		restore := withFaultInjectWAL(store, 1)
		defer restore()
		defer store.Close()

		So(store.LogInsert([]byte("key"), []byte("value"), 1, 1), ShouldNotBeNil)

		Convey("When attempting another insert", func() {
			logErr := store.LogInsert([]byte("later"), []byte("value"), 1, 2)

			Convey("Then the store should remain fail-closed", func() {
				So(logErr, ShouldNotBeNil)
				So(store.fatalError(), ShouldNotBeNil)
			})
		})
	})
}

func TestWALCrashBoundaryAcknowledgedRecordsSurviveReopen(t *testing.T) {
	Convey("Given a store with multiple fsync-acknowledged inserts", t, func() {
		tmpDir := t.TempDir()

		store, err := NewPersistentStore(tmpDir)
		So(err, ShouldBeNil)

		entries := []struct {
			key   string
			value string
			index uint64
		}{
			{"alpha", "one", 1},
			{"beta", "two", 2},
			{"gamma", "three", 3},
		}

		for _, entry := range entries {
			So(
				store.LogInsert([]byte(entry.key), []byte(entry.value), 1, entry.index),
				ShouldBeNil,
			)
		}

		walPath := filepath.Join(tmpDir, "wal.log")
		walInfo, statErr := os.Stat(walPath)
		So(statErr, ShouldBeNil)
		So(store.Close(), ShouldBeNil)

		Convey("When reopening after a simulated crash", func() {
			reopened, reopenErr := NewPersistentStore(tmpDir)
			So(reopenErr, ShouldBeNil)
			defer reopened.Close()

			Convey("Then every acknowledged record should replay", func() {
				replayed, replayErr := reopened.Replay()
				So(replayErr, ShouldBeNil)
				So(len(replayed), ShouldEqual, len(entries))

				for index, entry := range entries {
					So(string(replayed[index].Key), ShouldEqual, entry.key)
					So(string(replayed[index].Value), ShouldEqual, entry.value)
					So(replayed[index].Index, ShouldEqual, entry.index)
				}

				_, lastIndex := reopened.GetLastState()
				So(lastIndex, ShouldEqual, entries[len(entries)-1].index)

				reopenedInfo, reopenedStatErr := os.Stat(walPath)
				So(reopenedStatErr, ShouldBeNil)
				So(reopenedInfo.Size(), ShouldEqual, walInfo.Size())
			})
		})
	})
}

func TestValidateWALMonotonicRejectsRegression(t *testing.T) {
	Convey("Given WAL entries with a regressing index", t, func() {
		entries := []WALEntry{
			{Op: opInsert, Term: 2, Index: 5, Key: []byte("a"), Value: []byte("1")},
			{Op: opInsert, Term: 2, Index: 4, Key: []byte("b"), Value: []byte("2")},
		}

		Convey("validateWALMonotonic should reject the batch", func() {
			So(validateWALMonotonic(entries), ShouldNotBeNil)
		})
	})

	Convey("Given WAL entries with a regressing term", t, func() {
		entries := []WALEntry{
			{Op: opInsert, Term: 3, Index: 1, Key: []byte("a"), Value: []byte("1")},
			{Op: opInsert, Term: 2, Index: 2, Key: []byte("b"), Value: []byte("2")},
		}

		Convey("validateWALMonotonic should reject the batch", func() {
			So(validateWALMonotonic(entries), ShouldNotBeNil)
		})
	})
}
