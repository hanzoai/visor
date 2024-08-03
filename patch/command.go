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
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"unicode/utf8"
)

type Patch struct {
	Name           string `json:"name"`
	Category       string `json:"category"`
	Title          string `json:"title"`
	Url            string `json:"url"`
	Size           string `json:"size"`
	ExpectedStatus string `json:"expectedStatus"`
	Status         string `json:"status"`
	InstallTime    string `json:"installTime"`
	Message        string `json:"message"`
}

func decodePowerShellOutput(input []byte) (string, error) {
	if utf8Output := string(input); strings.HasPrefix(utf8Output, "\uFEFF") {
		return strings.TrimPrefix(utf8Output, "\uFEFF"), nil
	} else if utf8.Valid(input) {
		return utf8Output, nil
	}
	return "", fmt.Errorf("the console output fails UTF-8 encoding")
}

func executePowerShellCommand(command string) (string, error) {
	cmd := exec.Command("powershell", "-Command",
		"$OutputEncoding = [Console]::OutputEncoding = [Text.Encoding]::UTF8; "+
			"$PSDefaultParameterValues['*:UICulture'] = 'en-US'; "+
			"$PSDefaultParameterValues['*:Culture'] = 'en-US'; "+command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		decodedError, err := decodePowerShellOutput(stderr.Bytes())
		if err != nil {
			return "", fmt.Errorf("decode error: %v", err)
		}
		return "", fmt.Errorf("execution error: %v\nStandard Error Output: %v", err, decodedError)
	}

	//  Decode and return the output, removing the line breaks at the end
	decodedOutput, err := decodePowerShellOutput(stdout.Bytes())
	if err != nil {
		return "", fmt.Errorf("decode output failure: %v", err)
	}
	return strings.TrimSpace(decodedOutput), nil
}

