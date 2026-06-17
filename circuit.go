package datura

import (
	"bytes"

	"github.com/consensys/gnark-crypto/ecc/bn254"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// AuthCircuit defines the zk-SNARK proof structure
// This proves an employee has access without revealing identity

type AuthCircuit struct {
	PseudonymHash frontend.Variable `gnark:",private"` // Employee's private credential
	MerkleRoot    frontend.Variable `gnark:",public"`  // Must match the stored Merkle root
}

// Define implements the frontend.Circuit interface.
// In a real circuit, constraints would be defined here.
func (c *AuthCircuit) Define(api frontend.API) error {
	// For example, one might enforce a relationship between the pseudonym hash and the Merkle root.
	// Here we simply return nil as a placeholder.
	return nil
}

// GenerateProof creates a zk-SNARK proof for an artifact
func GenerateProof(artifact Artifact, pk groth16.ProvingKey) ([]byte, error) {
	// Extract necessary fields from the Cap'n Proto type
	pseudonymHash, err := artifact.PseudonymHash()

	if err != nil {
		return nil, err
	}

	merkleRoot, err := artifact.MerkleRoot()

	if err != nil {
		return nil, err
	}

	var circuit AuthCircuit

	// Use the BN254 curve's field modulus
	modulus := fr.Modulus()
	compiled, err := frontend.Compile(modulus, r1cs.NewBuilder, &circuit)

	if err != nil {
		return nil, err
	}

	// Create witness
	witness, err := frontend.NewWitness(&AuthCircuit{
		PseudonymHash: pseudonymHash,
		MerkleRoot:    merkleRoot,
	}, modulus)

	if err != nil {
		return nil, err
	}

	// Create proof using provided proving key
	proof, err := groth16.Prove(compiled, pk, witness)
	if err != nil {
		return nil, err
	}

	// Serialize proof to bytes using the WriteTo method.
	var buf bytes.Buffer
	_, err = proof.WriteTo(&buf)

	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// VerifyProof checks if a given zk-SNARK proof is valid
func VerifyProof(artifact Artifact, proofBytes []byte, vk groth16.VerifyingKey) bool {
	// Extract the public Merkle root from the artifact.
	merkleRoot, err := artifact.MerkleRoot()

	if err != nil {
		return false
	}

	// Deserialize proof from bytes via the ReadFrom method.
	proof := groth16.NewProof(bn254.ID)
	buf := bytes.NewBuffer(proofBytes)

	_, err = proof.ReadFrom(buf)

	if err != nil {
		return false
	}

	// Create a public witness with only the public inputs.
	modulus := fr.Modulus()

	publicWitness, err := frontend.NewWitness(&AuthCircuit{
		MerkleRoot: merkleRoot,
	}, modulus, frontend.PublicOnly())

	if err != nil {
		return false
	}

	// Verify the proof.
	return groth16.Verify(proof, vk, publicWitness) == nil
}
