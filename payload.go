package datura

import (
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

/*
PeekPayload returns a typed payload value or the type zero when absent.
*/
func PeekPayload[T any](artifact *Artifact, path string) T {
	var zero T

	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(artifact.DecryptPayload())
	}).Or(func(err error) {
		artifact.WithError(errnie.Error(errnie.Err(
			errnie.Validation,
			"payload parse",
			err,
		)))
	}).Value()

	mapped := errnie.Does(func() (map[string]any, error) {
		return root.Map()
	}).Or(func(err error) {
		artifact.WithError(errnie.Error(errnie.Err(
			errnie.Validation,
			"payload map",
			err,
		)))
	}).Value()

	if mapped == nil {
		return zero
	}

	var chunkMap any

	for _, chunk := range strings.Split(path, ".") {
		chunkMap = mapped[chunk]
	}

	return chunkMap.(T)
}
