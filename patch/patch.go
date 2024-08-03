// Copyright 2023 The casbin Authors. All Rights Reserved.
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

package patch

import (
	"fmt"
	"strings"
)

func UpdatePatches(patches []*Patch) error {
	for _, patchItem := range patches {
		if patchItem.ExpectedStatus == "Install" && patchItem.Status == "Uninstall" {
			err := InstallPatch(patchItem)
			if err != nil {
				return err
			}
		}
		if patchItem.ExpectedStatus == "Uninstall" && patchItem.Status == "Install" {
			err := UninstallPatch(patchItem)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func RefreshPatches(patches []*Patch) ([]*Patch, error) {
	newPatches, err := getPatches()
	if err != nil {
		return nil, err
	}
	patchMap := make(map[string]*Patch)
	for _, patchItem := range patches {
		patchMap[patchItem.Name] = patchItem
	}

	for _, newPatch := range newPatches {
		oldpatch, ok := patchMap[newPatch.Name]
		if ok {
			mergedpatch, err := mergeTwoPatches(oldpatch, newPatch)
			if err != nil {
				return nil, err
			}
			patchMap[newPatch.Name] = mergedpatch
		} else {
			patchMap[newPatch.Name] = newPatch
		}
	}
	var outputPatches []*Patch
	for _, patchItem := range patchMap {
		outputPatches = append(newPatches, patchItem)
	}

	return outputPatches, nil
}

func InstallPatch(patchItem *Patch) error {
	output, err := installPatch(patchItem.Name)
	if err != nil {
		return err
	}
	if strings.Contains(output, "Failed") {
		patchItem.Status = "Failed"
		patchItem.Message = fmt.Sprintf("update failure: %v", err)
		return fmt.Errorf("update failure: %v", err)
	} else {
		patchItem.Status = "Installed"
	}
	return err
}

func UninstallPatch(patchItem *Patch) error {
	out, err := uninstallPatch(patchItem.Name)
	if err != nil {
		return err
	}
	if strings.Contains(out, "Failed") {
		patchItem.Status = "Failed"
		patchItem.Message = fmt.Sprintf("uninstall failure: %v", err)
		return fmt.Errorf("uninstall failure: %v", err)
	} else {
		patchItem.Status = "Uninstall"
	}
	return nil
}
