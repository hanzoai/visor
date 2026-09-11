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

// Command zap states the ZAP wire of every urpc payload, reading it from the
// type and writing it as Go source with the offsets as constants. Nothing
// reflects when a call is served.
//
// Run it after changing a payload:
//
//	make zap
//
// A payload it cannot state is named on stderr and left alone. Answering that
// is a change to the type: a map, an interface or a fixed array of anything but
// bytes has no offset, so there is no layout to write down.
//
// The layouts live in the packages that declare the payloads, so a type change
// can leave a layout that no longer compiles — which is the point, since a
// stale offset is silent corruption. `make zap` empties each layout before
// regenerating it, so the package the generator reflects over is the package
// the types are in today.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/zap-proto/zip"

	"gvisor.dev/gvisor/pkg/sentry/control"
	"gvisor.dev/gvisor/pkg/sentry/state/stateipc"
	"gvisor.dev/gvisor/pkg/urpc"
	"gvisor.dev/gvisor/runsc/boot"
	"gvisor.dev/gvisor/runsc/container"
)

// module is the import path this repository serves. A layout outside it names a
// package we cannot write to, which is a refusal and not an output.
const module = "gvisor.dev/gvisor/"

// roots are the arguments and results of every method urpc serves. Everything
// they reach is stated with them, so only the top of each call appears here.
//
// A payload missing from this list states no wire, and the call that carries it
// fails naming the type. Adding a method means adding its two payloads.
var roots = []reflect.Type{
	// pkg/urpc: the envelope itself.
	reflect.TypeFor[urpc.Call](),
	reflect.TypeFor[urpc.Echo](),
	reflect.TypeFor[urpc.FilePayload](),
	reflect.TypeFor[urpc.Result](),

	// pkg/sentry/control.
	reflect.TypeFor[control.CatOpts](),
	reflect.TypeFor[control.CgroupsReadArgs](),
	reflect.TypeFor[control.CgroupsResults](),
	reflect.TypeFor[control.CgroupsWriteArgs](),
	reflect.TypeFor[control.ContainerArgs](),
	reflect.TypeFor[control.EventsOpts](),
	reflect.TypeFor[control.ExecArgs](),
	reflect.TypeFor[control.ExitStatus](),
	reflect.TypeFor[control.GetRegisteredMetricsOpts](),
	reflect.TypeFor[control.LoggingArgs](),
	reflect.TypeFor[control.MemoryUsage](),
	reflect.TypeFor[control.MemoryUsageFile](),
	reflect.TypeFor[control.MemoryUsageFileOpts](),
	reflect.TypeFor[control.MemoryUsageOpts](),
	reflect.TypeFor[control.MetricsExportData](),
	reflect.TypeFor[control.MetricsExportOpts](),
	reflect.TypeFor[control.MetricsRegistrationResponse](),
	reflect.TypeFor[control.MountOpts](),
	reflect.TypeFor[control.Process](),
	reflect.TypeFor[control.PsArgs](),
	reflect.TypeFor[control.PsResult](),
	reflect.TypeFor[control.ReadOpts](),
	reflect.TypeFor[control.RunningResult](),
	reflect.TypeFor[control.SaveOpts](),
	reflect.TypeFor[control.SaveRestoreExecOpts](),
	reflect.TypeFor[control.SignalContainerArgs](),
	reflect.TypeFor[control.SignalProcessArgs](),
	reflect.TypeFor[control.StartContainerArgs](),
	reflect.TypeFor[control.TarRootfsUpperLayerOpts](),
	reflect.TypeFor[control.UmountOpts](),
	reflect.TypeFor[control.UsageReduceOpts](),
	reflect.TypeFor[control.UsageReduceOutput](),

	// pkg/sentry/control, the profile options.
	reflect.TypeFor[control.BlockProfileOpts](),
	reflect.TypeFor[control.CPUProfileOpts](),
	reflect.TypeFor[control.GoroutineProfileOpts](),
	reflect.TypeFor[control.HeapProfileOpts](),
	reflect.TypeFor[control.MutexProfileOpts](),
	reflect.TypeFor[control.TraceProfileOpts](),

	// pkg/sentry/state/stateipc.
	reflect.TypeFor[stateipc.CloseRequest](),
	reflect.TypeFor[stateipc.CloseResponse](),
	reflect.TypeFor[stateipc.OpenRequest](),
	reflect.TypeFor[stateipc.OpenResponse](),
	reflect.TypeFor[stateipc.RegisterClientFileRequest](),
	reflect.TypeFor[stateipc.RegisterClientFileResponse](),

	// runsc/boot.
	reflect.TypeFor[boot.CreateArgs](),
	reflect.TypeFor[boot.CreateLinksAndRoutesArgs](),
	reflect.TypeFor[boot.CreateTraceSessionArgs](),
	reflect.TypeFor[boot.DeleteTraceSessionArgs](),
	reflect.TypeFor[boot.EventOut](),
	reflect.TypeFor[boot.ExecResult](),
	reflect.TypeFor[boot.FSSaveArgs](),
	reflect.TypeFor[boot.InitPluginStackArgs](),
	reflect.TypeFor[boot.MountArgs](),
	reflect.TypeFor[boot.PortForwardOpts](),
	reflect.TypeFor[boot.ProcessesResult](),
	reflect.TypeFor[boot.ProcfsResult](),
	reflect.TypeFor[boot.RuntimeStateResult](),
	reflect.TypeFor[boot.Savings](),
	reflect.TypeFor[boot.SignalArgs](),
	reflect.TypeFor[boot.Stacks](),
	reflect.TypeFor[boot.StartArgs](),
	reflect.TypeFor[boot.TraceSessionsResult](),
	reflect.TypeFor[boot.WaitFSRestoreArgs](),
	reflect.TypeFor[boot.WaitPIDArgs](),

	// runsc/container.
	reflect.TypeFor[container.OpenMountArgs](),
	reflect.TypeFor[container.OpenMountResult](),
}

