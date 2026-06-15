using Go = import "/go.capnp";
@0x85d3acc39d94e0f8;
$Go.package("datura");
$Go.import("github.com/theapemachine/datura");

struct Artifact {
    uuid      @0 :Text;
    timestamp @1 :Int64;
    error     @2 :Error;

    struct Error {
        type      @0 :Type;
        timestamp @1 :Int64;
        message   @2 :Text;

        enum Type {
            unknown    @0;
            validation @1;
        }
    }

    type @3 :Type;

    enum Type {
        json @0;
    }

    origin      @4 :Text;
    destination @5 :Text;
    role        @6 :Text;
    scope       @7 :Text;
    attributes  @8 :List(Attribute);

    struct Attribute {
        key @0 :Text;
        value @1 :Text;
    }

    payload     @9 :Data;
}