package transport

import (
	"io"

	"github.com/theapemachine/datura"
)

type Coupler struct {
	origin      io.ReadWriter
	destination io.ReadWriter
}

func NewCoupler() *Coupler {
	return &Coupler{}
}

/*
Connect binds origin then destination and returns the coupler for chaining.
*/
func (coupler *Coupler) Connect(rw io.ReadWriter) *Coupler {
	if coupler.origin == nil {
		coupler.origin = rw

		return coupler
	}

	if coupler.destination == nil {
		coupler.destination = rw
	}

	return coupler
}

func (coupler *Coupler) Read(p []byte) (int, error) {
	_, err := io.Copy(coupler.destination, coupler.origin)

	if err != nil && err != io.EOF {
		return 0, err
	}

	return coupler.destination.Read(p)
}

func (coupler *Coupler) Write(p []byte) (int, error) {
	if coupler.routeWriteToDestination(p) {
		return coupler.destination.Write(p)
	}

	return coupler.origin.Write(p)
}

func (coupler *Coupler) Close() error {
	return nil
}

func (coupler *Coupler) routeWriteToDestination(frame []byte) bool {
	inbound := datura.Acquire("coupler", datura.Artifact_Type_json)
	_, _ = inbound.Write(frame)

	return datura.Peek[string](inbound, "destination") == "median"
}
