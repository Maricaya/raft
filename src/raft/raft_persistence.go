package raft

import (
	"bytes"
	"course/labgob"
	"fmt"
)

func (rf *Raft) persistString() string {
	return fmt.Sprintf("T%d, VotedFor: %d, Log: [0: %d)", rf.currentTerm, rf.votedFor, rf.log.size())
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persistLocked() {
	// Your code here (PartC).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftState := w.Bytes()
	// rf.persister.Save(raftState, nil)

	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	rf.log.persist(e)
	raftState := w.Bytes()
	// leave the second parameter nil, will use
	rf.persister.Save(raftState, rf.log.snapshot)
	LOG(rf.me, rf.currentTerm, DPersist, "Persist: %v", rf.persistString())
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (PartC).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
	r := bytes.NewReader(data)
	d := labgob.NewDecoder(r)

	var currentTerm int

	if err := d.Decode(&currentTerm); err != nil {
		LOG(rf.me, rf.currentTerm, DPersist, "Read currentTerm error: %v", err)
		return
	}
	rf.currentTerm = currentTerm

	var votedFor int

	if err := d.Decode(&votedFor); err != nil {
		LOG(rf.me, rf.currentTerm, DPersist, "Read votedFor error: %v", err)
		return
	}

	rf.votedFor = votedFor

	if err := rf.log.readPersist(d); err != nil {
		LOG(rf.me, rf.currentTerm, DPersist, "Read log error: %v", err)
		return
	}
	rf.log.snapshot = rf.persister.ReadSnapshot()

	if rf.log.snapLastIdx > rf.commitIndex {
		rf.commitIndex = rf.log.snapLastIdx
		rf.lastApplied = rf.log.snapLastIdx
	}
	LOG(rf.me, rf.currentTerm, DPersist, "Read Persist: %v", rf.persistString())
}
