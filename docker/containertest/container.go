// Licensed to Elasticsearch B.V. under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Elasticsearch B.V. licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package containertest

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
)

// NOTE: This is PoC code and should not be used in production. It's goal is to
// demonstrate that cleanup can occur when tests exit. It has many issues
// including a bad API and no thread-safety.
//
// When you Ctrl+C Bazel it stops the process hard and none of the signal
// handlers run. When you Ctrl+C a regular 'go test' the cleanup will run.

var testContext context.Context

var cleanupFuncs []func()

type Runner interface {
	Run() int
}

func RunAndCleanup(runner Runner) int {
	testContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup

	// Execute cleanup when TestMain completes or a signal is received.
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-testContext.Done()
		fmt.Printf("Running %d cleanup functions.\n", len(cleanupFuncs))
		for _, f := range cleanupFuncs {
			f()
		}
	}()

	// Handle external interrupt signals then exit with non-zero.
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)
	go func() {
		sig := <-signalCh
		fmt.Println("Received signal: ", sig)
		cancel()

		// Wait for cleanup to complete then exit.
		wg.Wait()
		os.Exit(1)
	}()

	return runner.Run()
}

func RunAtExit(f func()) {
	cleanupFuncs = append(cleanupFuncs, f)
}

func cleanup() {
	for _, f := range cleanupFuncs {
		f()
	}
}
