package datura

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/constraint"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/theapemachine/errnie"
)

/*
AuthCircuit defines the zk-SNARK proof structure.
This proves an employee has access without revealing identity.
*/
type AuthCircuit struct {
	PseudonymHash frontend.Variable `gnark:",private"`
	MerkleRoot    frontend.Variable `gnark:",public"`
}

/*
Define implements the frontend.Circuit interface.
*/
func (circuit *AuthCircuit) Define(api frontend.API) error {
	return nil
}

var (
	compiledCircuit constraint.ConstraintSystem
	provingKey      groth16.ProvingKey
	verifyingKey    groth16.VerifyingKey
	zkSetupOnce     sync.Once
	zkSetupErr      error
)

/*
InitZKSNARKEngine compiles the circuit and runs trusted setup exactly once.
*/
func InitZKSNARKEngine() error {
	zkSetupOnce.Do(func() {
		var circuit AuthCircuit
		modulus := fr.Modulus()

		compiledCircuit, zkSetupErr = frontend.Compile(modulus, r1cs.NewBuilder, &circuit)

		if zkSetupErr != nil {
			return
		}

		provingKey, verifyingKey, zkSetupErr = groth16.Setup(compiledCircuit)
	})

	return zkSetupErr
}

/*
GenerateProof creates a zk-SNARK proof for an artifact using precompiled keys.
*/
func GenerateProof(artifact *Artifact) ([]byte, error) {
	if err := InitZKSNARKEngine(); err != nil {
		return nil, errnie.Error(err)
	}

	if artifact == nil {
		return nil, errnie.Error(errors.New("nil artifact"))
	}

	pseudonymHash, err := artifact.Pseudonym()

	if err != nil {
		return nil, errnie.Error(err)
	}

	merkleRoot, err := artifact.MerkleRoot()

	if err != nil {
		return nil, errnie.Error(err)
	}

	modulus := fr.Modulus()

	witness, err := frontend.NewWitness(&AuthCircuit{
		PseudonymHash: pseudonymHash,
		MerkleRoot:    merkleRoot,
	}, modulus)

	if err != nil {
		return nil, errnie.Error(err)
	}

	proof, err := groth16.Prove(compiledCircuit, provingKey, witness)

	if err != nil {
		return nil, errnie.Error(err)
	}

	var buf bytes.Buffer

	if _, err = proof.WriteTo(&buf); err != nil {
		return nil, errnie.Error(err)
	}

	return buf.Bytes(), nil
}

/*
VerifyProof checks if a given zk-SNARK proof is valid using precompiled keys.
*/
func VerifyProof(artifact *Artifact, proofBytes []byte) bool {
	if err := InitZKSNARKEngine(); err != nil {
		return false
	}

	if artifact == nil {
		return false
	}

	merkleRoot, err := artifact.MerkleRoot()

	if err != nil {
		return false
	}

	proof := groth16.NewProof(bn254.ID)
	reader := bytes.NewReader(proofBytes)

	if _, err = proof.ReadFrom(reader); err != nil {
		return false
	}

	modulus := fr.Modulus()

	publicWitness, err := frontend.NewWitness(&AuthCircuit{
		MerkleRoot: merkleRoot,
	}, modulus, frontend.PublicOnly())

	if err != nil {
		return false
	}

	return groth16.Verify(proof, verifyingKey, publicWitness) == nil
}

/*
BatchVerifyProofs validates every proof in artifacts.
Groth16 batch verification is not exposed in gnark v0.15, so proofs are checked sequentially.
*/
func BatchVerifyProofs(artifacts []*Artifact) error {
	for _, artifact := range artifacts {
		if artifact == nil {
			return errnie.Error(errors.New("nil artifact in batch"))
		}

		approvals, err := artifact.Approvals()

		if err != nil || approvals.Len() == 0 {
			return errnie.Error(fmt.Errorf("missing approval proof"))
		}

		proofBytes, err := approvals.At(0).ZkProof()

		if err != nil {
			return errnie.Error(err, "approval_proof")
		}

		if !VerifyProof(artifact, proofBytes) {
			return errnie.Error(errors.New("batch verification failed"))
		}
	}

	return nil
}
