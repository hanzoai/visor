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

package dbgate

import (
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
)

const (
	defaultEncryptionKey = "mQAUaXhavRGJDxDTXSCg7Ej0xMmGCrx6OKA07DIMBiDcYYkvkaXjTAzPUEHEHEf9"
	keyFileName          = ".key"
)

var (
	encryptionKey string
	encryptor     *Encryptor
)

type keyData struct {
	EncryptionKey string `json:"encryptionKey"`
}

func loadEncryptionKey() (string, error) {
	if encryptionKey != "" {
		return encryptionKey, nil
	}

	encryptor, _ := NewEncryptor(defaultEncryptionKey)
	keyFile := filepath.Join(dataDir(), keyFileName)

	if _, err := os.Stat(keyFile); os.IsNotExist(err) {
		newKey := make([]byte, 32)
		_, err := rand.Read(newKey)
		if err != nil {
			return "", err
		}

		result := keyData{EncryptionKey: string(newKey)}
		encryptedKey, err := encryptor.encrypt(result)
		if err != nil {
			return "", err
		}

		err = os.WriteFile(keyFile, []byte(encryptedKey), 0o644)
		if err != nil {
			return "", err
		}
	}

	encryptedData, err := os.ReadFile(keyFile)
	if err != nil {
		return "", err
	}

	decryptedData, err := encryptor.decrypt(string(encryptedData))
	if err != nil {
		return "", err
	}
	dataMap, ok := decryptedData.(map[string]interface{})
	if !ok {
		return "", errors.New("failed to parse decrypted data")
	}

	var data keyData
	data.EncryptionKey, ok = dataMap["encryptionKey"].(string)
	if !ok {
		return "", errors.New("failed to parse encryptionKey")
	}

	encryptionKey = data.EncryptionKey

	return encryptionKey, nil
}

func getEncryptor() (*Encryptor, error) {
	if encryptor != nil {
		return encryptor, nil
	}

	key, err := loadEncryptionKey()
	if err != nil {
		return nil, err
	}

	encryptor, _ = NewEncryptor(key)
	if err != nil {
		return nil, err
	}

	return encryptor, nil
}
