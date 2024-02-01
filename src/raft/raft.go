package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	//	"bytes"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	//	"course/labgob"
	"course/labrpc"
)

// todo 这些值是怎么确定的？
const (
	electionTimeoutMin time.Duration = 250 * time.Millisecond
	electionTimeoutMax time.Duration = 400 * time.Millisecond

	replicateInterval = 250 * time.Millisecond
)

func (rf *Raft) resetElectionTimerLocked() {
	rf.electionStart = time.Now()
	randRange := int64(electionTimeoutMax - electionTimeoutMin)
	// 时间随机
	rf.electionTimeout = electionTimeoutMin + time.Duration(rand.Int63()%randRange)
}

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in part PartD you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh, but set CommandValid to false for these
// other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int

	// For PartD:
	SnapshotValid bool
	Snapshot      []byte
	SnapshotTerm  int
	SnapshotIndex int
}

type Role string

const (
	Follower  Role = "Follower"
	Candidate Role = "Candidate"
	Leader    Role = "Leader"
)

/*
任期大于当前任期 -> Follower
重置 voteFor
当前任期
*/
func (rf *Raft) becomeFollowerLocked(term int) {
	if term < rf.currentTerm {
		LOG(rf.me, rf.currentTerm, DError, "can not become follower, lower term: T%d", term)
		return
	}
	LOG(rf.me, rf.currentTerm, DLog, "%s -> follower For T%v->T%v", rf.role, rf.currentTerm, term)
	if term > rf.currentTerm {
		rf.votedFor = -1
	}
	rf.role = Follower
	rf.currentTerm = term
}

func (rf *Raft) becomeCandidateLocked() {
	if rf.role == Leader {
		LOG(rf.me, rf.currentTerm, DError, "Leader can not become Candidate")
	}
	LOG(rf.me, rf.currentTerm, DLog, "%s -> Candidate For T%v->T%v", rf.role, rf.currentTerm, rf.currentTerm+1)

	rf.role = Candidate
	rf.currentTerm++
	rf.votedFor = rf.me
}

func (rf *Raft) becomeLeaderLocked() {
	if rf.role != Candidate {
		LOG(rf.me, rf.currentTerm, DError, "%s can not become Leader", rf.role)
	}
	LOG(rf.me, rf.currentTerm, DLog, "%s -> Leader For T%v->T%v", rf.role, rf.currentTerm, rf.currentTerm)
	rf.role = Leader

	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	for i := range rf.peers {
		rf.nextIndex[i] = rf.getLastIndex() + 1
		rf.matchIndex[i] = 0
	}
}

func (rf *Raft) contextLostLocked(role Role, term int) bool {
	return !(rf.role == role && rf.currentTerm == term)
}

type LogEntry struct {
	LogIndex int
	LogTerm  int
	Cmd      interface{}
}

func (rf *Raft) getLastIndex() int {
	return rf.log[len(rf.log)-1].LogIndex
}

func (rf *Raft) getLastTerm() int {
	return rf.log[len(rf.log)-1].LogTerm
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (PartA, PartB, PartC).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	role        Role
	currentTerm int
	votedFor    int // -1 means vote for none
	log         []LogEntry
	applyCh     chan ApplyMsg

	electionStart   time.Time
	electionTimeout time.Duration // random

	// todo Volatile state 什么意思
	commitIndex int
	lastApplied int

	// leader
	nextIndex  []int
	matchIndex []int
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term := rf.currentTerm
	isLeader := rf.role == Leader
	return term, isLeader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	// Your code here (PartC).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// raftstate := w.Bytes()
	// rf.persister.Save(raftstate, nil)
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
}

