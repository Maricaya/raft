package raft

import "fmt"

func (rf *Raft) applicationTicker() {
	for !rf.killed() {
		rf.mu.Lock()
		// todo ??
		rf.applyCond.Wait()

		entries := make([]LogEntry, 0)
		for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
			entries = append(entries, rf.log[i])
			//fmt.Println("rf.log[i]", rf.log[i])
		}
		//println("hhhhhh")
		//fmt.Println(entries)
		for _, entry := range entries {
			fmt.Println(entry)
		}
		rf.mu.Unlock()

		for i, entry := range entries {
			rf.applyCh <- ApplyMsg{
				CommandValid: entry.CommandValid,
				Command:      entry.Command,
				CommandIndex: rf.lastApplied + 1 + i, // must be cautious
			}
			fmt.Println(entry)
		}

		rf.mu.Lock()
		LOG(rf.me, rf.currentTerm, DApply, "Apply log for [%d, %d]", rf.lastApplied+1, rf.lastApplied+len(entries))
		rf.lastApplied += len(entries)
		rf.mu.Unlock()
	}
}
