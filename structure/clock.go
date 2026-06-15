package structure

import (
	"errors"
	"fmt"
	"time"
)

var (
	errClockNil        = errors.New("structure: clock ring is nil")
	errClockTrackNil   = errors.New("structure: clock track is nil")
	errClockTrackAlloc = errors.New("structure: clock track allocation failed")
	errClockTrackHold  = errors.New("structure: clock track has no second-hand value to hold")
	errClockHand       = errors.New("structure: clock hand is invalid")
)

/*
Hand names one gear on the virtual click clock.
*/
type Hand int

const (
	HandSecond Hand = iota
	HandLittle
	HandBig
)

/*
ClockSlot is one click on a hand. Wall is zero for virtual fills that hold the
previous observation across click space without a new market event.
*/
type ClockSlot struct {
	Wall  time.Time
	Click int64
}

/*
Fresh reports whether the slot came from a real observation timestamp.
*/
func (slot ClockSlot) Fresh() bool {
	return !slot.Wall.IsZero()
}

/*
ClockCascade reports which slower hands ticked on one second-hand click.
*/
type ClockCascade struct {
	Little bool
	Big    bool
}

/*
ClockRing is a three-gear virtual click clock backed by ListRings.

SecondHand advances on every click. Each time SecondHand completes one lap,
LittleHand advances once; each LittleHand lap advances BigHand once. Virtual
clicks advance SecondHand without a wall timestamp so sparse streams can fill
click space for sequence detection at the cost of freshness.
*/
type ClockRing[T any] struct {
	SecondHand *ListRing[ClockSlot]
	LittleHand *ListRing[ClockSlot]
	BigHand    *ListRing[ClockSlot]
	clicks     int64
	secondLap  int
	littleLap  int
}

/*
NewClockRing builds a click clock with positive second, little, and big capacities.
*/
func NewClockRing[T any](secondCapacity, littleCapacity, bigCapacity int) (*ClockRing[T], error) {
	if secondCapacity <= 0 || littleCapacity <= 0 || bigCapacity <= 0 {
		return nil, errors.New("structure: clock ring capacities must be positive")
	}

	secondHand := NewListRing[ClockSlot](secondCapacity)
	littleHand := NewListRing[ClockSlot](littleCapacity)
	bigHand := NewListRing[ClockSlot](bigCapacity)

	if secondHand == nil || littleHand == nil || bigHand == nil {
		return nil, fmt.Errorf("structure: clock ring allocation failed")
	}

	return &ClockRing[T]{
		SecondHand: secondHand,
		LittleHand: littleHand,
		BigHand:    bigHand,
	}, nil
}

/*
NewDefaultClockRing returns a click clock with 10, 100, and 1000 slot hands.
*/
func NewDefaultClockRing[T any]() (*ClockRing[T], error) {
	return NewClockRing[T](10, 100, 1000)
}

/*
Click returns the monotonic virtual click counter.
*/
func (clock *ClockRing[T]) Click() int64 {
	if clock == nil {
		return 0
	}

	return clock.clicks
}

/*
ObserveSecond records a fresh second-hand click at wall and cascades slower hands.
*/
func (clock *ClockRing[T]) ObserveSecond(wall time.Time) (ClockCascade, error) {
	if clock == nil {
		return ClockCascade{}, errors.New("structure: clock ring is nil")
	}

	if wall.IsZero() {
		return ClockCascade{}, errors.New("structure: clock ObserveSecond requires wall time")
	}

	return clock.pushSecond(ClockSlot{Wall: wall}), nil
}

/*
ObserveLittle records a fresh little-hand click at wall and may cascade BigHand.
*/
func (clock *ClockRing[T]) ObserveLittle(wall time.Time) (ClockCascade, error) {
	if clock == nil {
		return ClockCascade{}, errors.New("structure: clock ring is nil")
	}

	if wall.IsZero() {
		return ClockCascade{}, errors.New("structure: clock ObserveLittle requires wall time")
	}

	return clock.pushLittle(ClockSlot{Wall: wall}), nil
}

/*
ObserveBig records a fresh big-hand click at wall.
*/
func (clock *ClockRing[T]) ObserveBig(wall time.Time) error {
	if clock == nil {
		return errors.New("structure: clock ring is nil")
	}

	if wall.IsZero() {
		return errors.New("structure: clock ObserveBig requires wall time")
	}

	clock.pushBig(ClockSlot{Wall: wall})

	return nil
}

/*
AdvanceVirtual advances SecondHand by clicks virtual steps without wall time.
*/
func (clock *ClockRing[T]) AdvanceVirtual(clicks int) ([]ClockCascade, error) {
	if clock == nil {
		return nil, errors.New("structure: clock ring is nil")
	}

	if clicks <= 0 {
		return nil, errors.New("structure: clock AdvanceVirtual requires positive clicks")
	}

	cascades := make([]ClockCascade, 0, clicks)

	for step := 0; step < clicks; step++ {
		cascades = append(cascades, clock.pushSecond(ClockSlot{}))
	}

	return cascades, nil
}

