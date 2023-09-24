package main

type EntryUpdate struct {
	UpdateID uint32
	Expiry   int
	KeyType  int
	KeyValue interface{}
	Values   []int
}

type Entry struct {
	Key    interface{}
	Values []int
}

type TableValue interface{}
type TableKey interface{}

type TableDefinition struct {
	StickTableID int
	Name         string
	KeyType      int
	KeyLen       int
	DataType     int
	Expiry       int
	Frequency    [][]int
}
type TableKeyType int
type DataType int

type Table struct {
	localUpdateId uint32
	definition    TableDefinition
	entries       map[string]Entry
}
