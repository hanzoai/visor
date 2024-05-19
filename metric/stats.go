// Copyright 2024 The casbin Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package metric

import (
	"bufio"
	"strconv"
	"strings"
	"sync"

	"github.com/casvisor/casvisor/util/term"
	"golang.org/x/crypto/ssh"
)

type FsInfo struct {
	MountPoint string `json:"mountPoint"`
	Used       uint64 `json:"used"`
	Free       uint64 `json:"free"`
}

type Network struct {
	Ipv4 string `json:"ipv4"`
	Ipv6 string `json:"ipv6"`
	Rx   uint64 `json:"rx"`
	Tx   uint64 `json:"tx"`
}

type cpuRaw struct {
	User    int64 // time spent in user mode
	Nice    int64 // time spent in user mode with low priority (nice)
	System  int64 // time spent in system mode
	Idle    int64 // time spent in the idle task
	IoWait  int64 // time spent waiting for I/O to complete (since Linux 2.5.41)
	Irq     int64 // time spent servicing  interrupts  (since  2.6.0-test4)
	SoftIrq int64 // time spent servicing softirqs (since 2.6.0-test4)
	Steal   int64 // time spent in other OSes when running in a virtualized environment
	Guest   int64 // time spent running a virtual Cpu for guest operating systems under the control of the Linux kernel.
	Total   int64 // total of all time fields
}

type CpuInfo struct {
	User    float32 `json:"user"`
	Nice    float32 `json:"nice"`
	System  float32 `json:"system"`
	Idle    float32 `json:"idle"`
	IoWait  float32 `json:"ioWait"`
	Irq     float32 `json:"irq"`
	SoftIrq float32 `json:"softIrq"`
	Steal   float32 `json:"steal"`
	Guest   float32 `json:"guest"`
	CoreNum int     `json:"coreNum"`
}

type Stat struct {
	Uptime         int64              `json:"uptime"`
	Hostname       string             `json:"hostname"`
	Load1          string             `json:"load1"`
	Load5          string             `json:"load5"`
	Load10         string             `json:"load10"`
	RunningProcess string             `json:"runningProcess"`
	TotalProcess   string             `json:"totalProcess"`
	MemTotal       int64              `json:"memTotal"`
	MemFree        int64              `json:"memFree"`
	MemBuffers     int64              `json:"memBuffers"`
	MemAvailable   int64              `json:"memAvailable"`
	MemCached      int64              `json:"memCached"`
	SwapTotal      int64              `json:"swapTotal"`
	SwapFree       int64              `json:"swapFree"`
	FsInfos        []FsInfo           `json:"fsInfos"`
	Network        map[string]Network `json:"network"`
	Cpu            CpuInfo            `json:"cpu"`
}

func GetAllStat(client *ssh.Client, stats *Stat) (*Stat, error) {
	runner := Runner{}

	runner.Add(func() error {
		return getMemInfo(client, stats)
	})
	runner.Add(func() error {
		return getFsInfo(client, stats)
	})

	runner.Add(func() error {
		return getCpu(client, stats)
	})

	runner.Wait()
	return stats, nil
}

func getMemInfo(client *ssh.Client, stats *Stat) error {
	lines, err := term.RunCommand(client, "/bin/cat /proc/meminfo")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(lines))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 3 {
			val, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				continue
			}
			val *= 1024
			switch parts[0] {
			case "MemTotal:":
				stats.MemTotal = val
			case "MemFree:":
				stats.MemFree = val
			case "MemAvailable:":
				stats.MemAvailable = val
			case "Buffers:":
				stats.MemBuffers = val
			case "Cached:":
				stats.MemCached = val
			case "SwapTotal:":
				stats.SwapTotal = val
			case "SwapFree:":
				stats.SwapFree = val
			}
		}
	}

	return nil
}

func getFsInfo(client *ssh.Client, stats *Stat) error {
	lines, err := term.RunCommand(client, "/bin/df -B1")
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(lines))
	flag := 0
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Fields(line)
		n := len(parts)
		dev := n > 0 && strings.Index(parts[0], "/dev/") == 0
		if n == 1 && dev {
			flag = 1
		} else if (n == 5 && flag == 1) || (n == 6 && dev) {
			i := flag
			flag = 0
			used, err := strconv.ParseUint(parts[2-i], 10, 64)
			if err != nil {
				continue
			}
			free, err := strconv.ParseUint(parts[3-i], 10, 64)
			if err != nil {
				continue
			}
			stats.FsInfos = append(stats.FsInfos, FsInfo{
				parts[5-i], used, free,
			})
		}
	}

	return nil
}

func parseCpuFields(fields []string, stat *cpuRaw) {
	numFields := len(fields)
	for i := 1; i < numFields; i++ {
		val, err := strconv.ParseInt(fields[i], 10, 64)
		if err != nil {
			continue
		}

		stat.Total += val
		switch i {
		case 1:
			stat.User = val
		case 2:
			stat.Nice = val
		case 3:
			stat.System = val
		case 4:
			stat.Idle = val
		case 5:
			stat.IoWait = val
		case 6:
			stat.Irq = val
		case 7:
			stat.SoftIrq = val
		case 8:
			stat.Steal = val
		case 9:
			stat.Guest = val
		}
	}
}

// PreCPUMap stores the previous Cpu stats for each client
var preCpuMap sync.Map

func getCpu(client *ssh.Client, stats *Stat) error {
	lines, err := term.RunCommand(client, "/bin/cat /proc/stat")
	if err != nil {
		return err
	}

	var (
		nowCPU cpuRaw
		total  float32
	)

	scanner := bufio.NewScanner(strings.NewReader(lines))
	core := 0
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == "cpu" { // changing here if want to get every cpu-core's stats
			parseCpuFields(fields, &nowCPU)
			continue
		}
		if len(fields) > 0 && strings.HasPrefix(fields[0], "cpu") {
			core++
		}
	}
	stats.Cpu.CoreNum = core

	// Fetch the previous Cpu stats for this client
	value, ok := preCpuMap.Load(client)
	var preCPU cpuRaw
	if ok {
		preCPU = value.(cpuRaw)
	} else {
		// If this is the first time, initialize preCPU
		preCPU = nowCPU
		preCpuMap.Store(client, preCPU)
		goto END
	}

	total = float32(nowCPU.Total - preCPU.Total)
	stats.Cpu.User = float32(nowCPU.User-preCPU.User) / total * 100
	stats.Cpu.Nice = float32(nowCPU.Nice-preCPU.Nice) / total * 100
	stats.Cpu.System = float32(nowCPU.System-preCPU.System) / total * 100
	stats.Cpu.Idle = float32(nowCPU.Idle-preCPU.Idle) / total * 100
	stats.Cpu.IoWait = float32(nowCPU.IoWait-preCPU.IoWait) / total * 100
	stats.Cpu.Irq = float32(nowCPU.Irq-preCPU.Irq) / total * 100
	stats.Cpu.SoftIrq = float32(nowCPU.SoftIrq-preCPU.SoftIrq) / total * 100
	stats.Cpu.Guest = float32(nowCPU.Guest-preCPU.Guest) / total * 100

END:
	// Store the new Cpu stats for this client
	preCpuMap.Store(client, nowCPU)
	return nil
}

func CleanupPreCpuMap() {
	preCpuMap.Range(func(key, value interface{}) bool {
		preCpuMap.Delete(key)
		return true
	})
}
