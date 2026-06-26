package datura

import (
	"context"
	"errors"
	"io"

	capnp "capnproto.org/go/capnp/v3"
	"capnproto.org/go/capnp/v3/flowcontrol"
	"capnproto.org/go/capnp/v3/rpc"
)

const artifactStreamFlowLimit = 1 << 16

/*
ArtifactStream implements the generated Cap'n Proto Stream server.
*/
type ArtifactStream struct {
	write func(context.Context, *Artifact) error
	done  func(context.Context) error
}

/*
NewArtifactStream creates a Stream server from write and done callbacks.
*/
func NewArtifactStream(
	write func(context.Context, *Artifact) error,
	done func(context.Context) error,
) *ArtifactStream {
	return &ArtifactStream{
		write: write,
		done:  done,
	}
}

/*
Write handles a streamed artifact RPC call.
*/
func (artifactStream *ArtifactStream) Write(
	ctx context.Context,
	call Stream_write,
) error {
	if artifactStream == nil || artifactStream.write == nil {
		return errors.New("datura: artifact stream write handler is nil")
	}

	inbound, err := call.Args().Artifact()

	if err != nil {
		return err
	}

	artifact, err := (&inbound).Clone()

	if err != nil {
		return err
	}

	return artifactStream.write(ctx, artifact)
}

/*
Done handles the stream completion RPC call.
*/
func (artifactStream *ArtifactStream) Done(
	ctx context.Context,
	_ Stream_done,
) error {
	if artifactStream == nil || artifactStream.done == nil {
		return nil
	}

	return artifactStream.done(ctx)
}

/*
NewArtifactStreamConnection exposes an ArtifactStream server over rwc.
*/
func NewArtifactStreamConnection(
	rwc io.ReadWriteCloser,
	artifactStream *ArtifactStream,
) *rpc.Conn {
	client := Stream_ServerToClient(artifactStream)

	return rpc.NewConn(rpc.NewStreamTransport(rwc), &rpc.Options{
		BootstrapClient: capnp.Client(client),
	})
}

/*
ArtifactStreamClient sends artifacts through the generated Stream capability.
*/
type ArtifactStreamClient struct {
	Stream
}

/*
NewArtifactStreamClient bootstraps a Stream capability from rwc.
*/
func NewArtifactStreamClient(
	ctx context.Context,
	rwc io.ReadWriteCloser,
) (*ArtifactStreamClient, *rpc.Conn) {
	conn := rpc.NewConn(rpc.NewStreamTransport(rwc), nil)

	return &ArtifactStreamClient{
		Stream: Stream(conn.Bootstrap(ctx)),
	}, conn
}

/*
Send writes artifacts through Cap'n Proto streaming RPC and waits for completion.
*/
func (artifactStreamClient *ArtifactStreamClient) Send(
	ctx context.Context,
	artifacts ...*Artifact,
) error {
	if artifactStreamClient == nil || !artifactStreamClient.Stream.IsValid() {
		return errors.New("datura: artifact stream client is invalid")
	}

	artifactStreamClient.Stream.SetFlowLimiter(
		flowcontrol.NewFixedLimiter(artifactStreamFlowLimit),
	)

	for _, artifact := range artifacts {
		if artifact == nil || !artifact.IsValid() {
			return errors.New("datura: artifact stream send got invalid artifact")
		}

		inbound := *artifact
		err := artifactStreamClient.Stream.Write(ctx, func(params Stream_write_Params) error {
			return params.SetArtifact(inbound)
		})

		if err != nil {
			return err
		}
	}

	future, release := artifactStreamClient.Stream.Done(ctx, nil)
	defer release()

	if _, err := future.Struct(); err != nil {
		return err
	}

	return artifactStreamClient.Stream.WaitStreaming()
}
