package main

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/mattermost/mattermost-server/plugin"
)

const (
	jenkinsUsername = "Jenkins Plugin"
	jenkinsTokenKey = "_jenkinsToken"
)

type Plugin struct {
	plugin.MattermostPlugin

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration
}

type JenkinsUserInfo struct {
	Username string
	Token    string
}

func (p *Plugin) OnActivate() error {
	p.API.RegisterCommand(getCommand())
	return nil
}

func (p *Plugin) storeJenkinsUserInfo(info *JenkinsUserInfo) error {
	config := p.getConfiguration()

	encryptedToken, err := encrypt([]byte(config.EncryptionKey), info.Token)
	if err != nil {
		return err
	}

	info.Token = encryptedToken

	jsonInfo, err := json.Marshal(info)
	if err != nil {
		return err
	}

	if err := p.API.KVSet(info.Username+jenkinsTokenKey, jsonInfo); err != nil {
		return err
	}

	return nil
}

func (p *Plugin) getJenkinsUserInfo(userID string) (*JenkinsUserInfo, error) {
	config := p.getConfiguration()

	var userInfo JenkinsUserInfo

	if infoBytes, err := p.API.KVGet(userID + jenkinsTokenKey); err != nil || infoBytes == nil {
		return nil, err
	} else if err := json.Unmarshal(infoBytes, &userInfo); err != nil {
		return nil, err
	}

	unencryptedToken, err := decrypt([]byte(config.EncryptionKey), userInfo.Token)
	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}

	userInfo.Token = unencryptedToken

	return &userInfo, nil
}