// the service says it has created a snapshot that has
// all info up to and including index. this means the
// service no longer needs the log through (and including)
// that index. Raft should now trim its log as much as possible.
func (rf *Raft) Snapshot(index int, snapshot []byte) {
	// Your code here (PartD).

}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (PartA, PartB).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	Term        int
	VoteGranted bool // 已投票
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (PartA, PartB).
	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm {
		reply.VoteGranted = false
		return
	}
	if args.Term > rf.currentTerm {
		// 投票
		rf.becomeFollowerLocked(args.Term)
	}
	if args.LastLogTerm < rf.getLastTerm() ||
		(args.LastLogTerm == rf.getLastTerm() &&
			args.LastLogIndex < rf.getLastIndex()) {
		reply.VoteGranted = false
		LOG(rf.me, rf.currentTerm, DError, "candidate's log < receiver's log")

		return
	} else if rf.votedFor != -1 {
		reply.VoteGranted = false
		LOG(rf.me, rf.currentTerm, DError, "There are already voters available: %v", rf.votedFor)
		return
	}
	if args.LastLogIndex > rf.getLastIndex() {

	}
	reply.VoteGranted = true
	rf.votedFor = args.CandidateId
	// todo 比较日志
	// candidate's log is at least as up-to-date as receiver's log, grant vote
	//if rf.log[1] > reply.Term {
	//
	//}
	rf.resetElectionTimerLocked()
	LOG(rf.me, rf.currentTerm, DVote, "-> S%d, vote granted", args.CandidateId)

}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PreLogIndex  int
	PreLogTerm   int
	Entries      []LogEntry
	LeaderCommit int
}
type AppendEntriesReply struct {
	Term      int
	Success   bool
	NextIndex int
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	reply.Term = rf.currentTerm
	if args.Term < rf.currentTerm {
		reply.Success = false
		return
	}

	// todo ????
	if args.PreLogIndex > rf.getLastIndex() {
		reply.NextIndex = rf.getLastIndex() + 1
		return
	}

	// todo ????
	term := rf.log[args.PreLogIndex].LogTerm
	if args.PreLogTerm != term {
		for i := args.PreLogIndex - 1; i >= 0; i-- {
			if rf.log[i].LogTerm != term {
				reply.NextIndex = i + 1
				break
			}
		}
		return
	}
	// todo ?? 什么意思
	rf.log = rf.log[:args.PreLogIndex+1]
	rf.log = append(rf.log, args.Entries...)
	reply.NextIndex = rf.getLastIndex() + 1

	// commit log
	if args.LeaderCommit > rf.commitIndex {
		rf.commitIndex = min(args.LeaderCommit, rf.getLastIndex())
		//rf.chanCommit <- true
	}

	if args.Term > rf.currentTerm {
		rf.becomeFollowerLocked(args.Term)
		reply.Success = true
		// 重置选举计时器
		rf.resetElectionTimerLocked()
	}

}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) bool {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	return ok
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	rf.mu.Lock()
	defer rf.mu.Unlock()

	//DPrintf("%d send append entities to %d :%s",rf.me,server,reply)

	// todo 这块为啥要这样写？？
	if ok {
		// if rf is not the leader or outofdate, return
		if rf.role != Leader || args.Term != rf.currentTerm {
			return ok
		}

		// if higher term is found, set follower and return
		if rf.currentTerm < reply.Term {
			rf.becomeFollowerLocked(reply.Term)
			return ok
		}

		// set the next index of follower
		rf.nextIndex[server] = reply.NextIndex
		// if append entries successfully, set match index of follower
		if reply.Success {
			rf.matchIndex[server] = rf.nextIndex[server] - 1
		}
	}
	return ok
}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	term, isLeader := rf.GetState()

	if rf.role != Leader {
		return -1, term, isLeader
	}
	// todo Your code here (PartB).
	rf.log = append(rf.log, LogEntry{
		LogTerm:  rf.currentTerm,
		LogIndex: len(rf.log),
		Cmd:      command,
	})
	index := rf.getLastIndex()
	// start the agreement and return immediately
	rf.startReplication(term)
	// AppendEntries RPC 2 follower
	// 将所有已提交日志按顺序发送到 applyCh

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) startReplication(term int) bool {
	replicateToPeer := func(peer int, args *AppendEntriesArgs) {
		reply := &AppendEntriesReply{}
		rf.sendAppendEntries(peer, args, reply)
		// todo 这块要做什么
	}

	rf.contextLostLocked(Leader, rf.currentTerm)

	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			continue
		}

		args := &AppendEntriesArgs{
			Term:     rf.currentTerm,
			LeaderId: rf.me,
			// todo 这些值应该填什么？
			PreLogIndex:  rf.nextIndex[peer] - 1,
			PreLogTerm:   rf.log[rf.nextIndex[peer]-1].LogTerm,
			LeaderCommit: rf.commitIndex,
		}
		args.Entries = make([]LogEntry, len(rf.log[rf.nextIndex[peer]:]))
		copy(args.Entries, rf.log[rf.nextIndex[peer]:])

		go replicateToPeer(peer, args)
	}
	return true
}

