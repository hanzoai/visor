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

package vault

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/beego/beego"
)

type Client struct {
	Address string
	Token   string
}

func initVaultClient() (*Client, error) {
	vaultEndpoint := beego.AppConfig.String("vaultEndpoint")
	vaultToken := beego.AppConfig.String("vaultToken")

	if vaultEndpoint == "" {
		return nil, nil
	}

	client := &Client{
		Address: vaultEndpoint,
		Token:   vaultToken,
	}

	// Check and configure transit secrets engine and key
	if err := client.ensureTransitKey(); err != nil {
		return nil, err
	}

	return client, nil
}

func (client *Client) ensureTransitKey() error {
	// Enable transit secrets engine if not enabled
	if err := client.enableTransitEngine(); err != nil {
		return err
	}

	// Create transit key if not created
	if err := client.createTransitKey("casvisor"); err != nil {
		return err
	}

	return nil
}

func (client *Client) enableTransitEngine() error {
	url := fmt.Sprintf("%s/v1/sys/mounts/transit", client.Address)

	req, err := http.NewRequest("POST", url, strings.NewReader(`{"type": "transit"}`))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("X-Vault-Token", client.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("failed to enable transit secrets engine: %v", err)
		}
	}
	defer resp.Body.Close()

	return nil
}

func (client *Client) createTransitKey(keyName string) error {
	url := fmt.Sprintf("%s/v1/transit/keys/%s", client.Address, keyName)

	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("X-Vault-Token", client.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("failed to create transit key: %v", err)
		}
	}
	defer resp.Body.Close()

	return nil
}

func (client *Client) write(path string, data map[string]interface{}) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/v1/%s", client.Address, path)
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %v", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}

	req.Header.Set("X-Vault-Token", client.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform HTTP request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected response code: %d", resp.StatusCode)
	}

	var responseData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&responseData); err != nil {
		return nil, fmt.Errorf("failed to decode response data: %v", err)
	}

	return responseData, nil
}

func (client *Client) encryptPassword(plaintext string) (string, error) {
	// Convert plaintext to base64
	plaintextBase64 := base64.StdEncoding.EncodeToString([]byte(plaintext))
	data := map[string]interface{}{
		"plaintext": plaintextBase64,
	}

	secret, err := client.write("transit/encrypt/casvisor", data)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt data: %v", err)
	}

	secretData, ok := secret["data"].(map[string]interface{})
	if !ok {
		return "", errors.New("unexpected response data format")
	}

	ciphertext, ok := secretData["ciphertext"].(string)
	if !ok {
		return "", errors.New("ciphertext not found in response")
	}

	return ciphertext, nil
}

func (client *Client) decryptPassword(ciphertext string) (string, error) {
	// Supporting scenarios where Vault is not used
	re := regexp.MustCompile(`^vault.*`)
	if !re.MatchString(ciphertext) {
		return ciphertext, nil
	}
	data := map[string]interface{}{
		"ciphertext": ciphertext,
	}

	secret, err := client.write("transit/decrypt/casvisor", data)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt data: %v", err)
	}

	secretData, ok := secret["data"].(map[string]interface{})
	if !ok {
		return "", errors.New("unexpected response data format")
	}

	plaintextBase64, ok := secretData["plaintext"].(string)
	if !ok {
		return "", errors.New("plaintext not found in response")
	}
	// Decode the base64-encoded plaintext
	plaintext, err := base64.StdEncoding.DecodeString(plaintextBase64)
	if err != nil {
		return "", fmt.Errorf("failed to decode plaintext: %v", err)
	}

	return string(plaintext), nil
}

func GetEncryptedPassword(password string) (string, error) {
	vaultClient, err := initVaultClient()
	if err != nil || vaultClient == nil {
		return password, err
	}

	encryptedPassword, err := vaultClient.encryptPassword(password)
	if err != nil {
		return password, err
	}

	return encryptedPassword, nil
}

func GetDecryptedPassword(password string) (string, error) {
	vaultClient, err := initVaultClient()
	if err != nil || vaultClient == nil {
		return password, err
	}

	decryptedPassword, err := vaultClient.decryptPassword(password)
	if err != nil {
		return password, err
	}

	return decryptedPassword, nil
}
