package datura

import (
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
Peek returns a typed attributes value or the type zero when absent.
*/
func Peek[T any](artifact *Artifact, path ...any) T {
	payload := errnie.Does(func() ([]byte, error) {
		return artifact.Attributes()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(payload, path...)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	out := errnie.Does(func() (any, error) {
		return root.Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	return out.(T)
}

func (artifact *Artifact) Poke(value any, path ...any) *Artifact {
	attributes := errnie.Does(func() ([]byte, error) {
		return artifact.Attributes()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	if len(attributes) == 0 {
		attributes = []byte("{}")
	}

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(attributes)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "attributes unavailable", err))
	}).Value()

	errnie.Does(func() (bool, error) {
		return root.GetByPath(path[:len(path)-1]...).SetAny(path[len(path)-1].(string), value)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "attributes unavailable", err))
	})

	encoded := errnie.Does(func() ([]byte, error) {
		return root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "attributes unavailable", err))
	}).Value()

	errnie.Error(artifact.SetAttributes(encoded))
	return artifact
}
