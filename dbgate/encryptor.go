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
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
)

const (
	MinKeyLength = 16
)

type Encryptor struct {
	key        []byte
	verifyHmac bool
}

func NewEncryptor(key string) (*Encryptor, error) {
	if len(key) < MinKeyLength {
		return nil, fmt.Errorf("key must be at least %d characters long", MinKeyLength)
	}

	cryptoKey := sha256.Sum256([]byte(key))

	return &Encryptor{
		key:        cryptoKey[:],
		verifyHmac: true,
	}, nil
}

func (e *Encryptor) hmac(text []byte) []byte {
	mac := hmac.New(sha256.New, e.key)
	mac.Write(text)
	return mac.Sum(nil)
}

func (e *Encryptor) encrypt(obj interface{}) (string, error) {
	var data []byte
	if str, ok := obj.(string); ok {
		// quote string to make it a valid JSON in dbgate
		data = []byte(strconv.Quote(str))
	} else {
		var err error
		data, err = json.Marshal(obj)
		if err != nil {
			return "", err
		}
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	// PKCS#7 padding
	blockSize := block.BlockSize()
	padding := blockSize - (len(data) % blockSize)
	paddedData := append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)

	iv := make([]byte, blockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	ciphertext := make([]byte, len(paddedData))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, paddedData)

	encryptedJSON := base64.StdEncoding.EncodeToString(ciphertext)

	result := hex.EncodeToString(iv) + encryptedJSON

	if e.verifyHmac {
		result = hex.EncodeToString(e.hmac([]byte(result))) + result
	}

	return result, nil
}

func (e *Encryptor) decrypt(cipherText string) (interface{}, error) {
	if cipherText == "" {
		return nil, nil
	}

	if e.verifyHmac {
		expectedHmac, err := hex.DecodeString(cipherText[:64])
		if err != nil {
			return nil, err
		}

		cipherText = cipherText[64:]

		actualHmac := e.hmac([]byte(cipherText))
		if !hmac.Equal(actualHmac, expectedHmac) {
			return nil, errors.New("HMAC does not match")
		}
	}

	iv, err := hex.DecodeString(cipherText[:32])
	if err != nil {
		return nil, err
	}

	ciphertext, err := base64.StdEncoding.DecodeString(cipherText[32:])
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}

	decrypted := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(decrypted, ciphertext)

	// get the real length of the decrypted data
	padding := decrypted[len(decrypted)-1]
	decrypted = decrypted[:len(decrypted)-int(padding)]

	var obj interface{}
	err = json.Unmarshal(decrypted, &obj)
	if err != nil {
		return nil, err
	}

	return obj, nil
}
