package raft

import (
	"fmt"
	"sort"
	"time"
)

type LogEntry struct {
	Term         int         // the log entry's term
	CommandValid bool        // if it should be applied
	Command      interface{} // the command should be applied to the state machine
}

type AppendEntriesArgs struct {
	Term     int
	LeaderId int

	// used to probe the match point
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry

	// used to update the follower's commitIndex
	LeaderCommit int
}

func (args *AppendEntriesArgs) String() string {
	return fmt.Sprintf("Leader-%d, T%d, Prev:[%d]T%d, (%d, %d], CommitIdx: %d",
		args.LeaderId, args.Term, args.PrevLogIndex, args.PrevLogTerm,
		args.PrevLogIndex, args.PrevLogIndex+len(args.Entries), args.LeaderCommit)
}

type AppendEntriesReply struct {
	Term    int
	Success bool

	ConflictIndex int
	ConflictTerm  int
}

func (reply *AppendEntriesReply) String() string {
	return fmt.Sprintf("T%d, Sucess: %v, ConflictTerm: [%d]T%d", reply.Term, reply.Success, reply.ConflictIndex, reply.ConflictTerm)
}

// 辅助函数，返回两个数中的最小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Peer's callback
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	LOG(rf.me, rf.currentTerm, DDebug, "<- S%d, Appended, Args=%v", args.LeaderId, args.String())
	reply.Term = rf.currentTerm
	reply.Success = false

	// After we align the term, we accept the args.LeaderId as Leader,
	// then we must reset election timer whether we
	// accept the log or not

	// align the term
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject log, Higher term, T%d<T%d", args.LeaderId, args.Term, rf.currentTerm)
		return
	}

	if args.Term >= rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
	}

	defer func() {
		rf.resetElectionTimerLocked()
		if !reply.Success {
			LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Follower Conflict: [%d]T%d", args.LeaderId, reply.ConflictIndex, reply.ConflictTerm)

			LOG(rf.me, rf.currentTerm, DDebug, "<- S%d, Follower log=%v", args.LeaderId, rf.log.String())
		}
	}()

	// return failure if prevLog not matched
	if args.PrevLogIndex >= rf.log.size() {
		reply.ConflictIndex = rf.log.size()
		reply.ConflictTerm = InvalidTerm

		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject log, Follower log too short, Len: %d < Prev: %d", args.LeaderId, rf.log.size(), args.PrevLogIndex)
		return
	}
	if args.PrevLogIndex < rf.log.snapLastIdx {
		reply.ConflictTerm = rf.log.snapLastTerm
		reply.ConflictIndex = rf.log.snapLastIdx
		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject log, Follower log truncated in %d", args.LeaderId, rf.log.snapLastIdx)
		return
	}
	// 后面要使用 PrevLogIndex，先判断，防止越界
	if rf.log.at(args.PrevLogIndex).Term != args.PrevLogTerm {
		reply.ConflictTerm = rf.log.at(args.PrevLogIndex).Term
		reply.ConflictIndex = rf.log.firstLogFor(reply.ConflictTerm)

		LOG(rf.me, rf.currentTerm, DLog2, "<- S%d, Reject log, Prev log not match, [%d]: T%d != T%d", args.LeaderId, args.PrevLogIndex, rf.log.at(args.PrevLogIndex).Term, args.PrevLogTerm)

		return
	}

	// append the leader log entries to local
	rf.log.appendFrom(args.PrevLogIndex, args.Entries)
	rf.persistLocked()

	LOG(rf.me, rf.currentTerm, DLog2, "Follower accept logs: (%d, %d]", args.PrevLogIndex, args.PrevLogIndex+len(args.Entries))
	reply.Success = true

	// TODO: handle LeaderCommit
	if args.LeaderCommit > rf.commitIndex {
		LOG(rf.me, rf.currentTerm, DApply, "Follower Update the commit index %d->%d", rf.commitIndex, args.LeaderCommit)
		rf.commitIndex = args.LeaderCommit
		if rf.log.size()-1 < rf.commitIndex {
			rf.commitIndex = rf.log.size() - 1
		}
		rf.applyCond.Signal()
	}
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	return ok
}

func (rf *Raft) getMajorityIndexLocked() int {
	tmpIndexes := make([]int, len(rf.matchIndex))
	copy(tmpIndexes, rf.matchIndex)
	// todo check
	// 升序排序
	sort.Ints(sort.IntSlice(tmpIndexes))
	// 计算出大多数节点的索引位置。Raft 协议中，大多数节点的定义是超过半数的节点，因此这里使用 (len(rf.peers) - 1) / 2 来确定大多数的位置。
	majorityIdx := (len(rf.peers) - 1) / 2
	LOG(rf.me, rf.currentTerm, DDebug, "Match index after sort: %v, majority[%d]=%d", tmpIndexes, majorityIdx, tmpIndexes[majorityIdx])
	return tmpIndexes[majorityIdx]
}

