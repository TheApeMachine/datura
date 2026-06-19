package datura

import (
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func Peek[T any](artifact *Artifact, path ...any) T {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(artifact.Attributes())
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload decryption failed", err))
	}).Value()

	if !root.Exists() {
		root = errnie.Does(func() (ast.Node, error) {
			return sonic.Get(artifact.DecryptPayload())
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "payload decryption failed", err))
		}).Value()
	}

	return errnie.Does(func() (any, error) {
		return root.GetByPath(path...).Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
	}).Value().(T)
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(artifact.Attributes())
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "payload decryption failed", err))
	}).Value()

	if !root.Exists() {
		root = errnie.Does(func() (ast.Node, error) {
			return sonic.Get(artifact.DecryptPayload())
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "payload decryption failed", err))
		}).Value()
	}

	parent := root.GetByPath(path[:len(path)-1]...)

	errnie.Does(func() (bool, error) {
		return parent.SetAny(path[len(path)-1].(string), value)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute poke failed", err))
	}).Value()

	return artifact
}
