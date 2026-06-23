package datura

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

func Peek[T any](artifact *Artifact, path ...any) T {
	var (
		zero T
		root ast.Node
		ok   bool
	)

	for _, region := range []func() ([]byte, error){
		artifact.Attributes, artifact.decryptPayload,
	} {
		root = errnie.Does(func() (ast.Node, error) {
			return sonic.Get(errnie.Does(func() ([]byte, error) {
				return region()
			}).Or(func(err error) {
				if strings.Contains(err.Error(), "value not exists") {
					return
				}

				errnie.Error(errnie.Err(
					errnie.Validation, err.Error(), err,
				).With(artifact.Log()...))
			}).Value(), path...)
		}).Or(func(err error) {
			if strings.Contains(err.Error(), "value not exists") {
				return
			}

			errnie.Error(errnie.Err(
				errnie.Validation, err.Error(), err,
			).With(artifact.Log()...))
		}).Value()

		if root.Exists() {
			break
		}
	}

	if !root.Exists() {
		return zero
	}

	if zero, ok = errnie.Does(func() (any, error) {
		return root.Interface()
	}).Or(func(err error) {
		if strings.Contains(err.Error(), "value not exists") {
			return
		}

		errnie.Error(errnie.Err(errnie.Validation, err.Error(), err))
	}).Value().(T); !ok {
		return zero
	}

	return zero
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(errnie.Does(func() ([]byte, error) {
			return artifact.Attributes()
		}).Or(func(err error) {
			errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
		}).Value())
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
	}).Value()

	if !root.Exists() {
		root = ast.NewObject(nil)
	}

	root.SetAnyByPath(value, path...)

	errnie.Error(artifact.SetAttributes(errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()))

	return artifact
}