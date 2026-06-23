using Go = import "/go.capnp";
@0x85d3acc39d94e0f8;
$Go.package("datura");
$Go.import("github.com/theapemachine/datura");

struct Node {
    id      @0 :Data;
    address @1 :Text;
    latency @2 :UInt32;
}

struct Artifact {
    uuid       @0 :Data;
    checksum   @1 :Data;
    timestamp  @2 :Int64;
    error      @3 :Error;
    pseudonym  @4 :Data;  # zk-SNARK identity hash
    merkleRoot @5 :Data;  # Root of the Merkle Tree

    struct Error {
        type      @0 :Type;
        timestamp @1 :Int64;
        message   @2 :Text;

        enum Type {
            unknown    @0;
            validation @1;
        }
    }

    type @6 :Type;

    enum Type {
        json      @0;
        jsonl     @1;
        artifact  @2;
        artifacts @3;
    }

    origin      @7  :Text;
    destination @8  :Text;
    role        @9  :Text;
    scope       @10 :Text;
    attributes  @11 :Data;

    payload       @12 :List(Data);
    encryptedKey  @13 :Data;
    publicKey     @14 :Data;

    struct Approval {
        zkProof   @0 :Data;  # Users's zero-knowledge proof
        signature @1 :Data;  # Operator's blind signature approval
    }

    approvals @15 :List(Approval);
    signature @16 :Data;
}

interface Compute {
    ping  @0 (challenger :Node) -> (responder :Node);
    write @1 (artifact :Artifact) -> stream;
    done  @2 ();
}