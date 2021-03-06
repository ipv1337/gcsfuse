// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fs_test

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"runtime"
	"sync"
	"time"

	"github.com/jacobsa/fuse/fusetesting"
	. "github.com/jacobsa/ogletest"
)

////////////////////////////////////////////////////////////////////////
// Helpers
////////////////////////////////////////////////////////////////////////

// Run the supplied function for each name, with parallelism.
func forEachName(names []string, f func(string)) {
	const parallelism = 8

	// Fill a channel.
	c := make(chan string, len(names))
	for _, n := range names {
		c <- n
	}
	close(c)

	// Run workers.
	var wg sync.WaitGroup
	for i := 0; i < parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range c {
				f(n)
			}
		}()
	}

	wg.Wait()
}

////////////////////////////////////////////////////////////////////////
// Stress testing
////////////////////////////////////////////////////////////////////////

type StressTest struct {
	fsTest
}

func init() { RegisterTestSuite(&StressTest{}) }

func (t *StressTest) CreateAndReadManyFilesInParallel() {
	// Ensure that we get parallelism for this test.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))

	// Exercise lease revocation logic.
	numFiles := 2 * t.serverCfg.TempDirLimitNumFiles

	// Choose a bunch of file names.
	var names []string
	for i := 0; i < numFiles; i++ {
		names = append(names, fmt.Sprintf("%d", i))
	}

	// Create a file for each name with concurrent workers.
	forEachName(
		names,
		func(n string) {
			err := ioutil.WriteFile(path.Join(t.Dir, n), []byte(n), 0400)
			AssertEq(nil, err)
		})

	// Read each back.
	forEachName(
		names,
		func(n string) {
			contents, err := ioutil.ReadFile(path.Join(t.Dir, n))
			AssertEq(nil, err)
			AssertEq(n, string(contents))
		})
}

func (t *StressTest) TruncateFileManyTimesInParallel() {
	// Ensure that we get parallelism for this test.
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(runtime.NumCPU()))

	// Create a file.
	f, err := os.Create(path.Join(t.Dir, "foo"))
	AssertEq(nil, err)
	defer f.Close()

	// Set up a function that repeatedly truncates the file to random lengths,
	// writing the final size to a channel.
	worker := func(finalSize chan<- int64) {
		const desiredDuration = 500 * time.Millisecond

		var size int64
		startTime := time.Now()
		for time.Since(startTime) < desiredDuration {
			for i := 0; i < 10; i++ {
				size = rand.Int63n(1 << 14)
				err := f.Truncate(size)
				AssertEq(nil, err)
			}
		}

		finalSize <- size
	}

	// Run several workers.
	const numWorkers = 16
	finalSizes := make(chan int64, numWorkers)

	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker(finalSizes)
		}()
	}

	wg.Wait()
	close(finalSizes)

	// The final size should be consistent.
	fi, err := f.Stat()
	AssertEq(nil, err)

	var found = false
	for s := range finalSizes {
		if s == fi.Size() {
			found = true
			break
		}
	}

	ExpectTrue(found, "Unexpected size: %d", fi.Size())
}

func (t *StressTest) CreateInParallel_NoTruncate() {
	fusetesting.RunCreateInParallelTest_NoTruncate(t.ctx, t.Dir)
}

func (t *StressTest) CreateInParallel_Truncate() {
	fusetesting.RunCreateInParallelTest_Truncate(t.ctx, t.Dir)
}

func (t *StressTest) CreateInParallel_Exclusive() {
	fusetesting.RunCreateInParallelTest_Exclusive(t.ctx, t.Dir)
}

func (t *StressTest) MkdirInParallel() {
	fusetesting.RunMkdirInParallelTest(t.ctx, t.Dir)
}

func (t *StressTest) SymlinkInParallel() {
	fusetesting.RunSymlinkInParallelTest(t.ctx, t.Dir)
}
