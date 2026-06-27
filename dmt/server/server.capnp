@0xad058c9d70413d66;

using Go = import "/go.capnp";
$Go.package("server");
$Go.import("github.com/theapemachine/datura/dmt/server");

using import "../../artifact.capnp".Artifact;

interface Server {
  write          @0 (key :UInt64, value :Data) -> stream;
  done           @1 ();
  lookup         @2 (keys :List(UInt64)) -> (values :List(Artifact));
}
