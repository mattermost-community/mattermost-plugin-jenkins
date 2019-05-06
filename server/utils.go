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
	"github.com/mattermost/mattermost-server/model"
	"io"
	"regexp"
	"strings"
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
	cfb.XORKeyStream(ciphertext[aes.BlockSize:], []byte(msg))
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
		return "", errors.New("blocksize must be multipe of decoded message length")
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

// parseBuildParameters checks if the parameters are valid and returns three values.
// The first return value is considered as job name and the second value is considered as build number.
// Examples of valid parameters as per the regex:
// 1. jobname OR folder/jobname
// 2. jobname 22 OR folder/jobname 22
// 3. "jobname" OR "folder/jobname"
// 4. "jobname" 22 OR "folder/jobname" 22
// 5. "job name with space" OR "folder/job name with space" OR "folder with space/job name with space"
// 6. "job name with space" 22 OR "folder with space/job name with space" 22
func parseBuildParameters(parameters []string) (string, string, bool) {
	paramString := strings.Join(parameters, " ")
	if len(parameters) == 1 {
		if strings.HasPrefix(paramString, "\"") && strings.HasSuffix(paramString, "\"") {
			tempString := strings.TrimLeft(strings.TrimRight(paramString, `\"`), `\"`)
			return tempString, "", true
		}
		return paramString, "", true
	}
	regex, _ := regexp.Compile(`("[^"]*"|[^"\s]+)\s*(\w*)`)
	submatches := regex.FindAllStringSubmatch(paramString, -1)
	if len(submatches) == 0 || len(submatches) > 1 {
		return "", "", false
	}
	return strings.TrimLeft(strings.TrimRight(submatches[0][1], `\"`), `\"`), submatches[0][2], true
}

func generateSlackAttachment(text string) *model.SlackAttachment {
	slackAttachment := &model.SlackAttachment{
		Text:  text,
		Color: "#7FC1EE",
	}
	return slackAttachment
}
