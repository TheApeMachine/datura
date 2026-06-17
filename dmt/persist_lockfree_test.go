package dmt

import (
	"strconv"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPersistentStoreConcurrentLogInsert(test *testing.T) {
	Convey("Given concurrent WAL writers", test, func() {
		tmpDir := test.TempDir()

		store, err := NewPersistentStore(tmpDir)
		So(err, ShouldBeNil)
		defer store.Close()

		var waitGroup sync.WaitGroup
		errors := make(chan error, 32)

		for workerIndex := range 32 {
			waitGroup.Add(1)

			go func(index int) {
				defer waitGroup.Done()

				key := []byte("key/" + strconv.Itoa(index))
				value := []byte("value/" + strconv.Itoa(index))
				logErr := store.LogInsert(key, value, 1, uint64(index+1))

				if logErr != nil {
					errors <- logErr
				}
			}(workerIndex)
		}

		waitGroup.Wait()
		close(errors)

		for logErr := range errors {
			So(logErr, ShouldBeNil)
		}

		Convey("It should persist every insert", func() {
			entries, replayErr := store.Replay()
			So(replayErr, ShouldBeNil)
			So(len(entries), ShouldEqual, 32)
		})
	})
}
