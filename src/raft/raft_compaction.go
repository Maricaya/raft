package raft

import "fmt"

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (PartD).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if index > rf.commitIndex {
		LOG(rf.me, rf.currentTerm, DSnap, "Couldn't snapshot before CommitIdx: %d>%d", index, rf.commitIndex)
		return
	}
	if index <= rf.log.snapLastIdx {
		LOG(rf.me, rf.currentTerm, DSnap, "Already snapshot in %d<=%d", index, rf.log.snapLastIdx)
		return
	}

	rf.log.doSnapshot(index, snapshot)
	rf.persistLocked()
}

type InstallSnapshotArgs struct {
	Term     int
	LeaderId int

	LastIncludedIndex int
	LastIncludedTerm  int

	Snapshot []byte
}

func (args *InstallSnapshotArgs) String() string {
	return fmt.Sprintf("Leader-%d, T%d, Last: [%d]T%d", args.LeaderId, args.Term, args.LastIncludedIndex, args.LastIncludedTerm)
}

type InstallSnapshotReply struct {
	Term int
}

func (reply *InstallSnapshotReply) String() string {
	return fmt.Sprintf("T%d", reply.Term)
}

// follower
func (rf *Raft) InstallSnapshot(args *InstallSnapshotArgs, reply *InstallSnapshotReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	LOG(rf.me, rf.currentTerm, DDebug, "<- S%d, RecvSnap, Args=%v", args.LeaderId, args.String())

	reply.Term = rf.currentTerm
	// align the term
	if args.Term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DSnap, "<- S%d, Reject Snap, Higher Term, T%d>T%d", args.LeaderId, rf.currentTerm, args.Term)
		return
	}
	if args.Term >= rf.currentTerm { // handle the case when the peer is candidate
		rf.becomeFollowerLocked(args.Term)
	}

	// check if it is a RPC which is out of order
	if rf.log.snapLastIdx >= args.LastIncludedIndex {
		LOG(rf.me, rf.currentTerm, DSnap, "<- S%d, Reject Snap, Already installed, Last: %d>=%d", args.LeaderId, rf.log.snapLastIdx, args.LastIncludedIndex)
		return
	}
	// install the snapshot in the memory/persister/app
	rf.log.installSnapshot(args.LastIncludedIndex, args.LastIncludedTerm, args.Snapshot)
	rf.persistLocked()
	rf.snapPending = true
	rf.applyCond.Signal()
}

// leader
func (rf *Raft) sendInstallSnapshot(server int, args *InstallSnapshotArgs, reply *InstallSnapshotReply) bool {
	ok := rf.peers[server].Call("Raft.InstallSnapshot", args, reply)
	return ok
}

//func (rf *Raft) installOnPeer(peer, term int, args *InstallSnapshotArgs) {
//	reply := &InstallSnapshotReply{}
//	ok := rf.sendInstallSnapshot(peer, args, reply)
//
//	rf.mu.Lock()
//	defer rf.mu.Unlock()
//	if !ok {
//		LOG(rf.me, rf.currentTerm, DSnap, "-> S%d, Lost or crashed", peer)
//		return
//	}
//	LOG(rf.me, rf.currentTerm, DDebug, "-> S%d, Append, Reply=%v", peer, reply.String())
//
//	// align the term
//	if reply.Term > rf.currentTerm {
//		rf.becomeFollowerLocked(reply.Term)
//		return
//	}
//
//	if rf.contextLostLocked(Leader, term) {
//		LOG(rf.me, rf.currentTerm, DLog, "-> S%d, Context Lost, T%d:Leader->T%d:%s", peer, term, rf.currentTerm, rf.role)
//		return
//	}
//
//	// update the match and next
//	if args.LastIncludedIndex > rf.matchIndex[peer] { // to avoid disorder reply
//		rf.matchIndex[peer] = args.LastIncludedIndex
//		rf.nextIndex[peer] = args.LastIncludedIndex + 1
//	}
//
//	// note: we need not try to update the commitIndex again,
//	// because the snapshot included indexes are all committed
//}
