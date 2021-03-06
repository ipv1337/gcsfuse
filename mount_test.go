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

package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/gcloud/gcs"
	"github.com/jacobsa/gcloud/gcs/gcsfake"
	"github.com/jacobsa/gcloud/gcs/gcsutil"
	. "github.com/jacobsa/oglematchers"
	. "github.com/jacobsa/ogletest"
	"github.com/jacobsa/timeutil"
)

func TestMount(t *testing.T) { RunTests(t) }

////////////////////////////////////////////////////////////////////////
// Boilerplate
////////////////////////////////////////////////////////////////////////

type MountTest struct {
	ctx   context.Context
	clock timeutil.SimulatedClock
	conn  gcs.Conn

	// A temporary directory that is cleaned up at the end of the test run.
	dir string
}

var _ SetUpInterface = &MountTest{}
var _ TearDownInterface = &MountTest{}

func init() { RegisterTestSuite(&MountTest{}) }

func (t *MountTest) SetUp(ti *TestInfo) {
	var err error

	t.ctx = ti.Ctx
	t.clock.SetTime(time.Date(2012, 8, 15, 22, 56, 0, 0, time.Local))
	t.conn = gcsfake.NewConn(&t.clock)

	// Set up the temporary directory.
	t.dir, err = ioutil.TempDir("", "run_test")
	AssertEq(nil, err)
}

func (t *MountTest) TearDown() {
	var err error

	err = os.RemoveAll(t.dir)
	AssertEq(nil, err)
}

func (t *MountTest) mount(
	args []string) (mfs *fuse.MountedFileSystem, err error) {
	fs := new(flag.FlagSet)

	// Don't spam the console on error.
	fs.Usage = func() {}

	mfs, err = mount(t.ctx, args, fs, t.conn)
	return
}

// Unmount the file system. Try again on "resource busy" errors.
func (t *MountTest) unmount() (err error) {
	delay := 10 * time.Millisecond
	for {
		err = fuse.Unmount(t.dir)
		if err == nil {
			return
		}

		if strings.Contains(err.Error(), "resource busy") {
			log.Println("Resource busy error while unmounting; trying again")
			time.Sleep(delay)
			delay = time.Duration(1.3 * float64(delay))
			continue
		}

		err = fmt.Errorf("Unmount: %v", err)
		return
	}
}

////////////////////////////////////////////////////////////////////////
// Tests
////////////////////////////////////////////////////////////////////////

func (t *MountTest) IncorrectUsage() {
	var err error

	// No args
	_, err = t.mount([]string{})
	log.Printf("FOO: %T %q", err, err.Error())
	ExpectThat(err, Error(HasSubstr("usage")))

	// One arg
	_, err = t.mount([]string{"foo"})
	ExpectThat(err, Error(HasSubstr("usage")))

	// Three args
	_, err = t.mount([]string{"foo", "bar", "baz"})
	ExpectThat(err, Error(HasSubstr("usage")))
}

func (t *MountTest) HelpFlags() {
	var err error

	_, err = t.mount([]string{"-h"})
	ExpectEq(flag.ErrHelp, err)

	_, err = t.mount([]string{"--help"})
	ExpectEq(flag.ErrHelp, err)
}

func (t *MountTest) BasicUsage() {
	var err error
	const fileName = "foo"

	// Grab a bucket.
	bucket, err := t.conn.OpenBucket(t.ctx, "some_bucket")
	AssertEq(nil, err)

	// Mount that bucket.
	mfs, err := t.mount([]string{
		bucket.Name(),
		t.dir,
	})

	AssertEq(nil, err)

	// Create a file.
	err = ioutil.WriteFile(path.Join(t.dir, fileName), []byte("taco"), 0400)
	AssertEq(nil, err)

	// Read the object from the bucket.
	contents, err := gcsutil.ReadObject(t.ctx, bucket, fileName)
	AssertEq(nil, err)
	ExpectEq("taco", string(contents))

	// Read the file.
	contents, err = ioutil.ReadFile(path.Join(t.dir, fileName))
	AssertEq(nil, err)
	ExpectEq("taco", string(contents))

	// Unmount and join.
	err = t.unmount()
	AssertEq(nil, err)

	err = mfs.Join(t.ctx)
	AssertEq(nil, err)
}