func checkAndInstallModule() error {
	checkCmd := `if (Get-PackageProvider -ListAvailable -Name NuGet) { 
			Write-Output 'Exists' 
		} else { 
			Write-Output 'NotExists' 
		}`
	checkOutput, err := executePowerShellCommand(checkCmd)
	if err != nil {
		return err
	}

	if strings.Contains(checkOutput, "NotExists") {
		installCmd := `Install-PackageProvider -Name NuGet -MinimumVersion 2.8.5.201 -Force`
		_, err := executePowerShellCommand(installCmd)
		if err != nil {
			return err
		}
	}

	checkCmd = `if (Get-Module -ListAvailable -Name PSWindowsUpdate) {
			Write-Output 'Exists'
		} else {
			Write-Output 'NotExists'
		}`
	checkOutput, err = executePowerShellCommand(checkCmd)
	if err != nil {
		return err
	}

	if strings.Contains(checkOutput, "NotExists") {
		installCmd := `Install-Module -Name PSWindowsUpdate -Force -SkipPublisherCheck`
		_, err := executePowerShellCommand(installCmd)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkIfKbArticle(input string) bool {
	re := regexp.MustCompile(`^KB.*\d$`)
	return re.MatchString(input)
}

func installPatch(kb string) (string, error) {
	if !checkIfKbArticle(kb) {
		return "", fmt.Errorf("not have a valid KB article, please install manually")
	}
	err := checkAndInstallModule()
	if err != nil {
		return "", err
	}
	updateCmd := fmt.Sprintf(`Get-WindowsUpdate -KBArticleID %s -Install -AcceptAll -AutoReboot -ErrorAction Stop`, kb)

	output, err := executePowerShellCommand(updateCmd)
	if err != nil {
		return "", err
	}
	return output, nil
}

func uninstallPatch(kb string) (string, error) {
	if checkIfKbArticle(kb) {
		err := checkAndInstallModule()
		if err != nil {
			return "", err
		}
		stopCmd := fmt.Sprintf(`Import-Module PSWindowsUpdate; Remove-WindowsUpdate -KBArticleID %s`, kb)
		out, err := executePowerShellCommand(stopCmd)
		if err != nil {
			return "", err
		}
		return out, nil
	}
	return "", fmt.Errorf("not have a valid KB article, please uninstall manually")
}

func mergeTwoPatches(oldpatch *Patch, newPatch *Patch) (*Patch, error) {
	mergedpatch := newPatch
	if mergedpatch.Name == "" {
		mergedpatch.Name = oldpatch.Name
	}
	if mergedpatch.Category == "" {
		mergedpatch.Category = oldpatch.Category
	}
	if mergedpatch.Title == "" {
		mergedpatch.Title = oldpatch.Title
	}
	if mergedpatch.Url == "" {
		mergedpatch.Url = oldpatch.Url
	}
	if mergedpatch.Size == "" {
		mergedpatch.Size = oldpatch.Size
	}
	if mergedpatch.ExpectedStatus == "" {
		mergedpatch.ExpectedStatus = oldpatch.ExpectedStatus
	}
	if mergedpatch.Status == "" {
		mergedpatch.Status = oldpatch.Status
	}
	if mergedpatch.Message == "" {
		mergedpatch.Message = oldpatch.Message
	}
	if mergedpatch.InstallTime == "" {
		mergedpatch.InstallTime = oldpatch.InstallTime
	}
	return mergedpatch, nil
}

func getPatches() ([]*Patch, error) {
	err := checkAndInstallModule()
	if err != nil {
		return nil, err
	}
	PatchesInfoCmd := `
	# Get—Installed patches
	$updates = Get-WUHistory | Where-Object { $_.Date -gt $oneYearAgo } | Select-Object -Property Title, UpdateType, Date, Result, KB
	$installedPatches = $updates | ForEach-Object {
		[PSCustomObject]@{
		   Name        = if ($_.KB) { $_.KB } else { $_.Title }
		   Category    = $_.UpdateType
		   Title       = $_.Title
		   Url         = 'N/A'
		   Size        = 'N/A'
           ExpectedStatus = ''
		   Status      = if ($_.Result -eq 'Succeeded') { 'Installed' } else { 'Failed' }
		   InstallTime = if ($_.Result -eq 'Succeeded') { Get-Date $_.Date -Format 'yyyy-MM-dd HH:mm:ss' } else { $null }
		   Message     = if ($_.Result -eq 'Succeeded') { $null } else { 'Installation failed' }
		}
	}
	# Get Uninstalled patches
	Import-Module PSWindowsUpdate
	$availableUpdates = Get-WindowsUpdate
	$availablePatches = $availableUpdates | ForEach-Object {
		[PSCustomObject]@{
		   Name        = if ($_.KB) { $_.KB } else { $_.Title }
		   Category    = $_.Categories
		   Title       = $_.Title
		   Url         = $_.MoreInfoUrls
		   Size        = $_.Size
           ExpectedStatus = ''
		   Status      = 'Uninstall'
		   InstallTime = $null
		   Message     = $null
		}
	}
	
	# merge the results
	$allPatches = $installedPatches + $availablePatches
	
	# convert to JSON and output
	$json = $allPatches | ConvertTo-Json -Depth 3
	Write-Output $json
	`
	patchOutput, err := executePowerShellCommand(PatchesInfoCmd)

	// contrast the output
	jsonStart := strings.Index(patchOutput, "[")
	jsonEnd := strings.LastIndex(patchOutput, "]")

	if jsonStart == -1 || jsonEnd == -1 || jsonStart > jsonEnd {
		return nil, fmt.Errorf("no JSON array found in the input")
	}

	patchesOutput := patchOutput[jsonStart : jsonEnd+1]
	jsonOutput := strings.ReplaceAll(patchesOutput, "\r\n", "")
	var newPatch []*Patch
	err = json.Unmarshal([]byte(jsonOutput), &newPatch)
	if err != nil {
		return nil, err
	}
	var patchesOut []*Patch
	for _, patchItem := range newPatch {
		if patchItem.Name != "" {
			patchesOut = append(patchesOut, patchItem)
		}
	}
	return patchesOut, nil
}
