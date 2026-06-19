package datura

import "github.com/bytedance/sonic"

type Map[T any] map[string]T

func (m Map[T]) Marshal() []byte {
	payload, err := sonic.Marshal(m)

	if err != nil {
		return nil
	}

	return payload
}
