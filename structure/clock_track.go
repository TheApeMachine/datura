package structure

import "time"

/*
ClockTrack pairs a ClockRing with aligned value rings on each hand.

Values on a hand advance when that hand ticks on the clock. Virtual second-hand
clicks repeat the last second-hand value; cascaded little and big ticks repeat
their own last observed values.
*/
type ClockTrack[T any] struct {
	clock        *ClockRing[T]
	secondValues *ListRing[T]
	littleValues *ListRing[T]
	bigValues    *ListRing[T]
	secondLast   T
	littleLast   T
	bigLast      T
	hasSecond    bool
	hasLittle    bool
	hasBig       bool
}

/*
NewClockTrack binds value rings to an existing click clock.
*/
func NewClockTrack[T any](clock *ClockRing[T]) (*ClockTrack[T], error) {
	if clock == nil {
		return nil, errClockNil
	}

	secondValues := NewListRing[T](clock.SecondHand.Len())
	littleValues := NewListRing[T](clock.LittleHand.Len())
	bigValues := NewListRing[T](clock.BigHand.Len())

	if secondValues == nil || littleValues == nil || bigValues == nil {
		return nil, errClockTrackAlloc
	}

	return &ClockTrack[T]{
		clock:        clock,
		secondValues: secondValues,
		littleValues: littleValues,
		bigValues:    bigValues,
	}, nil
}

/*
ObserveSecond records value on a fresh second-hand click.
*/
func (track *ClockTrack[T]) ObserveSecond(wall time.Time, value T) (ClockCascade, error) {
	if track == nil {
		return ClockCascade{}, errClockTrackNil
	}

	track.secondLast = value
	track.hasSecond = true
	track.secondValues.Push(value)

	cascade, err := track.clock.ObserveSecond(wall, value)

	if err != nil {
		return ClockCascade{}, err
	}

	track.applyCascade(cascade)

	return cascade, nil
}

/*
ObserveLittle records value on a fresh little-hand click.
*/
func (track *ClockTrack[T]) ObserveLittle(wall time.Time, value T) (ClockCascade, error) {
	if track == nil {
		return ClockCascade{}, errClockTrackNil
	}

	track.littleLast = value
	track.hasLittle = true
	track.littleValues.Push(value)

	cascade, err := track.clock.ObserveLittle(wall)

	if err != nil {
		return ClockCascade{}, err
	}

	if cascade.Big {
		track.pushBigHold()
	}

	return cascade, nil
}

/*
ObserveBig records value on a fresh big-hand click.
*/
func (track *ClockTrack[T]) ObserveBig(wall time.Time, value T) error {
	if track == nil {
		return errClockTrackNil
	}

	track.bigLast = value
	track.hasBig = true
	track.bigValues.Push(value)

	return track.clock.ObserveBig(wall)
}

/*
AdvanceVirtual fills second-hand click space with the last second-hand value.
*/
func (track *ClockTrack[T]) AdvanceVirtual(clicks int) ([]ClockCascade, error) {
	if track == nil {
		return nil, errClockTrackNil
	}

	if !track.hasSecond {
		return nil, errClockTrackHold
	}

	cascades, err := track.clock.AdvanceVirtual(clicks)

	if err != nil {
		return nil, err
	}

	for _, cascade := range cascades {
		track.secondValues.Push(track.secondLast)
		track.applyCascade(cascade)
	}

	return cascades, nil
}

/*
ValueRing returns the aligned value ring for hand.
*/
func (track *ClockTrack[T]) ValueRing(hand Hand) (*ListRing[T], error) {
	if track == nil {
		return nil, errClockTrackNil
	}

	switch hand {
	case HandSecond:
		return track.secondValues, nil
	case HandLittle:
		return track.littleValues, nil
	case HandBig:
		return track.bigValues, nil
	default:
		return nil, errClockHand
	}
}

func (track *ClockTrack[T]) applyCascade(cascade ClockCascade) {
	if cascade.Little {
		track.pushLittleHold()
	}

	if cascade.Big {
		track.pushBigHold()
	}
}

func (track *ClockTrack[T]) pushLittleHold() {
	if !track.hasLittle {
		return
	}

	track.littleValues.Push(track.littleLast)
}

func (track *ClockTrack[T]) pushBigHold() {
	if !track.hasBig {
		return
	}

	track.bigValues.Push(track.bigLast)
}
