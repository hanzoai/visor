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
	"fmt"
	"os"

	"github.com/casvisor/casvisor/patch"
)

func isHostSelf(asset *Asset) (bool, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return false, err
	}
	return asset.Name == hostname, nil
}

func deepCopyPatches(patchesIn []*Patch) []*patch.Patch {
	var outputPatch []*patch.Patch
	for _, patchIn := range patchesIn {
		patchOut := &patch.Patch{
			Category:       patchIn.Category,
			ExpectedStatus: patchIn.ExpectedStatus,
			InstallTime:    patchIn.InstallTime,
			Message:        patchIn.Message,
			Name:           patchIn.Name,
			Size:           patchIn.Size,
			Status:         patchIn.Status,
			Title:          patchIn.Title,
			Url:            patchIn.Url,
		}
		outputPatch = append(outputPatch, patchOut)
	}
	return outputPatch
}

func deepCopyPatches2(patchses []*patch.Patch) []*Patch {
	var outputPatch []*Patch
	for _, patchIn := range patchses {
		patchOut := &Patch{
			Category:       patchIn.Category,
			ExpectedStatus: patchIn.ExpectedStatus,
			InstallTime:    patchIn.InstallTime,
			Message:        patchIn.Message,
			Name:           patchIn.Name,
			Size:           patchIn.Size,
			Status:         patchIn.Status,
			Title:          patchIn.Title,
			Url:            patchIn.Url,
		}
		outputPatch = append(outputPatch, patchOut)
	}
	return outputPatch
}

func UpdatePatches(asset *Asset) error {
	ifHostname, err := isHostSelf(asset)
	if err != nil {
		return err
	}
	if !ifHostname {
		return fmt.Errorf("hostname not match")
	}
	patches := asset.Patches
	err = patch.UpdatePatches(deepCopyPatches(patches))
	if err != nil {
		return err
	}
	return nil
}

func RefreshPatches(asset *Asset) error {
	ifHostname, err := isHostSelf(asset)
	if err != nil {
		return err
	}
	if !ifHostname {
		return fmt.Errorf("hostname not match")
	}
	patches := asset.Patches
	outputPatches, err := patch.RefreshPatches(deepCopyPatches(patches))
	if err != nil {
		return err
	}
	asset.Patches = deepCopyPatches2(outputPatches)
	return nil
}
