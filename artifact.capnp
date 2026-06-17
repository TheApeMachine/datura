using Go = import "/go.capnp";
@0x85d3acc39d94e0f8;
$Go.package("datura");
$Go.import("github.com/theapemachine/datura");

struct Artifact {
    uuid          @0 :Data;
    checksum      @1 :Data;
    timestamp     @2 :Int64;
    error         @3 :Error;
    pseudonymHash @4 :Data;  # zk-SNARK identity hash
    merkleRoot    @5 :Data;  # Root of the Merkle Tree

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
        artifact  @1;
        artifacts @2;
    }

    origin      @7  :Text;
    destination @8  :Text;
    role        @9  :Text;
    scope       @10 :Text;
    attributes  @11 :List(Attribute);

    struct Attribute {
        key @0 :Text;
        value :union {
            textValue   @1 :Text;
            intValue    @2 :Int64;
            floatValue  @3 :Float64;
            boolValue   @4 :Bool;
            binaryValue @5 :Data;
        }
    }

    encryptedPayload   @12 :Data;
    encryptedKey       @13 :Data;
    ephemeralPublicKey @14 :Data;

    struct Approval {
        zkProof                @0 :Data;  # Users's zero-knowledge proof
        operatorBlindSignature @1 :Data;  # Operator's blind signature approval
    }

    approvals @15 :List(Approval);
    signature @16 :Data;
}