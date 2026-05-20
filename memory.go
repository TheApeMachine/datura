package datura

/*
Memory is an implementation of agentic memory, that acts as a
unified interface for all data sources.
*/
type Memory struct {
	stores []Store
}