/*
Freshness returns how many clicks back the latest fresh slot sits on hand.
Zero means the latest slot is fresh.
*/
func (clock *ClockRing[T]) Freshness(hand Hand) (int, error) {
	if clock == nil {
		return 0, errors.New("structure: clock ring is nil")
	}

	ring := clock.handRing(hand)

	if ring == nil {
		return 0, fmt.Errorf("structure: clock hand %d is invalid", hand)
	}

	if ring.Len() == 0 {
		return 0, nil
	}

	steps := 0
	walk := ring.Select(-1)

	for steps < ring.Len() {
		slot := walk.Pop()

		if slot.Fresh() {
			return steps, nil
		}

		steps++
		walk = walk.Select(-1)
	}

	return steps, nil
}

/*
HandRing returns the slot ring for hand.
*/
func (clock *ClockRing[T]) HandRing(hand Hand) (Ring[ClockSlot], error) {
	if clock == nil {
		return nil, errors.New("structure: clock ring is nil")
	}

	ring := clock.handRing(hand)

	if ring == nil {
		return nil, fmt.Errorf("structure: clock hand %d is invalid", hand)
	}

	return ring, nil
}

/*
Push records slot on the second hand and cascades slower hands when a lap completes.
*/
func (clock *ClockRing[T]) Push(slot ClockSlot) bool {
	if clock == nil {
		return false
	}

	clock.pushSecond(slot)

	return true
}

/*
Pop returns the second-hand slot at the write cursor without advancing it.
*/
func (clock *ClockRing[T]) Pop() ClockSlot {
	if clock == nil || clock.SecondHand == nil {
		return ClockSlot{}
	}

	return clock.SecondHand.Pop()
}

/*
Select returns a second-hand view offset step slots from the write cursor.
*/
func (clock *ClockRing[T]) Select(step int) Ring[ClockSlot] {
	if clock == nil || clock.SecondHand == nil {
		return nil
	}

	return clock.SecondHand.Select(step)
}

/*
Merge splices other into this clock. When other is another ClockRing, all three
hands merge and lap counters reset. When other is a ListRing, only SecondHand merges.
*/
func (clock *ClockRing[T]) Merge(other Ring[ClockSlot]) bool {
	if clock == nil {
		return false
	}

	otherClock, ok := other.(*ClockRing[T])

	if !ok {
		return clock.SecondHand.Merge(other)
	}

	if !clock.SecondHand.Merge(otherClock.SecondHand) {
		return false
	}

	if !clock.LittleHand.Merge(otherClock.LittleHand) {
		return false
	}

	if !clock.BigHand.Merge(otherClock.BigHand) {
		return false
	}

	if otherClock.clicks > clock.clicks {
		clock.clicks = otherClock.clicks
	}

	clock.secondLap = 0
	clock.littleLap = 0

	return true
}

/*
Slice detaches count second-hand slots into a new ring view.
*/
func (clock *ClockRing[T]) Slice(count int) Ring[ClockSlot] {
	if clock == nil || clock.SecondHand == nil {
		return nil
	}

	return clock.SecondHand.Slice(count)
}

/*
Len returns the second-hand capacity in slots.
*/
func (clock *ClockRing[T]) Len() int {
	if clock == nil || clock.SecondHand == nil {
		return 0
	}

	return clock.SecondHand.Len()
}

/*
Do visits every second-hand slot in cursor order.
*/
func (clock *ClockRing[T]) Do(visitor func(ClockSlot)) {
	if clock == nil || clock.SecondHand == nil {
		return
	}

	clock.SecondHand.Do(visitor)
}

/*
Close is a no-op. It exists so ClockRing satisfies Ring[ClockSlot].
*/
func (clock *ClockRing[T]) Close() error {
	return nil
}

/*
Error is always nil. It exists so ClockRing satisfies Ring[ClockSlot].
*/
func (clock *ClockRing[T]) Error() error {
	return nil
}

func (clock *ClockRing[T]) pushSecond(slot ClockSlot) ClockCascade {
	clock.clicks++
	slot.Click = clock.clicks
	clock.SecondHand.Push(slot)
	clock.secondLap++

	if clock.secondLap < clock.SecondHand.Len() {
		return ClockCascade{}
	}

	clock.secondLap = 0

	return clock.pushLittle(slot)
}

func (clock *ClockRing[T]) pushLittle(slot ClockSlot) ClockCascade {
	if slot.Click == 0 {
		slot.Click = clock.clicks
	}

	clock.LittleHand.Push(slot)
	clock.littleLap++

	if clock.littleLap < clock.LittleHand.Len() {
		return ClockCascade{Little: true}
	}

	clock.littleLap = 0
	clock.pushBig(slot)

	return ClockCascade{Little: true, Big: true}
}

func (clock *ClockRing[T]) pushBig(slot ClockSlot) {
	if slot.Click == 0 {
		slot.Click = clock.clicks
	}

	clock.BigHand.Push(slot)
}

func (clock *ClockRing[T]) handRing(hand Hand) *ListRing[ClockSlot] {
	switch hand {
	case HandSecond:
		return clock.SecondHand
	case HandLittle:
		return clock.LittleHand
	case HandBig:
		return clock.BigHand
	default:
		return nil
	}
}
