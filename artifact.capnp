using Go = import "/go.capnp";
@0x85d3acc39d94e0f8;
$Go.package("datura");
$Go.import("github.com/theapemachine/datura");

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
            unknown              @0;
            validation           @1;
            io                   @2;
            eof                  @3;
            cancelled            @4;
            deadline             @5;
            badRequest           @6;
            unauthorized         @7;
            forbidden            @8;
            notFound             @9;
            methodNotAllowed     @10;
            notAcceptable        @11;
            timeout              @12;
            conflict             @13;
            preconditionFailed   @14;
            unsupportedMedia     @15;
            expectationFailed    @16;
            unprocessableContent @17;
            tooManyRequests      @18;
            internal             @19;
            notImplemented       @20;
            badGateway           @21;
            serviceUnavailable   @22;
        }
    }

    type @6 :Type;

    enum Type {
        artifact  @0;
        artifacts @1;
        octet     @2;
        json      @3;
        jsonl     @4;
    }

    origin      @7  :Text;
    destination @8  :Text;
    role        @9  :Text;
    scope       @10 :Text;
    attributes  @11 :Data;

    payload       @12 :Data;
    encryptedKey  @13 :Data;
    publicKey     @14 :Data;

    struct Approval {
        zkProof   @0 :Data; # Users's zero-knowledge proof
        signature @1 :Data; # Operator's blind signature approval
    }

    approvals @15 :List(Approval);
    signature @16 :Data;
}

interface Stream {
    write @0 (artifact :Artifact) -> stream;
    done  @1 ();
}