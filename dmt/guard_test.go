package dmt

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGuardValue(test *testing.T) {
	cases := []struct {
		name       string
		run        func(test *testing.T, batch *batch) int
		wantZero   bool
		wantFailed bool
	}{
		{
			name: "returns value when all steps succeed",
			run: func(test *testing.T, batch *batch) int {
				first := guardValue(batch, func() (int, error) {
					return 7, nil
				})

				second := guardValue(batch, func() (int, error) {
					return first + 5, nil
				})

				return second
			},
			wantZero:   false,
			wantFailed: false,
		},
		{
			name: "returns zero after the first failing step",
			run: func(test *testing.T, batch *batch) int {
				_ = guardValue(batch, func() (int, error) {
					return 0, errors.New("first failure")
				})

				return guardValue(batch, func() (int, error) {
					test.Fatal("subsequent guardValue step should not run")
					return 99, nil
				})
			},
			wantZero:   true,
			wantFailed: true,
		},
		{
			name: "skips subsequent guardValue steps after failure",
			run: func(test *testing.T, batch *batch) int {
				_ = guardValue(batch, func() (int, error) {
					return 11, nil
				})

				_ = guardValue(batch, func() (int, error) {
					return 0, errors.New("second failure")
				})

				return guardValue(batch, func() (int, error) {
					test.Fatal("guardValue should stop after batch failure")
					return 42, nil
				})
			},
			wantZero:   true,
			wantFailed: true,
		},
	}

	for _, testCase := range cases {
		testCase := testCase

		test.Run(testCase.name, func(nestedTest *testing.T) {
			Convey("Given guardValue batch steps", nestedTest, func() {
				batch := newBatch("guardValue")

				value := testCase.run(nestedTest, batch)

				Convey("Then batch failure state should match expectations", func() {
					So(batch.Failed(), ShouldEqual, testCase.wantFailed)

					if testCase.wantFailed {
						So(batch.Err(), ShouldNotBeNil)
					}

					if testCase.wantZero {
						So(value, ShouldEqual, 0)
					}
				})
			})
		})
	}
}

func TestGuardStep(test *testing.T) {
	cases := []struct {
		name       string
		run        func(test *testing.T, batch *batch)
		wantFailed bool
	}{
		{
			name: "runs all steps when none fail",
			run: func(test *testing.T, batch *batch) {
				ran := 0

				guardStep(batch, func() error {
					ran++
					return nil
				})

				guardStep(batch, func() error {
					ran++
					return nil
				})

				So(ran, ShouldEqual, 2)
			},
			wantFailed: false,
		},
		{
			name: "first error stops subsequent guardStep calls",
			run: func(test *testing.T, batch *batch) {
				guardStep(batch, func() error {
					return errors.New("first failure")
				})

				guardStep(batch, func() error {
					test.Fatal("subsequent guardStep should not run")
					return nil
				})
			},
			wantFailed: true,
		},
		{
			name: "preserves the first failure across later steps",
			run: func(test *testing.T, batch *batch) {
				firstErr := errors.New("first failure")

				guardStep(batch, func() error {
					return firstErr
				})

				guardStep(batch, func() error {
					return errors.New("second failure")
				})
			},
			wantFailed: true,
		},
	}

	for _, testCase := range cases {
		testCase := testCase

		test.Run(testCase.name, func(nestedTest *testing.T) {
			Convey("Given guardStep batch steps", nestedTest, func() {
				batch := newBatch("guardStep")

				testCase.run(nestedTest, batch)

				Convey("Then batch failure state should match expectations", func() {
					So(batch.Failed(), ShouldEqual, testCase.wantFailed)

					if testCase.wantFailed {
						So(batch.Err(), ShouldNotBeNil)
					}
				})
			})
		})
	}
}
