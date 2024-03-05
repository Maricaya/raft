package raft

import (
	"course/labgob"
	"fmt"
)

type RaftLog struct {
	snapLastIdx  int
	snapLastTerm int

	// contains index [1, snapLastIdx]
	snapshot []byte
	// the first entry is `snapLastIdx`, but only contains the snapLastTerm
	// the entries between (snapLastIdx, snapLastIdx+len(tailLog)-1] have real data
	tailLog []LogEntry
}

// called as : rf.log = NewLog(InvalidIndex, InvalidTerm, nil, nil)
func NewLog(snapLastIdx, snapLastTerm int, snapshot []byte, entries []LogEntry) *RaftLog {
	rl := &RaftLog{
		snapLastIdx:  snapLastIdx,
		snapLastTerm: snapLastTerm,
		snapshot:     snapshot,
	}

	// make the len = 0, cap = 1 + len(entries)
	rl.tailLog = make([]LogEntry, 0, 1+len(entries))
	rl.tailLog = append(rl.tailLog, LogEntry{
		Term: snapLastTerm,
	})
	rl.tailLog = append(rl.tailLog, entries...)

	return rl
}

// return detailed error for the caller to log
func (rl *RaftLog) readPersist(d *labgob.LabDecoder) error {
	var lastIdx int
	if err := d.Decode(&lastIdx); err != nil {
		return fmt.Errorf("decode last include index failed")
	}
	rl.snapLastIdx = lastIdx

	var lastTerm int
	if err := d.Decode(&lastTerm); err != nil {
		return fmt.Errorf("decode last include term failed")
	}
	rl.snapLastTerm = lastTerm

	var log []LogEntry
	if err := d.Decode(&log); err != nil {
		return fmt.Errorf("decode tail log failed")
	}
	rl.tailLog = log

	return nil
}

func (rl *RaftLog) persist(e *labgob.LabEncoder) {
	e.Encode(rl.snapLastIdx)
	e.Encode(rl.snapLastTerm)
	e.Encode(rl.tailLog)
}

// the dummy log is counted
func (rl *RaftLog) size() int {
	return rl.snapLastIdx + len(rl.tailLog)
}

// index conversion
// access the index `rl.snapLastIdx` is allowed, although it's not exist, actually.
func (rl *RaftLog) idx(logicIdx int) int {
	if logicIdx < rl.snapLastIdx || logicIdx >= rl.size() {
		panic(fmt.Sprintf("%d is out of [%d, %d]", logicIdx, rl.snapLastIdx+1, rl.size()-1))
	}
	return logicIdx - rl.snapLastIdx
}

func (rl *RaftLog) at(logicIdx int) LogEntry {
	return rl.tailLog[rl.idx(logicIdx)]
}

// more detailed
func (rl *RaftLog) String() string {
	var terms string
	prevTerm := rl.snapLastTerm
	prevStart := rl.snapLastIdx
	for i := 0; i < len(rl.tailLog); i++ {
		if rl.tailLog[i].Term != prevTerm {
			terms += fmt.Sprintf(" [%d, %d]T%d", prevStart, rl.snapLastIdx+i-1, prevTerm)
			prevTerm = rl.tailLog[i].Term
			prevStart = i
		}
	}
	terms += fmt.Sprintf("[%d, %d]T%d", prevStart, rl.snapLastIdx+len(rl.tailLog)-1, prevTerm)
	return terms
}

// more simplified
func (rl *RaftLog) Str() string {
	lastIdx, lastTerm := rl.last()
	return fmt.Sprintf("[%d]T%d~[%d]T%d", rl.snapLastIdx, rl.snapLastTerm, lastIdx, lastTerm)
}

func (rl *RaftLog) last() (idx, term int) {

	return rl.size() - 1, rl.tailLog[len(rl.tailLog)-1].Term
}

func (rl *RaftLog) tail(startIdx int) []LogEntry {
	if startIdx >= rl.size() {
		return nil
	}
	return rl.tailLog[rl.idx(startIdx):]
}

func (rl *RaftLog) firstLogFor(term int) int {
	for idx, entry := range rl.tailLog {
		if entry.Term == term {
			return idx
		} else if entry.Term > term {
			break
		}
	}
	return InvalidIndex
}

func (rl *RaftLog) append(e LogEntry) {
	rl.tailLog = append(rl.tailLog, e)
}

func (rl *RaftLog) appendFrom(prevIdx int, entries []LogEntry) {
	rl.tailLog = append(rl.tailLog[:rl.idx(prevIdx)+1], entries...)
}

func (rl *RaftLog) doSnapshot(index int, snapshot []byte) {
	if index <= rl.snapLastIdx {
		return
	}
	// since idx() will use rl.snapLastIdx, so we should keep it first
	idx := rl.idx(index)

	rl.snapLastTerm = rl.tailLog[idx].Term
	rl.snapLastIdx = index
	rl.snapshot = snapshot

	// allocate a new slice
	newLog := make([]LogEntry, 0, rl.size()-rl.snapLastIdx)
	newLog = append(newLog, LogEntry{
		Term: rl.snapLastTerm,
	})
	newLog = append(newLog, rl.tailLog[idx+1:]...)

	rl.tailLog = newLog
}

// --- raft_log.go
// isntall snapshot from the leader to the follower
func (rl *RaftLog) installSnapshot(index, term int, snapshot []byte) {
	rl.snapLastIdx = index
	rl.snapLastTerm = term
	rl.snapshot = snapshot

	// make a new log array
	// just discard all the local log, and use the leader's snapshot
	newLog := make([]LogEntry, 0, 1)
	newLog = append(newLog, LogEntry{
		Term: rl.snapLastTerm,
	})
	rl.tailLog = newLog
}