func (rf *Raft) replicationTicker(term int) {
	// todo ???
	//rf.mu.Lock()
	//defer rf.mu.Unlock()

	for !rf.killed() {
		// todo ???
		ok := rf.startReplication(term)
		if !ok {
			return
		}
		time.Sleep(replicateInterval)
	}

}

func (rf *Raft) startElection(term int) bool {
	votes := 0
	askVoteFromPeer := func(peer int, args *RequestVoteArgs) bool {
		reply := &RequestVoteReply{}
		ok := rf.sendRequestVote(peer, args, reply)

		rf.mu.Lock()
		defer rf.mu.Unlock()
		if !ok {
			LOG(rf.me, rf.currentTerm, DDebug, "Ask vote from %d, Lost or error", peer)
			return false
		}

		if reply.Term > rf.currentTerm {
			// become follower
			rf.becomeFollowerLocked(reply.Term)
			return false
		}

		// todo 为什么要 check context
		if rf.contextLostLocked(Candidate, rf.currentTerm) {
			LOG(rf.me, rf.currentTerm, DVote, "Lost Candidate to %s, abort RequestVote", rf.role)
			return false
		}

		// 计算投给自己的票数
		if reply.VoteGranted {
			votes++
		}

		if votes > len(rf.peers)/2 {
			if rf.role == Candidate {
				rf.becomeLeaderLocked()
				go rf.replicationTicker(term)
			}
		}
		return true
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()

	// todo 为什么要判断任期一致
	if rf.contextLostLocked(Candidate, rf.currentTerm) {
		LOG(rf.me, rf.currentTerm, DVote, "Lost Candidate to %s, abort RequestVote", rf.role)
		return false
	}

	for peer := 0; peer < len(rf.peers); peer++ {
		if peer == rf.me {
			votes++
			continue
		}

		args := &RequestVoteArgs{
			Term:         rf.currentTerm,
			CandidateId:  rf.me,
			LastLogIndex: rf.getLastIndex(),
			LastLogTerm:  rf.getLastTerm(),
		}

		go askVoteFromPeer(peer, args)
	}
	return true
}

func (rf *Raft) electionTicker() {
	for rf.killed() == false {

		rf.mu.Lock()
		// Your code here (PartA)
		// Check if a leader election should be started.
		// 超时，Follower 成为 Candidate
		if rf.role != Leader && time.Since(rf.electionStart) > rf.electionTimeout {
			rf.becomeCandidateLocked()
			// 开始选举
			go rf.startElection(rf.currentTerm)
		}
		rf.mu.Unlock()
		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 300)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (PartA, PartB, PartC).
	rf.role = Follower
	rf.currentTerm = 0
	rf.votedFor = -1

	rf.log = append(rf.log, LogEntry{LogTerm: 0})
	rf.mu.Lock()
	for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
		applyCh <- ApplyMsg{CommandIndex: i, Command: rf.log[i].Cmd}
		//DPrintf("%d apply index %d: %d", rf.me, i, rf.log[i])
		rf.lastApplied = i
	}
	rf.applyCh = applyCh
	rf.mu.Unlock()

	//rf.log = applyCh

	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())

	// start ticker goroutine to start elections
	go rf.electionTicker()

	return rf
}
