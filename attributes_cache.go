package datura

import (
	"sync"
	"unsafe"

	"github.com/bytedance/sonic"
	"github.com/bytedance/sonic/ast"
	"github.com/theapemachine/errnie"
)

type attributesCacheEntry struct {
	root  ast.Node
	dirty bool
}

var attributesCaches sync.Map

func artifactCacheKey(artifact *Artifact) uintptr {
	if artifact == nil {
		return 0
	}

	return uintptr(unsafe.Pointer(artifact))
}

func invalidateAttributesCache(artifact *Artifact) {
	attributesCaches.Delete(artifactCacheKey(artifact))
}

func replaceAttributesCache(artifact *Artifact, encoded []byte) {
	root := parseAttributesRoot(encoded)

	attributesCaches.Store(
		artifactCacheKey(artifact),
		&attributesCacheEntry{root: root, dirty: false},
	)
}

func initAttributesCache(artifact *Artifact) {
	replaceAttributesCache(artifact, []byte("{}"))
}

func attributesCacheEntryFor(artifact *Artifact) *attributesCacheEntry {
	key := artifactCacheKey(artifact)

	if cached, ok := attributesCaches.Load(key); ok {
		return cached.(*attributesCacheEntry)
	}

	encoded, err := capnpArtifact(artifact).Attributes()

	if err != nil || len(encoded) == 0 {
		encoded = []byte("{}")
	}

	entry := &attributesCacheEntry{
		root:  parseAttributesRoot(encoded),
		dirty: false,
	}

	attributesCaches.Store(key, entry)

	return entry
}

func (artifact *Artifact) attributesRoot() *ast.Node {
	entry := attributesCacheEntryFor(artifact)

	return &entry.root
}

func (artifact *Artifact) flushAttributes() {
	entry := attributesCacheEntryFor(artifact)

	if !entry.dirty {
		return
	}

	encoded := errnie.Does(func() ([]byte, error) {
		return entry.root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	errnie.Error(capnpArtifact(artifact).SetAttributes(encoded))

	entry.dirty = false
}

func parseAttributesRoot(encoded []byte) ast.Node {
	root := errnie.Does(func() (ast.Node, error) {
		return sonic.Get(encoded)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attribute peek failed", err))
	}).Value()

	if !root.Exists() {
		return ast.NewObject(nil)
	}

	return root
}

func rematerializeAttributes(entry *attributesCacheEntry) {
	encoded := errnie.Does(func() ([]byte, error) {
		return entry.root.MarshalJSON()
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.Validation, "attributes marshal failed", err))
	}).Value()

	entry.root = parseAttributesRoot(encoded)
}

func capnpArtifact(artifact *Artifact) Artifact {
	return *artifact
}
