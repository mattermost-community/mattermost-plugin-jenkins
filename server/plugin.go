package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-server/model"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mattermost/mattermost-server/plugin"
	"github.com/pkg/errors"
	"github.com/waseem18/gojenkins"
)

const (
	jenkinsUsername = "Jenkins Plugin"
	jenkinsTokenKey = "_jenkinsToken"
)

type Plugin struct {
	plugin.MattermostPlugin

	router *mux.Router

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
	p.router = p.InitAPI()
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

	infoBytes, infoErr := p.API.KVGet(userID + jenkinsTokenKey)

	if infoErr != nil {
		return nil, infoErr
	} else if infoBytes == nil {
		return nil, errors.New("User not found")
	} else if err := json.Unmarshal(infoBytes, &userInfo); err != nil {
		return nil, err
	}

	unencryptedToken, err := decrypt([]byte(config.EncryptionKey), userInfo.Token)
	if err != nil {
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

// createEphemeralPost creates an ephemeral post
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

// createPost creates a non epehemeral post
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

// getJenkinsClient creates a Jenkins client given user ID.
func (p *Plugin) getJenkinsClient(userID string) (*gojenkins.Jenkins, error) {
	pluginConfig := p.getConfiguration()
	userInfo, err := p.getJenkinsUserInfo(userID)
	if err != nil {
		return nil, errors.Wrap(err, "Error fetching Jenkins user information")
	}

	jenkins := gojenkins.CreateJenkins(nil, pluginConfig.JenkinsURL, userInfo.Username, userInfo.Token)
	_, errJenkins := jenkins.Init()
	if errJenkins != nil {
		wrap := errors.Wrap(errJenkins, "Error creating Jenkins client")
		return nil, wrap
	}
	return jenkins, nil
}

// getJob returns a Job object given the jobname.
func (p *Plugin) getJob(userID, jobName string) (*gojenkins.Job, error) {
	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return nil, errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}

	containsSlash := strings.Contains(jobName, "/")
	if containsSlash {
		jobName = strings.Replace(jobName, "/", "/job/", -1)
	}

	job, jobErr := jenkins.GetJob(jobName)
	if jobErr != nil {
		return nil, errors.Wrap(jobErr, "Error fetching job")
	}

	return job, nil
}

// getBuild returns the last build of the given job if buildID is specified.
// Returns last build of the job if buildID is an empty string.
func (p *Plugin) getBuild(jobName, userID, buildID string) (*gojenkins.Build, error) {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return nil, jobErr
	}

	var build *gojenkins.Build
	var buildErr error
	if buildID == "" {
		build, buildErr = job.GetLastBuild()
		if buildErr != nil {
			return nil, buildErr
		}
		return build, nil
	}
	buildIDInt, _ := strconv.ParseInt(buildID, 10, 64)
	build, buildErr = job.GetBuild(buildIDInt)
	if buildErr != nil {
		return nil, buildErr
	}
	return build, nil
}

// triggerJenkinsJob triggers a Jenkins build and polls the build in the queue to see if the build has started.
func (p *Plugin) triggerJenkinsJob(userID, channelID, jobName string, parameters map[string]string) (string, error) {
	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return "", errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}
	buildQueueID, buildErr := p.buildJenkinsJob(jenkins, userID, channelID, jobName, parameters)
	if buildErr != nil {
		return "", buildErr
	}
	build, err := p.checkIfJobHasStarted(jenkins, jobName, buildQueueID)
	if err != nil {
		return "", err
	}
	return build.GetUrl(), nil
}

// buildJenkinsJob starts a given Jenkins build and
// creates an epehemeral post once the build has been successfully triggered.
func (p *Plugin) buildJenkinsJob(jenkins *gojenkins.Jenkins, userID, channelID, jobName string, parameters map[string]string) (int64, error) {
	buildQueueID, buildErr := jenkins.BuildJob(jobName, parameters)
	if buildErr != nil {
		return -1, errors.Wrap(buildErr, "Error building job")
	}

	p.createEphemeralPost(userID, channelID, fmt.Sprintf("Build for the job '%s' has been triggered and is in queue.", strings.Replace(jobName, "/job", "/", -1)))
	return buildQueueID, nil
}

// checkIfJobHasStarted polls the job to see if the build has started
func (p *Plugin) checkIfJobHasStarted(jenkins *gojenkins.Jenkins, jobName string, buildQueueID int64) (*gojenkins.Build, error) {
	task, taskErr := jenkins.GetQueueItem(buildQueueID)
	if taskErr != nil {
		return nil, taskErr
	}

	for {
		if task.Raw.Executable.URL != "" {
			break
		}
		time.Sleep(pollingSleepTime * time.Second)
		task.Poll()
	}
	buildInfo, buildErr := jenkins.GetBuild(jobName, task.Raw.Executable.Number)
	if buildErr != nil {
		return nil, errors.Wrap(buildErr, "Error building job")
	}

	return buildInfo, nil
}

