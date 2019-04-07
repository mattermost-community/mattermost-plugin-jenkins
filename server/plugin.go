package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mattermost/mattermost-server/model"
	"net/http"
	"strconv"
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
	url := fmt.Sprintf("https://%s:%s@%s", username, token, pluginConfig.JenkinsURL)

	response, _ := http.Get(url)

	if response.StatusCode == 200 {
		return true
	}
	return false
}

func (p *Plugin) createEphemeralPost(userID, channelID, message string) {
	post := &model.Post{
		UserId:    userID,
		ChannelId: channelID,
		Message:   message,
		Type:      model.POST_DEFAULT,
		Props: map[string]interface{}{
			"from_webhook":      "true",
			"override_username": jenkinsUsername,
			"override_icon_url": p.getConfiguration().ProfileImageURL,
		},
	}
	p.API.SendEphemeralPost(userID, post)
}

func (p *Plugin) createPost(userID, channelID, message string) {
	post := &model.Post{
		UserId:    userID,
		ChannelId: channelID,
		Message:   message,
		Type:      model.POST_DEFAULT,
		Props: map[string]interface{}{
			"from_webhook":      "true",
			"override_username": jenkinsUsername,
			"override_icon_url": p.getConfiguration().ProfileImageURL,
		},
	}
	p.API.CreatePost(post)
}

func (p *Plugin) createJenkinsClient(userID string) (*gojenkins.Jenkins, error) {
	pluginConfig := p.getConfiguration()
	userInfo, err := p.getJenkinsUserInfo(userID)
	if err != nil {
		return nil, errors.New("Error fetching Jenkins user info " + err.Error())
	}

	jenkins := gojenkins.CreateJenkins(nil, "https://"+pluginConfig.JenkinsURL, userInfo.Username, userInfo.Token)
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

	p.createEphemeralPost(userID, channelID, buildTriggerResponse)

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

func (p *Plugin) fetchAndUploadArtifactsOfABuild(userID, channelID, jobName, buildNumber string) error {
	config := p.API.GetConfig()

	jenkins, jenkinsErr := p.createJenkinsClient(userID)
	if jenkinsErr != nil {
		p.API.LogError(jenkinsErr.Error())
		return jenkinsErr
	}

	job, jobErr := jenkins.GetJob(jobName)
	if jobErr != nil {
		p.API.LogError(jobErr.Error())
		return jobErr
	}

	buildNumberInt64, err := strconv.ParseInt(buildNumber, 10, 64)
	if err != nil {
		p.API.LogError(err.Error())
		return jenkinsErr
	}

	build, buildErr := job.GetBuild(buildNumberInt64)
	if buildErr != nil {
		p.API.LogError(buildErr.Error())
		return jenkinsErr
	}

	artifacts := build.GetArtifacts()
	if len(artifacts) == 0 {
		p.createEphemeralPost(userID, channelID, "Not artifacts found.")
	}
	for _, a := range artifacts {
		fileData, _ := a.GetData()
		fileInfo, err := p.API.UploadFile(fileData, channelID, a.FileName)
		if err != nil {
			p.API.LogError("Error uploading file with the name", a.FileName)
		}
		p.createPost(userID, channelID, "Here's the artifact : "+*config.ServiceSettings.SiteURL+"/api/v4/files/"+fileInfo.Id)
	}
	return nil
}

func (p *Plugin) fetchTestReportsLinkOfABuild(userID, channelID string, parameters []string) (string, error) {
	jobName := parameters[0]
	jenkins, jenkinsErr := p.createJenkinsClient(userID)
	if jenkinsErr != nil {
		p.API.LogError(jenkinsErr.Error())
		return "", jenkinsErr
	}

	job, jobErr := jenkins.GetJob(jobName)
	if jobErr != nil {
		p.API.LogError(jobErr.Error())
		return "", jenkinsErr
	}

	lastBuild, buildErr := job.GetLastBuild()
	if buildErr != nil {
		return "", buildErr
	}
	pluginConfig := p.getConfiguration()
	userInfo, userInfoErr := p.getJenkinsUserInfo(userID)
	if userInfoErr != nil {
		return "", userInfoErr
	}
	// TODO: Use gojenkins package if the requirement is to fetch the test results
	// rather than replying to the slash command with the test reports link.
	testReportInternalURL := fmt.Sprintf("https://%s:%s@%s/job/%s/%d/testReport", userInfo.Username, userInfo.Token, pluginConfig.JenkinsURL, jobName, lastBuild.GetBuildNumber())
	testReportAPIURL := testReportInternalURL + "/api/json"

	response, respErr := http.Get(testReportAPIURL)
	if respErr != nil {
		return "", respErr
	}
	
	if response.StatusCode == 200 {
		testReportsURL := fmt.Sprintf("https://%s/job/%s/%d/testReport", pluginConfig.JenkinsURL, jobName, lastBuild.GetBuildNumber())
		return "Test reports URL: " + testReportsURL, nil
	} else if response.StatusCode == 404 {
		return "Test reports for the job " + jobName + " doesn't exist", nil
	}

	return "", errors.New("Error fetching test results")
}
