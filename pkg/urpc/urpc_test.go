// Copyright 2018 The gVisor Authors.
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

package urpc

import (
	"errors"
	"os"
	"testing"

	"gvisor.dev/gvisor/pkg/unet"
)

type test struct {
}

func (t test) Func(a *Echo, r *Echo) error {
	r.Text = a.Text
	r.Number = a.Number
	return nil
}

func (t test) Err(a *Echo, r *Echo) error {
	return errors.New("test error")
}

func (t test) FailNoFile(a *Echo, r *Echo) error {
	if a.Files == nil {
		return errors.New("no file found")
	}

	return nil
}

func (t test) SendFile(a *Echo, r *Echo) error {
	r.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
	return nil
}

func (t test) TooManyFiles(a *Echo, r *Echo) error {
	for i := 0; i <= maxFiles; i++ {
		r.Files = append(r.Files, os.Stdin)
	}
	return nil
}

func startServer(socket *unet.Socket) {
	s := NewServer()
	s.Register(test{})
	s.StartHandling(socket)
}

func testClient() (*Client, error) {
	serverSock, clientSock, err := unet.SocketPair(false)
	if err != nil {
		return nil, err
	}
	startServer(serverSock)

	return NewClient(clientSock), nil
}

func TestCall(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	if err := c.Call("test.Func", &Echo{}, &r); err != nil {
		t.Errorf("basic call failed: %v", err)
	} else if r.Text != "" || r.Number != 0 {
		t.Errorf("unexpected result, got %v expected zero value", r)
	}
	if err := c.Call("test.Func", &Echo{Text: "hello"}, &r); err != nil {
		t.Errorf("basic call failed: %v", err)
	} else if r.Text != "hello" {
		t.Errorf("unexpected result, got %v expected hello", r.Text)
	}
	if err := c.Call("test.Func", &Echo{Number: 1}, &r); err != nil {
		t.Errorf("basic call failed: %v", err)
	} else if r.Number != 1 {
		t.Errorf("unexpected result, got %v expected 1", r.Number)
	}
}

func TestUnknownMethod(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	if err := c.Call("test.Unknown", &Echo{}, &r); err == nil {
		t.Errorf("expected non-nil err, got nil")
	} else if err.Error() != ErrUnknownMethod.Error() {
		t.Errorf("expected test error, got %v", err)
	}
}

func TestErr(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	if err := c.Call("test.Err", &Echo{}, &r); err == nil {
		t.Errorf("expected non-nil err, got nil")
	} else if err.Error() != "test error" {
		t.Errorf("expected test error, got %v", err)
	}
}

func TestSendFile(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	if err := c.Call("test.FailNoFile", &Echo{}, &r); err == nil {
		t.Errorf("expected non-nil err, got nil")
	}
	if err := c.Call("test.FailNoFile", &Echo{FilePayload: FilePayload{Files: []*os.File{os.Stdin, os.Stdout, os.Stdin}}}, &r); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
}

func TestRecvFile(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	if err := c.Call("test.SendFile", &Echo{}, &r); err != nil {
		t.Errorf("expected nil err, got %v", err)
	}
	if r.Files == nil {
		t.Errorf("expected file, got nil")
	}
}

func TestShutdown(t *testing.T) {
	serverSock, clientSock, err := unet.SocketPair(false)
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	clientSock.Close()

	s := NewServer()
	if err := s.Handle(serverSock); err == nil {
		t.Errorf("expected non-nil err, got nil")
	}
}

func TestTooManyFiles(t *testing.T) {
	c, err := testClient()
	if err != nil {
		t.Fatalf("error creating test client: %v", err)
	}
	defer c.Close()

	var r Echo
	var a Echo
	for i := 0; i <= maxFiles; i++ {
		a.Files = append(a.Files, os.Stdin)
	}

	// Client-side error.
	if err := c.Call("test.Func", &a, &r); err != ErrTooManyFiles {
		t.Errorf("expected ErrTooManyFiles, got %v", err)
	}

	// Server-side error.
	if err := c.Call("test.TooManyFiles", &Echo{}, &r); err == nil {
		t.Errorf("expected non-nil err, got nil")
	} else if err.Error() != "too many files" {
		t.Errorf("expected too many files, got %v", err.Error())
	}
}
