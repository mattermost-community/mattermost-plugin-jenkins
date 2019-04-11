package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mattermost/mattermost-server/model"
	"net/http"
	"net/url"
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
	configuration := p.getConfiguration()
	if err := p.IsValid(configuration); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) IsValid(configuration *configuration) error {
	if configuration.JenkinsURL == "" {
		return fmt.Errorf("Please add Jekins URL in plugin settings")
	}

	u, err := url.Parse(configuration.JenkinsURL)
	if err != nil {
		return err
	}

	if u.Scheme == "" {
		return fmt.Errorf("Please add scheme to the URL. HTTP or HTTPS")
	}

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
func (p *Plugin) verifyJenkinsCredentials(username, token string) (bool, error) {
	pluginConfig := p.getConfiguration()
	u, err := url.Parse(pluginConfig.JenkinsURL)
	if err != nil {
		return false, err
	}
	scheme := u.Scheme
	url := fmt.Sprintf("%s://%s:%s@%s", scheme, username, token, u.Host)
	response, respErr := http.Get(url)
	if respErr != nil {
		return false, respErr
	}
	if response.StatusCode == 200 {
		return true, nil
	}
	return false, errors.New("Error verifying Jenkins credentials")
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
	if _, err := p.API.CreatePost(post); err != nil {
		p.API.LogError("Could not create a post", "user_id", userID, "err", err.Error())
	}
}

func (p *Plugin) createJenkinsClient(userID string) (*gojenkins.Jenkins, error) {
	pluginConfig := p.getConfiguration()
	userInfo, err := p.getJenkinsUserInfo(userID)
	if err != nil {
		return nil, errors.New("Error fetching Jenkins user info " + err.Error())
	}

	jenkins := gojenkins.CreateJenkins(nil, pluginConfig.JenkinsURL, userInfo.Username, userInfo.Token)
	_, errJenkins := jenkins.Init()
	if errJenkins != nil {
		p.API.LogError("Error creating the jenkins client", "Err", errJenkins.Error())
		return nil, errors.New("Error creating jenkins client " + err.Error())
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

	p.createEphemeralPost(userID, channelID, fmt.Sprintf("Build for the job '%s' has been triggered and is in queue.", jobName))

	// Polling the job to see if the build has started
	for {
		if task.Raw.Executable.URL != "" {
			break
		}
		time.Sleep(30 * time.Second)
		task.Poll()
	}

	buildInfo, buildErr := jenkins.GetBuild(jobName, task.Raw.Executable.Number)
	if buildErr != nil {
		return nil, errors.New("Error fetching the build details. " + buildErr.Error())
	}

	return buildInfo, nil
}

func (p *Plugin) fetchAndUploadArtifactsOfABuild(userID, channelID, jobName string) error {
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

	build, buildErr := job.GetLastBuild()
	if buildErr != nil {
		p.API.LogError(buildErr.Error())
		return jenkinsErr
	}
	artifacts := build.GetArtifacts()
	if len(artifacts) == 0 {
		p.createEphemeralPost(userID, channelID, "No artifacts found in the last build.")
	} else {
		p.createEphemeralPost(userID, channelID, fmt.Sprintf("%d Artifact(s) found", len(artifacts)))
	}
	for _, a := range artifacts {
		fileData, fileDataErr := a.GetData()
		if fileDataErr != nil {
			p.API.LogError("Error uploading file", "file_name", a.FileName)
			return fileDataErr
		}
		p.createEphemeralPost(userID, channelID, fmt.Sprintf("Uploading artifact '%s' ...", a.FileName))
		fileInfo, fileInfoErr := p.API.UploadFile(fileData, channelID, a.FileName)
		if fileInfoErr != nil {
			p.API.LogError("Error uploading file", "file_name", a.FileName)
			return fileInfoErr
		}
		p.createPost(userID, channelID, fmt.Sprintf("Artifact - %s : %s", fileInfo.Name, *config.ServiceSettings.SiteURL+"/api/v4/files/"+fileInfo.Id))
	}
	return nil
}

func (p *Plugin) fetchTestReportsLinkOfABuild(userID, channelID string, jobName string) (string, error) {
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

	u, err := url.Parse(pluginConfig.JenkinsURL)
	if err != nil {
		return "", err
	}

	scheme := u.Scheme

	// TODO: Use gojenkins package if the requirement is to fetch the test results
	// rather than replying to the slash command with the test reports link.
	testReportInternalURL := fmt.Sprintf("%s://%s:%s@%s/job/%s/%d/testReport", scheme, userInfo.Username, userInfo.Token, u.Host, jobName, lastBuild.GetBuildNumber())
	testReportAPIURL := testReportInternalURL + "/api/json"
	response, respErr := http.Get(testReportAPIURL)
	if respErr != nil {
		return "", respErr
	}

	if response.StatusCode == 200 {
		testReportsURL := fmt.Sprintf("%s/job/%s/%d/testReport", pluginConfig.JenkinsURL, job.GetName(), lastBuild.GetBuildNumber())
		return "Test reports URL: " + testReportsURL, nil
	} else if response.StatusCode == 404 {
		return "Test reports for the job " + jobName + " doesn't exist", nil
	}

	return "", errors.New("Error fetching test results")
}
