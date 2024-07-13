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

package object

import (
	"time"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/mem"
)

type SystemInfo struct {
	CpuUsage    []float64 `json:"cpuUsage"`
	MemoryUsed  uint64    `json:"memoryUsed"`
	MemoryTotal uint64    `json:"memoryTotal"`
	DiskUsed    uint64    `json:"diskUsed"`
	DiskTotal   uint64    `json:"diskTotal"`
}

// getCpuUsage get cpu usage
func getCpuUsage() ([]float64, error) {
	usage, err := cpu.Percent(time.Second, true)
	return usage, err
}

// getMemoryUsage get memory usage
func getMemoryUsage() (uint64, uint64, error) {
	virtualMem, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}

	return virtualMem.Used, virtualMem.Total, nil
}

func getDiskUsage() (uint64, uint64, error) {
	usage, err := disk.Usage("/")
	if err != nil {
		return 0, 0, err
	}

	return usage.Used, usage.Total, nil
}

func GetSystemInfo() (*SystemInfo, error) {
	cpuUsage, err := getCpuUsage()
	if err != nil {
		return nil, err
	}

	memoryUsed, memoryTotal, err := getMemoryUsage()
	if err != nil {
		return nil, err
	}

	diskUsed, diskTotal, err := getDiskUsage()

	res := &SystemInfo{
		CpuUsage:    cpuUsage,
		MemoryUsed:  memoryUsed,
		MemoryTotal: memoryTotal,
		DiskUsed:    diskUsed,
		DiskTotal:   diskTotal,
	}
	return res, nil
}
