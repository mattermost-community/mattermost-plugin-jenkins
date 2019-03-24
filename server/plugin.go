package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mattermost/mattermost-server/model"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bndr/gojenkins"
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
	UserID   string
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

	if err := p.API.KVSet(info.UserID+jenkinsTokenKey, jsonInfo); err != nil {
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
		p.API.LogError(err.Error())
		return nil, err
	}

	userInfo.Token = unencryptedToken

	return &userInfo, nil
}

// verifyJenkinsCredentials verifies the authenticity of the username and token
// by sending a GET call to the Jenkins URL specified in the config.
func (p *Plugin) verifyJenkinsCredentials(username, token string) bool {
	pluginConfig := p.getConfiguration()
	url := fmt.Sprintf("http://%s:%s@%s", username, token, pluginConfig.JenkinsURL)

	response, _ := http.Get(url)

	if response.StatusCode == 200 {
		return true
	}
	return false
}

func (p *Plugin) createEphemeralPost(userID, channelID string) {
	post := &model.Post{
		UserId:    userID,
		ChannelId: channelID,
		Message:   buildTriggerResponse,
		Type:      model.POST_DEFAULT,
		Props: map[string]interface{}{
			"from_webhook":      "true",
			"override_username": jenkinsUsername,
			"override_icon_url": p.getConfiguration().ProfileImageURL,
		},
	}
	p.API.SendEphemeralPost(userID, post)
}

func (p *Plugin) createJenkinsClient(userID string) (*gojenkins.Jenkins, error) {
	pluginConfig := p.getConfiguration()
	userInfo, err := p.getJenkinsUserInfo(userID)
	if err != nil {
		return nil, errors.New("Error fetching Jenkins user info " + err.Error())
	}

	jenkins := gojenkins.CreateJenkins(nil, "http://"+pluginConfig.JenkinsURL, userInfo.Username, userInfo.Token)
	_, errJenkins := jenkins.Init()
	if errJenkins != nil {
		p.API.LogError("Error creating the jenkins client", "Err", errJenkins.Error())
		return nil, errors.New("Error creating the jenkins client " + err.Error())
	}
	return jenkins, nil
}

func (p *Plugin) triggerJenkinsJob(userID, channelID, jobName string) (*gojenkins.Build, error) {
	jenkins, jenkinsErr := p.createJenkinsClient(userID)
	if jenkinsErr != nil {
		return nil, jenkinsErr
	}

	jobName = strings.TrimLeft(strings.TrimRight(jobName, `\"`), `\"`)

	containsSlash := strings.Contains(jobName, "/")
	if containsSlash {
		jobName = strings.Replace(jobName, "/", "/job/", -1)
	}

	buildQueueID, buildErr := jenkins.BuildJob(jobName)
	if buildErr != nil {
		return nil, errors.New("Error building the job. " + buildErr.Error())
	}

	task, taskErr := jenkins.GetQueueItem(buildQueueID)
	if taskErr != nil {
		return nil, errors.New("Error fetching job details from queue. " + taskErr.Error())
	}

	p.createEphemeralPost(userID, channelID)

	// Polling the job to see if the build has started
	retry := 10
	for retry > 0 {
		if task.Raw.Executable.URL != "" {
			break
		}
		time.Sleep(10 * time.Second)
		task.Poll()
		retry--
	}

	buildInfo, buildErr := jenkins.GetBuild(jobName, task.Raw.Executable.Number)
	if buildErr != nil {
		return nil, errors.New("Error fetching the build details. " + buildErr.Error())
	}

	return buildInfo, nil
}
