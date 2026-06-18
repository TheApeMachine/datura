package datura

import (
	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
PeekPayload returns a typed payload value or the type zero when absent.
*/
func PeekPayload[T any](artifact *Artifact, path ...any) T {
	payload := errnie.Does(func() ([]byte, error) {
		return artifact.DecryptPayload()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload unavailable", err))
	}).Value()

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(payload, path...)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload unavailable", err))
	}).Value()

	out := errnie.Does(func() (any, error) {
		return root.Interface()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.NotFound, "payload unavailable", err))
	}).Value()

	return out.(T)
}