func (p *Plugin) fetchAndUploadArtifactsOfABuild(userID, channelID, jobName string) error {
	config := p.API.GetConfig()

	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return errors.Wrap(jobErr, "Error fetching job")
	}

	build, buildErr := job.GetLastSuccessfulBuild()
	if buildErr != nil {
		return errors.Wrap(buildErr, "Error fetching build information")
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
			return errors.Wrap(fileDataErr, "Error fetching file data")
		}
		p.createEphemeralPost(userID, channelID, fmt.Sprintf("Uploading artifact '%s' ...", a.FileName))
		fileInfo, fileInfoErr := p.API.UploadFile(fileData, channelID, a.FileName)
		if fileInfoErr != nil {
			return errors.Wrap(fileInfoErr, "Error uploading file")
		}
		p.createPost(userID, channelID, fmt.Sprintf("Artifact '%s' : %s", fileInfo.Name, *config.ServiceSettings.SiteURL+"/api/v4/files/"+fileInfo.Id))
	}
	return nil
}

func (p *Plugin) fetchTestReportsLinkOfABuild(userID, channelID string, jobName string) (string, error) {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return "", errors.Wrap(jobErr, "Error fetching job")
	}

	lastBuild, buildErr := job.GetLastBuild()
	if buildErr != nil {
		return "", errors.Wrap(buildErr, "Error fetching build information")
	}

	hasTestResults, hasTestErr := lastBuild.HasTestResults()
	if hasTestErr != nil {
		return "", errors.Wrap(hasTestErr, "Error checking for test results")
	}

	msg := ""
	if hasTestResults {
		testReportsURL := fmt.Sprintf("%s%d/testReport", job.Raw.URL, lastBuild.GetBuildNumber())
		msg = fmt.Sprintf("Test reports for the last build: %s", testReportsURL)
	} else {
		msg = fmt.Sprintf("Last build of the job '%s' doesn't have test reports.", jobName)
	}

	return msg, nil
}

func (p *Plugin) disableJob(userID, jobName string) error {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return errors.Wrap(jobErr, "Error fetching job")
	}

	_, disableErr := job.Disable()

	if disableErr != nil {
		return errors.Wrap(disableErr, "Error disabling job")
	}
	return nil
}

func (p *Plugin) enableJob(userID, jobName string) error {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return errors.Wrap(jobErr, "Error fetching job")
	}
	_, enableErr := job.Enable()

	if enableErr != nil {
		return errors.Wrap(enableErr, "Error enabling job")
	}
	return nil
}

func (p *Plugin) checkIfJobAcceptsParameters(userID, jobName string) (bool, error) {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return false, errors.Wrap(jobErr, "Error fetching job")
	}

	jobParameters, err := job.GetParameters()
	if err != nil {
		return false, errors.Wrap(err, "Error fetching job parameters")
	}

	if len(jobParameters) > 0 {
		return true, nil
	}

	return false, nil
}

// createDialogueForParameters creates an interactive dialogue for the user to input build parameters.
func (p *Plugin) createDialogueForParameters(userID, triggerID, jobName, channelID string) error {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return errors.Wrap(jobErr, "Error fetching job")
	}

	jobParameters, err := job.GetParameters()
	if err != nil {
		return errors.Wrap(err, "Error fetching job parameters")
	}

	var dialogueElementArr []model.DialogElement

	for i := 0; i < len(jobParameters); i++ {
		d := model.DialogElement{DisplayName: jobParameters[i].Name, Name: jobParameters[i].Name, Type: "text", SubType: "text"}
		dialogueElementArr = append(dialogueElementArr, d)
	}
	siteURL := *p.API.GetConfig().ServiceSettings.SiteURL
	encodedJobName, _ := url.Parse(jobName)
	dialog := model.OpenDialogRequest{
		TriggerId: triggerID,
		URL:       fmt.Sprintf("%s/plugins/jenkins/triggerBuild/%s", siteURL, encodedJobName),
		Dialog: model.Dialog{
			Title:       fmt.Sprintf("Parameters of %s", jobName),
			CallbackId:  userID,
			SubmitLabel: "Trigger job",
			Elements:    dialogueElementArr,
		},
	}
	dialogErr := p.API.OpenInteractiveDialog(dialog)
	if dialogErr != nil {
		return errors.Wrap(dialogErr, "Error opening the interactive dialog")
	}

	return nil
}

// fetchAndUploadBuildLog fetches console log of the given job and build.
// and uploads the console log as file to Mattermost server.
func (p *Plugin) fetchAndUploadBuildLog(userID, channelID, jobName, buildID string) error {
	config := p.API.GetConfig()
	build, buildErr := p.getBuild(jobName, userID, buildID)
	if buildErr != nil {
		return buildErr
	}
	consoleOutput := build.GetConsoleOutput()
	fileInfo, fileUploadErr := p.API.UploadFile([]byte(consoleOutput), channelID, jobName)
	if fileUploadErr != nil {
		return errors.Wrap(fileUploadErr, "Error uploading file")
	}
	p.createPost(userID, channelID, fmt.Sprintf("Build log for the job '%s' : %s", jobName, *config.ServiceSettings.SiteURL+"/api/v4/files/"+fileInfo.Id))
	return nil
}

// abortBuild aborts a given build.
// If the build ID is specified as an empty string, method fetches and aborts the last build of the job.
func (p *Plugin) abortBuild(userID, jobName, buildID string) error {
	build, buildErr := p.getBuild(jobName, userID, buildID)
	if buildErr != nil {
		return buildErr
	}

	isStopped, stopErr := build.Stop()
	if stopErr != nil {
		return stopErr
	}

	if isStopped {
		return nil
	}
	return errors.New("error stopping the build")
}
