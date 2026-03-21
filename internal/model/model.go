package model

import "time"

type Commit struct {
	Hash      string
	Author    string
	Email     string
	Message   string
	Timestamp time.Time
	Parents   []string
}

type FileChange struct {
	Status  string
	Path    string
	OldPath string
}

type Symbol struct {
	ID       string
	Name     string
	Kind     string
	FilePath string
	Language string
	Line     int
}

type CallEdge struct {
	Caller   string
	Callee   string
	Line     int
	Args     []string
	FullName string
}

type FileGraph struct {
	Path    string
	Lang    string
	Symbols []Symbol
	Calls   []CallEdge
	Imports []string
}

type CommitGraph struct {
	CommitHash string
	Files      map[string]FileGraph
}