// only valid in the given `term`
func (rf *Raft) startReplication(term int) bool {
	replicateToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		ok := rf.sendAppendEntries(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()

		if !ok {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Lost or crashed", peer)
			return
		}
		LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Append, Reply=%v", peer, reply.String())

		// align the term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		// check context lost
		if rf.contextLostLocked(Leader, term) {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader -> T%d:%s", peer, term, rf.currentTerm, rf.role)
			return
		}

		// handle the reply
		// probe the lower index if the prevLog not matched
		if !reply.Success {
			prevIndex := rf.nextIndex[peer]
			if reply.ConflictTerm == InvalidTerm {
				rf.nextIndex[peer] = reply.ConflictIndex
			} else {
				firstIndex := rf.log.firstLogFor(reply.ConflictTerm)
				if firstIndex == InvalidTerm {
					rf.nextIndex[peer] = reply.ConflictIndex
				} else {
					rf.nextIndex[peer] = firstIndex + 1
				}
			}

			// avoid the late reply move the nextIndex forward again
			rf.nextIndex[peer] = min(rf.nextIndex[peer], prevIndex)

			// 用于获取指定节点（peer）上的日志的前一个条目的任期（term
			nextPrevIndex := rf.nextIndex[peer] - 1
			nextPrevTerm := InvalidTerm
			if nextPrevIndex >= rf.log.snapLastIdx {
				nextPrevTerm = rf.log.at(nextPrevIndex).Term
			}
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Not matched at Prev=[%d]T%d, Try next Prev=[%d]T%d",
				peer, args.PrevLogIndex, args.PrevLogTerm, nextPrevIndex, nextPrevTerm)
			LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Leader log=%v", peer, rf.log.String())
			return
		}

		// update match/next index if log appended successfully
		rf.matchIndex[peer] = args.PrevLogIndex + len(args.Entries) // important
		rf.nextIndex[peer] = rf.matchIndex[peer] + 1

		// TODO update the commitIndex
		majorityMatched := rf.getMajorityIndexLocked()
		if majorityMatched > rf.commitIndex && rf.log.at(majorityMatched).Term == rf.currentTerm {
			LOG(rf.me, rf.currentTerm, DApply, "Leader Update the commit index %d->%d", rf.commitIndex, majorityMatched)
			rf.commitIndex = majorityMatched
			rf.applyCond.Signal()
		}
	}

	installOnPeer := func(peer int, term int, args *InstallSnapshotArgs) {
		reply := &InstallSnapshotReply{}
		ok := rf.sendInstallSnapshot(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DSnap, "-> S%d, Lost or crashed", peer)
			return
		}
		LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Append, Reply=%v", peer, reply.String())

		// align the term
		if reply.Term > rf.currentTerm {
			rf.becomeFollowerLocked(reply.Term)
			return
		}

		if rf.contextLostLocked(Leader, term) {
			LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
			return
		}

		// update the match and next
		if args.LastIncludedIndex > rf.matchIndex[peer] { // to avoid disorder reply
			rf.matchIndex[peer] = args.LastIncludedIndex
			rf.nextIndex[peer] = args.LastIncludedIndex + 1
		}

		// note: we need not try to update the commitIndex again,
		// because the snapshot included indexes are all committed
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	if rf.contextLostLocked(Leader, term) {
		LOG(rf.me, rf.currentTerm, DLog, "Lost Leader[%d] to %s[T%d]", term, rf.role, rf.currentTerm)
		return false
	}

	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			// Don't forget to update Leader's matchIndex
			rf.matchIndex[peer] = rf.log.size() - 1
			rf.nextIndex[peer] = rf.log.size()
			continue
		}

		// 要发送的条目的上一个条目，探针
		prevIdx := rf.nextIndex[peer] - 1

		if prevIdx < rf.log.snapLastIdx {
			args := &InstallSnapshotArgs{
				Term:              rf.currentTerm,
				LeaderId:          rf.me,
				LastIncludedIndex: rf.log.snapLastIdx,
				LastIncludedTerm:  rf.log.snapLastTerm,
				Snapshot:          rf.log.snapshot,
			}
			LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, SendSnap, Args=%v", peer, args.String())
			go installOnPeer(peer, term, args)
			continue
		}

		prevTerm := rf.log.at(prevIdx).Term
		args := &AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderId:     rf.me,
			PrevLogIndex: prevIdx,
			PrevLogTerm:  prevTerm,
			Entries:      append([]LogEntry(nil), rf.log.tail(prevIdx+1)...),
			LeaderCommit: rf.commitIndex,
		}
		//LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Send log, Prev=[%d]T%d, Len()=%d", peer, args.PrevLogIndex, args.PrevLogTerm, len(args.Entries))
		LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Append, Args=%v", peer, args.String())

		go replicateToPeer(peer, args)
	}

	return true
}

// could only replicate in the given term
func (rf *Raft) replicationTicker(term int) {
	for rf.killed() == false {
		ok := rf.startReplication(term)
		if !ok {
			break
		}

		//rf.mu.Lock()
		//rf.mu.Unlock()
		time.Sleep(replicateInterval)
	}
}
