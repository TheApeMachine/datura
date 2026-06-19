package datura

import (
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func attributeBytes(artifact *Artifact) []byte {
	return errnie.Does(func() ([]byte, error) {
		return artifact.Attributes()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes unavailable", err))
	}).Value()
}

func attributeRoot(artifact *Artifact) ast.Node {
	encoded := attributeBytes(artifact)

	if len(encoded) > 0 {
		root := errnie.Does(func() (ast.Node, error) {
			return sonic.Get(encoded)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attributes parse failed", err))
		}).Value()

		if root.Exists() {
			return root
		}
	}

	payload, err := artifact.decryptPayload()

	if err != nil || len(payload) == 0 {
		return ast.Node{}
	}

	root, err := sonic.Get(payload)

	if err != nil || !root.Exists() {
		if err != nil {
			errnie.Error(errnie.Err(errnie.Validation, "payload parse failed", err))
		}

		return ast.Node{}
	}

	return root
}

func mutableAttributeRoot(artifact *Artifact) ast.Node {
	encoded := attributeBytes(artifact)

	if len(encoded) > 0 {
		root := errnie.Does(func() (ast.Node, error) {
			return sonic.Get(encoded)
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attributes parse failed", err))
		}).Value()

		if root.Exists() {
			return root
		}
	}

	return ast.NewObject(nil)
}

func Peek[T any](artifact *Artifact, path ...any) T {
	var zero T

	root := attributeRoot(artifact)

	if !root.Exists() {
		return zero
	}

	node := root.GetByPath(path...)

	if !node.Exists() {
		return zero
	}

	value, err := node.Interface()

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))

		return zero
	}

	typed, ok := value.(T)

	if !ok {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek type mismatch", nil))

		return zero
	}

	return typed
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	root := mutableAttributeRoot(artifact)

	errnie.Does(func() (bool, error) {
		return (&root).SetAnyByPath(value, path...)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute poke failed", err))
	}).Value()

	encoded := errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	errnie.Error(artifact.SetAttributes(encoded))

	return artifact
}
