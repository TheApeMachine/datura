package datura

import (
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
)

/*
PeekPayloadOK returns a typed value from the decrypted JSON payload at path.
Path segments use dot notation with numeric indices for arrays, e.g. "0.price".
*/
func PeekPayloadOK[T any](artifact *Artifact, path string) (T, bool) {
	var zero T

	if artifact == nil || path == "" {
		return zero, false
	}

	node, nodeOK := payloadNodeAt(artifact, path)

	if !nodeOK || node == nil {
		return zero, false
	}

	return payloadNodeAs[T](*node)
}

/*
PeekPayload returns a typed payload value or the type zero when absent.
*/
func PeekPayload[T any](artifact *Artifact, path string) T {
	value, _ := PeekPayloadOK[T](artifact, path)

	return value
}

/*
PayloadLen returns the element count when the payload root is a JSON array.
*/
func PayloadLen(artifact *Artifact) (int, bool) {
	root, rootOK := ensurePayloadRoot(artifact)

	if !rootOK {
		return 0, false
	}

	elements, err := root.ArrayUseNode()

	if err != nil {
		return 0, false
	}

	return len(elements), true
}

/*
PayloadEach visits each element of a JSON array payload without full unmarshaling.
Return false from visit to stop iteration.
*/
func PayloadEach(artifact *Artifact, visit func(index int, element ast.Node) bool) bool {
	if visit == nil {
		return false
	}

	root, rootOK := ensurePayloadRoot(artifact)

	if !rootOK {
		return false
	}

	elements, err := root.ArrayUseNode()

	if err != nil {
		return false
	}

	for index, element := range elements {
		if !visit(index, element) {
			return false
		}
	}

	return true
}

func invalidatePayloadCache(artifact *Artifact) {
	if artifact == nil {
		return
	}

	state := artifactStreamStateFor(artifact)
	state.payloadBytes = nil
	state.payloadRoot = ast.Node{}
	state.payloadParsed = false
}

func ensurePayloadRoot(artifact *Artifact) (ast.Node, bool) {
	if artifact == nil {
		return ast.Node{}, false
	}

	payload, err := artifact.DecryptPayload()

	if err != nil || len(payload) == 0 {
		return ast.Node{}, false
	}

	state := artifactStreamStateFor(artifact)

	if state.payloadParsed && bytesEqual(state.payloadBytes, payload) {
		return state.payloadRoot, true
	}

	root, getErr := sonic.Get(payload)

	if getErr != nil {
		return ast.Node{}, false
	}

	state.payloadBytes = append([]byte(nil), payload...)
	state.payloadRoot = root
	state.payloadParsed = true

	return root, true
}

func payloadNodeAt(artifact *Artifact, path string) (*ast.Node, bool) {
	root, rootOK := ensurePayloadRoot(artifact)

	if !rootOK {
		return nil, false
	}

	segments := strings.Split(path, ".")
	current := &root

	for _, segment := range segments {
		if segment == "" {
			return nil, false
		}

		if index, parseErr := strconv.Atoi(segment); parseErr == nil {
			current = current.Index(index)

			if current == nil {
				return nil, false
			}

			continue
		}

		next := current.Get(segment)

		if next == nil {
			return nil, false
		}

		current = next
	}

	return current, true
}

func payloadNodeAs[T any](node ast.Node) (T, bool) {
	var zero T

	switch any(zero).(type) {
	case string:
		value, err := node.String()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case float64:
		value, err := node.Float64()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case int:
		value, err := node.Int64()

		if err != nil {
			return zero, false
		}

		return any(int(value)).(T), true
	case int64:
		value, err := node.Int64()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	case bool:
		value, err := node.Bool()

		if err != nil {
			return zero, false
		}

		return any(value).(T), true
	default:
		return zero, false
	}
}

func bytesEqual(left, right []byte) bool {
	if len(left) != len(right) {
		return false
	}

	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}

	return true
}
