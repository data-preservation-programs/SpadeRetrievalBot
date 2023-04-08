package main

import (
	"context"
	"github.com/data-preservation-programs/RetrievalBot/pkg/task"
	"github.com/data-preservation-programs/RetrievalBot/worker/graphsync"
)

func main() {
	worker := graphsync.Worker{}
	process, err := task.NewTaskWorkerProcess(context.Background(), task.GraphSync, worker)
	if err != nil {
		panic(err)
	}

	defer process.Close()

	err = process.Poll(context.Background())
	if err != nil {
		panic(err)
	}
}
