// Encryption, decryption functions in this file have been picked up from
// https://github.com/mattermost/mattermost-plugin-github

package main

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

func pad(src []byte) []byte {
	padding := aes.BlockSize - len(src)%aes.BlockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(src, padtext...)
}

func unpad(src []byte) ([]byte, error) {
	length := len(src)
	unpadding := int(src[length-1])

	if unpadding > length {
		return nil, errors.New("unpad error. This could happen when incorrect encryption key is used")
	}

	return src[:(length - unpadding)], nil
}

func encrypt(key []byte, text string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	msg := pad([]byte(text))
	ciphertext := make([]byte, aes.BlockSize+len(msg))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", err
	}

	cfb := cipher.NewCFBEncrypter(block, iv)
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], msg)
	finalMsg := base64.URLEncoding.EncodeToString(ciphertext)
	return finalMsg, nil
}

func decrypt(key []byte, text string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	decodedMsg, err := base64.URLEncoding.DecodeString(text)
	if err != nil {
		return "", err
	}

	if (len(decodedMsg) % aes.BlockSize) != 0 {
		return "", errors.New("blocksize must be multiple of decoded message length")
	}

	iv := decodedMsg[:aes.BlockSize]
	msg := decodedMsg[aes.BlockSize:]

	cfb := cipher.NewCFBDecrypter(block, iv)
	cfb.XORKeyStream(msg, msg)

	unpadMsg, err := unpad(msg)
	if err != nil {
		return "", err
	}

	return string(unpadMsg), nil
}

// parseBuildParameters checks if the parameters are valid and returns multiple values.
// The first return value is considered as job name.
// The second return value is considered as build number (when applicable).
// The third return value is a map containing the key=value parameters.
// The last boolean return value indicates if the parsing was successful.
func parseBuildParameters(parameters []string) (string, string, map[string]string, bool) {
	if len(parameters) == 0 {
		return "", "", nil, false
	}

	// Handle the job name (which might be quoted with spaces)
	jobNameParts := []string{}
	paramIndex := 0

	// Collect job name parts until we find a closing quote or a non-quoted parameter
	inQuotes := false
	if len(parameters) > 0 && strings.HasPrefix(parameters[0], "\"") {
		inQuotes = true
		// Remove the starting quote
		parameters[0] = strings.TrimPrefix(parameters[0], "\"")

		if strings.HasSuffix(parameters[0], "\"") {
			// Handle case: "job name"
			parameters[0] = strings.TrimSuffix(parameters[0], "\"")
			jobNameParts = append(jobNameParts, parameters[0])
			paramIndex = 1
			inQuotes = false
		} else {
			// Start collecting job name parts
			jobNameParts = append(jobNameParts, parameters[0])
			paramIndex = 1
		}
	} else if len(parameters) > 0 {
		// Simple non-quoted job name
		jobNameParts = append(jobNameParts, parameters[0])
		paramIndex = 1
	}

	// Continue collecting job name parts if we're in quotes
	for paramIndex < len(parameters) && inQuotes {
		if strings.HasSuffix(parameters[paramIndex], "\"") {
			// Found closing quote
			parameters[paramIndex] = strings.TrimSuffix(parameters[paramIndex], "\"")
			jobNameParts = append(jobNameParts, parameters[paramIndex])
			paramIndex++
			inQuotes = false
		} else {
			// Still in quotes
			jobNameParts = append(jobNameParts, parameters[paramIndex])
			paramIndex++
		}
	}

	jobName := strings.Join(jobNameParts, " ")

	// Process build number and parameters
	buildNumber := ""
	var paramMap map[string]string

	// Check if the next parameter is a build number (numeric)
	if paramIndex < len(parameters) && isNumeric(parameters[paramIndex]) {
		buildNumber = parameters[paramIndex]
		paramIndex++
	}

	// Process key=value parameters
	for i := paramIndex; i < len(parameters); i++ {
		if strings.Contains(parameters[i], "=") {
			parts := strings.SplitN(parameters[i], "=", 2)
			if len(parts) == 2 {
				if paramMap == nil {
					paramMap = make(map[string]string)
				}
				paramMap[parts[0]] = parts[1]
			}
		}
	}

	return jobName, buildNumber, paramMap, true
}

// Helper function to check if a string is numeric
func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

func generateSlackAttachment(text string) *model.SlackAttachment {
	slackAttachment := &model.SlackAttachment{
		Text:  text,
		Color: "#7FC1EE",
	}
	return slackAttachment
}
