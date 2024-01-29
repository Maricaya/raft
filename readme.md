# Implement distributed kv from scratch

## Part A：Implement leader election

To get up and running, execute the following commands. 
```bash
cd src/raft
go test -run PartA -race

../tools/dstest PartA -p 30 -n 100 
```

View detailed logs
```bash
VERBOSE=0 go test -run PartA | tee out.txt

../tools/dslogs -c 3 out.txt
```
