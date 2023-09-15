package raft

import (
	"io/fs"
	"strconv"
	"strings"
)

// A wrapper to allow it to be sorted easier
type DirEntries []fs.DirEntry

func (entries DirEntries) Len() int {
	return len(entries)
}

func (entries DirEntries) Less(i, j int) bool {
	nameI := entries[i].Name()
	nameJ := entries[j].Name()

	partsI := strings.Split(nameI, "-")
	partsJ := strings.Split(nameJ, "-")

	if len(partsI) != 2 || len(partsJ) != 2 {
		return nameI < nameJ
	}

	secondNumberI, errI := strconv.Atoi(partsI[1])
	secondNumberJ, errJ := strconv.Atoi(partsJ[1])

	if errI != nil || errJ != nil {
		return nameI < nameJ
	}

	return secondNumberI < secondNumberJ
}

func (entries DirEntries) Swap(i, j int) {
	entries[i], entries[j] = entries[j], entries[i]
}
