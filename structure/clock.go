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
	Click uint64
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
type ClockRing struct {
	SecondHand *ListRing[ClockSlot]
	LittleHand *ListRing[ClockSlot]
	BigHand    *ListRing[ClockSlot]
	clicks     uint64
	secondLap  int
	littleLap  int
}

/*
NewClockRing builds a click clock with positive second, little, and big capacities.
*/
func NewClockRing(secondCapacity, littleCapacity, bigCapacity int) (*ClockRing, error) {
	if secondCapacity <= 0 || littleCapacity <= 0 || bigCapacity <= 0 {
		return nil, errors.New("structure: clock ring capacities must be positive")
	}

	secondHand := NewListRing[ClockSlot](secondCapacity)
	littleHand := NewListRing[ClockSlot](littleCapacity)
	bigHand := NewListRing[ClockSlot](bigCapacity)

	if secondHand == nil || littleHand == nil || bigHand == nil {
		return nil, fmt.Errorf("structure: clock ring allocation failed")
	}

	return &ClockRing{
		SecondHand: secondHand,
		LittleHand: littleHand,
		BigHand:    bigHand,
	}, nil
}

/*
NewDefaultClockRing returns a click clock with 10, 100, and 1000 slot hands.
*/
func NewDefaultClockRing() (*ClockRing, error) {
	return NewClockRing(10, 100, 1000)
}

/*
Click returns the monotonic virtual click counter.
*/
func (clock *ClockRing) Click() uint64 {
	if clock == nil {
		return 0
	}

	return clock.clicks
}

/*
ObserveSecond records a fresh second-hand click at wall and cascades slower hands.
*/
func (clock *ClockRing) ObserveSecond(wall time.Time) (ClockCascade, error) {
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
func (clock *ClockRing) ObserveLittle(wall time.Time) (ClockCascade, error) {
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
func (clock *ClockRing) ObserveBig(wall time.Time) error {
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
func (clock *ClockRing) AdvanceVirtual(clicks int) ([]ClockCascade, error) {
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
func (clock *ClockRing) Freshness(hand Hand) (int, error) {
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
func (clock *ClockRing) HandRing(hand Hand) (*ListRing[ClockSlot], error) {
	if clock == nil {
		return nil, errors.New("structure: clock ring is nil")
	}

	ring := clock.handRing(hand)

	if ring == nil {
		return nil, fmt.Errorf("structure: clock hand %d is invalid", hand)
	}

	return ring, nil
}

func (clock *ClockRing) pushSecond(slot ClockSlot) ClockCascade {
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

func (clock *ClockRing) pushLittle(slot ClockSlot) ClockCascade {
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

func (clock *ClockRing) pushBig(slot ClockSlot) {
	if slot.Click == 0 {
		slot.Click = clock.clicks
	}

	clock.BigHand.Push(slot)
}

func (clock *ClockRing) handRing(hand Hand) *ListRing[ClockSlot] {
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
