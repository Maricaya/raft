# Implement distributed kv from scratch

## Part A：Implement leader election

To get up and running, execute the following commands. 
```bash
cd src/raft
go test -run PartA -race

python3 ../tools/dstest.py PartA -p 30 -n 100 
dstest PartA -p 30 -n 100 
```

View detailed logs
```bash
VERBOSE=0 go test -run PartA | tee out.txt
VERBOSE=0 go test -run TestBasicAgreePartB | tee out.txt
VERBOSE=0 go test -run TestFigure8PartC | tee out.txt


../tools/dslogs -c 5 out.txt
```