func main() {
	dir := os.Getenv("BUILD_WORKSPACE_DIRECTORY")
	if dir == "" {
		fmt.Fprintln(os.Stderr, "zap: run me with bazel run //tools/zap")
		os.Exit(1)
	}

	// One root at a time, so a refusal names the payload that carries it
	// instead of stopping the run at the first one.
	var take []reflect.Type
	var refused []string
	for _, r := range roots {
		if _, err := zip.Layouts(r); err != nil {
			refused = append(refused, fmt.Sprintf("%s.%s: %s", short(r.PkgPath()), r.Name(), trim(err)))
			continue
		}
		take = append(take, r)
	}

	layouts, err := zip.Layouts(take...)
	if err != nil {
		fmt.Fprintln(os.Stderr, "zap:", err)
		os.Exit(1)
	}

	var wrote, outside []string
	for _, l := range layouts {
		rel, ok := strings.CutPrefix(l.Path, module)
		if !ok {
			outside = append(outside, fmt.Sprintf("%s: another module owns it (%s)", l.Path, strings.Join(l.Types, ", ")))
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			outside = append(outside, fmt.Sprintf("%s: no source here, bazel writes it (%s)", rel, strings.Join(l.Types, ", ")))
			continue
		}
		path := filepath.Join(dir, rel, "zap.go")
		if err := os.WriteFile(path, append([]byte(license), l.Source...), 0644); err != nil {
			fmt.Fprintln(os.Stderr, "zap:", err)
			os.Exit(1)
		}
		wrote = append(wrote, fmt.Sprintf("%s (%d types)", rel, len(l.Types)))
	}

	sort.Strings(wrote)
	for _, w := range wrote {
		fmt.Println("wrote", w)
	}
	if len(outside) > 0 {
		sort.Strings(outside)
		fmt.Fprintln(os.Stderr, "\nreached, but not ours to write:")
		for _, o := range outside {
			fmt.Fprintln(os.Stderr, " ", o)
		}
	}
	if len(refused) > 0 {
		sort.Strings(refused)
		fmt.Fprintln(os.Stderr, "\nno layout:")
		for _, r := range refused {
			fmt.Fprintln(os.Stderr, " ", r)
		}
		os.Exit(1)
	}
}

// short drops the module prefix from an import path so a verdict reads as the
// package a reader would open.
func short(path string) string {
	if rel, ok := strings.CutPrefix(path, module); ok {
		return rel
	}
	return path
}

// trim drops the emitter's own prefix from a refusal: the reader already knows
// which generator is speaking.
func trim(err error) string {
	return strings.TrimPrefix(err.Error(), "zip: ")
}

const license = `// Copyright 2026 The gVisor Authors.
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

`
