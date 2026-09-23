// Copyright 2026 The gVisor Authors.
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

package control

import (
	"encoding/json"
	"os"
	"testing"

	"gvisor.dev/gvisor/pkg/sentry/kernel/auth"
	"gvisor.dev/gvisor/pkg/urpc"
)

// execArgs is what `runsc exec` sends: the argument of the hottest call urpc
// carries.
func execArgs() *ExecArgs {
	return &ExecArgs{
		Filename: "/bin/sh",
		Argv:     []string{"/bin/sh", "-c", "echo hello"},
		Envv: []string{
			"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
			"HOSTNAME=6f3c9d2b1a4e",
			"TERM=xterm",
			"HOME=/root",
		},
		WorkingDirectory: "/var/lib/app",
		KUID:             auth.KUID(1000),
		KGID:             auth.KGID(1000),
		ExtraKGIDs:       []auth.KGID{4, 24, 27, 30, 46},
		NoNewPrivileges:  true,
		Capabilities: &auth.TaskCapabilities{
			PermittedCaps:   auth.CapabilitySet(0x00000000a80425fb),
			InheritableCaps: auth.CapabilitySet(0),
			EffectiveCaps:   auth.CapabilitySet(0x00000000a80425fb),
			BoundingCaps:    auth.CapabilitySet(0x00000000a80425fb),
		},
		StdioIsPty:  true,
		SupportTTYs: true,
		ContainerID: "6f3c9d2b1a4e8c07b5d31f2a9e4c6081d7b3a5f2c9e08d1467b2a35c8f9017de",
		FilePayload: FilePayload{
			FilePayload: urpc.FilePayload{Files: []*os.File{os.Stdin, os.Stdout, os.Stderr}},
			GuestFDs:    []int{0, 1, 2},
		},
	}
}

// TestExecArgsCrosses holds the ZAP layout against the value it carries: every
// field the wire is supposed to carry comes back, and the ones that cannot --
// the files, which ride SCM_RIGHTS -- are the caller's to set.
func TestExecArgsCrosses(t *testing.T) {
	want := execArgs()
	data, err := want.MarshalZAP()
	if err != nil {
		t.Fatalf("MarshalZAP: %v", err)
	}
	var got ExecArgs
	if err := got.UnmarshalZAP(data); err != nil {
		t.Fatalf("UnmarshalZAP: %v", err)
	}
	got.Files = want.Files
	if got.Filename != want.Filename ||
		got.WorkingDirectory != want.WorkingDirectory ||
		got.ContainerID != want.ContainerID ||
		got.KUID != want.KUID || got.KGID != want.KGID ||
		got.NoNewPrivileges != want.NoNewPrivileges ||
		got.StdioIsPty != want.StdioIsPty || got.SupportTTYs != want.SupportTTYs {
		t.Errorf("scalars differ:\ngot  %+v\nwant %+v", got, want)
	}
	if len(got.Argv) != len(want.Argv) || len(got.Envv) != len(want.Envv) || len(got.ExtraKGIDs) != len(want.ExtraKGIDs) {
		t.Fatalf("list lengths differ:\ngot  %+v\nwant %+v", got, want)
	}
	for i := range want.Argv {
		if got.Argv[i] != want.Argv[i] {
			t.Errorf("Argv[%d] = %q, want %q", i, got.Argv[i], want.Argv[i])
		}
	}
	for i := range want.Envv {
		if got.Envv[i] != want.Envv[i] {
			t.Errorf("Envv[%d] = %q, want %q", i, got.Envv[i], want.Envv[i])
		}
	}
	for i := range want.ExtraKGIDs {
		if got.ExtraKGIDs[i] != want.ExtraKGIDs[i] {
			t.Errorf("ExtraKGIDs[%d] = %d, want %d", i, got.ExtraKGIDs[i], want.ExtraKGIDs[i])
		}
	}
	if got.Capabilities == nil || *got.Capabilities != *want.Capabilities {
		t.Errorf("Capabilities = %+v, want %+v", got.Capabilities, want.Capabilities)
	}
}

// BenchmarkExecArgsZAP is one ExecArgs out and back through the envelope urpc
// writes today.
func BenchmarkExecArgsZAP(b *testing.B) {
	args := execArgs()
	b.ReportAllocs()
	for b.Loop() {
		arg, err := args.MarshalZAP()
		if err != nil {
			b.Fatal(err)
		}
		call := urpc.Call{Method: "containerManager.ExecuteAsync", Arg: arg}
		data, err := call.MarshalZAP()
		if err != nil {
			b.Fatal(err)
		}
		var in urpc.Call
		if err := in.UnmarshalZAP(data); err != nil {
			b.Fatal(err)
		}
		var out ExecArgs
		if err := out.UnmarshalZAP(in.Arg); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecArgsJSON is the same value through the envelope urpc used to
// write, so the two numbers are the same journey.
func BenchmarkExecArgsJSON(b *testing.B) {
	type clientCall struct {
		Method string `json:"method"`
		Arg    any    `json:"arg"`
	}
	type serverCall struct {
		Method string          `json:"method"`
		Arg    json.RawMessage `json:"arg"`
	}
	args := execArgs()
	b.ReportAllocs()
	for b.Loop() {
		data, err := json.Marshal(&clientCall{Method: "containerManager.ExecuteAsync", Arg: args})
		if err != nil {
			b.Fatal(err)
		}
		var in serverCall
		if err := json.Unmarshal(data, &in); err != nil {
			b.Fatal(err)
		}
		var out ExecArgs
		if err := json.Unmarshal(in.Arg, &out); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecArgsZAPMarshal is the encode half alone.
func BenchmarkExecArgsZAPMarshal(b *testing.B) {
	args := execArgs()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := args.MarshalZAP(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecArgsZAPUnmarshal is the decode half alone.
func BenchmarkExecArgsZAPUnmarshal(b *testing.B) {
	data, err := execArgs().MarshalZAP()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var out ExecArgs
		if err := out.UnmarshalZAP(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCallZAP is the envelope alone.
func BenchmarkCallZAP(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		call := urpc.Call{Method: "containerManager.ExecuteAsync", Arg: []byte("0123456789abcdef")}
		data, err := call.MarshalZAP()
		if err != nil {
			b.Fatal(err)
		}
		var in urpc.Call
		if err := in.UnmarshalZAP(data); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecArgsJSONMarshal is the encode half alone.
func BenchmarkExecArgsJSONMarshal(b *testing.B) {
	args := execArgs()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(args); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExecArgsJSONUnmarshal is the decode half alone.
func BenchmarkExecArgsJSONUnmarshal(b *testing.B) {
	data, err := json.Marshal(execArgs())
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var out ExecArgs
		if err := json.Unmarshal(data, &out); err != nil {
			b.Fatal(err)
		}
	}
}
