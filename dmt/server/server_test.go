package server

import (
	"context"
	"testing"

	capnp "capnproto.org/go/capnp/v3"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

func TestForest(t *testing.T) {
	Convey("Given a newly initialized Forest Server", t, func() {
		ctx := context.Background()
		server := NewForestServer(WithContext(ctx))
		client := Server_ServerToClient(server)
		defer client.Release()

		morton := datura.NewMortonCoder()

		Convey("It should drop incoming morton keys onto the exact grid coordinates", func() {
			keyH := morton.Encode(0, uint64('H'))
			artifact := datura.Acquire("rpc-write", datura.APPJSON)
			defer artifact.Release()
			artifact.WithPayload([]byte(`{"letter":"H"}`))
			value := artifact.Pack()

			err := client.Write(ctx, func(p Server_write_Params) error {
				p.SetKey(keyH)
				return p.SetValue(value)
			})
			So(err, ShouldBeNil)

			Convey("It should be retrievable via Lookup", func() {
				future, release := client.Lookup(ctx, func(p Server_lookup_Params) error {
					list, err := capnp.NewUInt64List(p.Segment(), 1)
					if err != nil {
						return err
					}
					list.Set(0, keyH)
					p.SetKeys(list)
					return nil
				})
				defer release()

				res, err := future.Struct()
				So(err, ShouldBeNil)
				values, err := res.Values()
				So(err, ShouldBeNil)
				So(values.Len(), ShouldEqual, 1)

				inbound := values.At(0)
				payload := (&inbound).DecryptPayload()
				So(payload, ShouldResemble, []byte(`{"letter":"H"}`))
			})

			Convey("It should silently drop duplicate keys reflecting collision entropy", func() {
				// We invoke a duplicate insert. It must return without error.
				err2 := client.Write(ctx, func(p Server_write_Params) error {
					p.SetKey(keyH)
					return p.SetValue(value)
				})
				So(err2, ShouldBeNil)
			})

		})
	})
}

func BenchmarkForestWrite(b *testing.B) {
	ctx := context.Background()
	server := NewForestServer(WithContext(ctx))
	client := Server_ServerToClient(server)
	defer client.Release()

	morton := datura.NewMortonCoder()
	keys := make([]uint64, 256)
	for index := range 256 {
		keys[index] = morton.Encode(uint64(index), uint64(index%256))
	}
	artifact := datura.Acquire("rpc-bench", datura.APPJSON)
	defer artifact.Release()
	artifact.WithPayload([]byte(`{"bench":true}`))
	value := artifact.Pack()

	for index := 0; b.Loop(); index++ {
		key := keys[index%256]
		_ = client.Write(ctx, func(p Server_write_Params) error {
			p.SetKey(key)
			return p.SetValue(value)
		})
	}
}

func BenchmarkForestLookup(b *testing.B) {
	ctx := context.Background()
	server := NewForestServer(WithContext(ctx))
	client := Server_ServerToClient(server)
	defer client.Release()

	morton := datura.NewMortonCoder()
	key := morton.Encode(0, uint64('H'))
	artifact := datura.Acquire("rpc-lookup", datura.APPJSON)
	defer artifact.Release()
	artifact.WithPayload([]byte(`{"letter":"H"}`))
	value := artifact.Pack()
	_ = client.Write(ctx, func(p Server_write_Params) error {
		p.SetKey(key)
		return p.SetValue(value)
	})

	for b.Loop() {
		future, release := client.Lookup(ctx, func(p Server_lookup_Params) error {
			list, _ := capnp.NewUInt64List(p.Segment(), 1)
			list.Set(0, key)
			p.SetKeys(list)
			return nil
		})
		_, _ = future.Struct()
		release()
	}
}
