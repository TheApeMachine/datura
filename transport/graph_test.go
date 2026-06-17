package transport

import (
	"io"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewGraph(t *testing.T) {
	Convey("Given a need for a new Graph", t, func() {
		Convey("When creating a new Graph without options", func() {
			graph := NewGraph()

			Convey("Then the graph should not be nil", func() {
				So(graph, ShouldNotBeNil)
			})

			Convey("And the nodes map should be empty", func() {
				So(len(graph.nodes), ShouldEqual, 0)
			})

			Convey("And the registry should be nil", func() {
				So(graph.registry, ShouldBeNil)
			})
		})

		Convey("When creating a new Graph with a registry", func() {
			registry := newTestBuffer([]byte{})
			graph := NewGraph(WithRegistry(registry))

			Convey("Then the graph should not be nil", func() {
				So(graph, ShouldNotBeNil)
			})

			Convey("And the registry should not be nil", func() {
				So(graph.registry, ShouldNotBeNil)
			})
		})
	})
}

func TestGraphRead(t *testing.T) {
	Convey("Given a Graph instance", t, func() {
		Convey("When reading from a graph without registry", func() {
			graph := NewGraph()
			buf := make([]byte, 10)
			n, err := graph.Read(buf)

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
				So(n, ShouldEqual, 0)
			})
		})

		Convey("When reading from a graph with connected nodes", func() {
			// Create test data
			testData := []byte("test data")
			source := newTestBuffer(testData)
			target := newTestBuffer([]byte{})
			registry := newTestBuffer(testData)

			// Create nodes and connect them
			node1 := &Node{ID: "source", Component: source}
			node2 := &Node{ID: "target", Component: target}
			edge := &Edge{From: "source", To: "target"}

			graph := NewGraph(
				WithRegistry(registry),
				WithNode(node1),
				WithNode(node2),
				WithEdge(edge),
			)

			// Read from the graph
			buf := make([]byte, 100)
			n, err := graph.Read(buf)

			Convey("Then it should read the data correctly", func() {
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(testData))
				So(string(buf[:n]), ShouldEqual, string(testData))
			})

			Convey("Then the target node should have the data", func() {
				So(target.String(), ShouldEqual, string(testData))
			})
		})
	})
}

func TestGraphWrite(t *testing.T) {
	Convey("Given a Graph instance", t, func() {
		Convey("When writing to a graph without registry", func() {
			graph := NewGraph()
			n, err := graph.Write([]byte("test"))

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
				So(n, ShouldEqual, 0)
			})
		})

		Convey("When writing to a graph with registry", func() {
			registry := newTestBuffer([]byte{})
			graph := NewGraph(WithRegistry(registry))

			testData := []byte("test data")
			n, err := graph.Write(testData)

			Convey("Then it should write successfully", func() {
				So(err, ShouldBeNil)
				So(n, ShouldEqual, len(testData))
			})

			Convey("Then the registry should contain the data", func() {
				So(registry.String(), ShouldEqual, string(testData))
			})

			Convey("Then the processed flag should be reset", func() {
				So(graph.processed, ShouldBeFalse)
			})
		})
	})
}

func TestGraphClose(t *testing.T) {
	Convey("Given a Graph instance", t, func() {
		Convey("When closing a graph without registry", func() {
			graph := NewGraph()
			err := graph.Close()

			Convey("Then it should return an error", func() {
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When closing a graph with registry", func() {
			registry := newTestBuffer([]byte{})
			graph := NewGraph(WithRegistry(registry))
			err := graph.Close()

			Convey("Then it should close successfully", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestGraphGetEdges(t *testing.T) {
	Convey("Given a Graph instance with edges", t, func() {
		node1 := &Node{ID: "node1", Component: newTestBuffer([]byte{})}
		node2 := &Node{ID: "node2", Component: newTestBuffer([]byte{})}
		node3 := &Node{ID: "node3", Component: newTestBuffer([]byte{})}
		edge1 := &Edge{From: "node1", To: "node2"}
		edge2 := &Edge{From: "node1", To: "node3"}

		graph := NewGraph(
			WithRegistry(newTestBuffer([]byte{})),
			WithNode(node1),
			WithNode(node2),
			WithNode(node3),
			WithEdge(edge1),
			WithEdge(edge2),
		)

		Convey("When getting edges for a node with multiple edges", func() {
			edges := graph.GetEdges("node1")

			Convey("Then it should return all edges", func() {
				So(len(edges), ShouldEqual, 2)
				So(edges, ShouldContain, "node2")
				So(edges, ShouldContain, "node3")
			})
		})

		Convey("When getting edges for a node without edges", func() {
			edges := graph.GetEdges("node2")

			Convey("Then it should return empty slice", func() {
				So(len(edges), ShouldEqual, 0)
			})
		})

		Convey("When getting edges for a non-existent node", func() {
			edges := graph.GetEdges("nonexistent")

			Convey("Then it should return empty slice", func() {
				So(len(edges), ShouldEqual, 0)
			})
		})
	})
}

func BenchmarkGraphRead(b *testing.B) {
	testData := []byte("test data")

	b.ResetTimer()

	for b.Loop() {
		source := newTestBuffer(testData)
		target := newTestBuffer([]byte{})
		registry := newTestBuffer(testData)

		graph := NewGraph(
			WithRegistry(registry),
			WithNode(&Node{ID: "source", Component: source}),
			WithNode(&Node{ID: "target", Component: target}),
			WithEdge(&Edge{From: "source", To: "target"}),
		)

		buffer := make([]byte, len(testData))

		if _, err := graph.Read(buffer); err != nil && err != io.EOF {
			b.Fatal(err)
		}
	}
}

func BenchmarkGraphWrite(b *testing.B) {
	testData := []byte("test data")

	b.ResetTimer()

	for b.Loop() {
		registry := newTestBuffer([]byte{})
		graph := NewGraph(WithRegistry(registry))

		if _, err := graph.Write(testData); err != nil {
			b.Fatal(err)
		}
	}
}
