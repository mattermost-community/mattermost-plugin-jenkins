package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"github.com/waseem18/gojenkins"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
	"github.com/mattermost/mattermost/server/public/pluginapi"
)

const (
	jenkinsTokenKey = "_jenkinsToken"
	botUserName     = "jenkins"
	botDisplayName  = "Jenkins"
	botDescription  = "Created by the Jenkins Plugin."
)

type Plugin struct {
	plugin.MattermostPlugin
	client *pluginapi.Client

	router *mux.Router

	// configurationLock synchronizes access to the configuration.
	configurationLock sync.RWMutex

	// configuration is the active plugin configuration. Consult getConfiguration and
	// setConfiguration for usage.
	configuration *configuration

	botUserID string
}

type JenkinsUserInfo struct {
	UserID   string
	Username string
	Token    string
}

func (p *Plugin) OnActivate() error {
	p.client = pluginapi.NewClient(p.API, p.Driver)

	botUserID, err := p.client.Bot.EnsureBot(&model.Bot{
		Username:    botUserName,
		DisplayName: botDisplayName,
		Description: botDescription,
	})
	if err != nil {
		return errors.Wrap(err, "failed to ensure bot")
	}
	p.botUserID = botUserID

	bundlePath, err := p.API.GetBundlePath()
	if err != nil {
		return errors.Wrap(err, "failed to get bundle path")
	}

	profileImage, err := os.ReadFile(filepath.Join(bundlePath, "assets", "jenkins.png"))
	if err != nil {
		return errors.Wrap(err, "failed to read profile image")
	}

	if appErr := p.API.SetProfileImage(botUserID, profileImage); appErr != nil {
		return errors.Wrap(appErr, "failed to set profile image")
	}

	command, err := p.getCommand()
	if err != nil {
		return errors.Wrap(err, "failed to get command")
	}

	if err := p.API.RegisterCommand(command); err != nil {
		p.API.LogWarn("Error registering Jenkins custom command", "err", err)
	}

	p.router = p.InitAPI()
	conf := p.getConfiguration()
	if err := p.IsValid(conf); err != nil {
		return err
	}
	return nil
}

func (p *Plugin) IsValid(configuration *configuration) error {
	if configuration.JenkinsURL == "" {
		return fmt.Errorf("please add Jenkins URL in plugin settings")
	}

	u, err := url.Parse(configuration.JenkinsURL)
	if err != nil {
		return err
	}

	if u.Scheme == "" {
		return fmt.Errorf("please add scheme to the URL. HTTP or HTTPS")
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
		return nil, errors.New("user not found")
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
	response, respErr := http.Get(fmt.Sprintf("%s://%s:%s@%s", scheme, username, token, u.Host))
	if respErr != nil {
		return false, respErr
	}
	defer response.Body.Close()
	if response.StatusCode == 200 {
		return true, nil
	}
	return false, errors.New("error verifying Jenkins credentials")
}

// createEphemeralPost creates an ephemeral post
func (p *Plugin) createEphemeralPost(userID, channelID, message string) {
	post := &model.Post{
		UserId:    p.botUserID,
		ChannelId: channelID,
		Message:   message,
		Type:      model.PostTypeDefault,
	}
	p.API.SendEphemeralPost(userID, post)
}

// createPost creates a non epehemeral post
func (p *Plugin) createPost(userID, channelID, message string, fileIds ...string) {
	userInfo, userInfoErr := p.getJenkinsUserInfo(userID)
	if userInfoErr != nil {
		p.API.LogError("Error fetching Jenkins user details", "err", userInfoErr.Error())
		return
	}

	slackAttachment := generateSlackAttachment(message)
	slackAttachment.Pretext = fmt.Sprintf("Initiated by Jenkins user: %s", userInfo.Username)
	post := &model.Post{
		UserId:    p.botUserID,
		ChannelId: channelID,
		Type:      model.PostTypeDefault,
		Props: map[string]interface{}{
			"attachments": []*model.SlackAttachment{slackAttachment},
		},
	}

	if len(fileIds) > 0 {
		post.FileIds = append(post.FileIds, fileIds[0])
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
		jobName = strings.ReplaceAll(jobName, "/", "/job/")
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
			return nil, errors.Wrap(buildErr, "Error fetching last build")
		}
		return build, nil
	}
	buildIDInt, _ := strconv.ParseInt(buildID, 10, 64)
	build, buildErr = job.GetBuild(buildIDInt)
	if buildErr != nil {
		return nil, errors.Wrap(buildErr, "Error fetching the build")
	}
	return build, nil
}

// triggerJenkinsJob triggers a Jenkins build and polls the build in the queue to see if the build has started.
func (p *Plugin) triggerJenkinsJob(userID, channelID, jobName string, parameters map[string]string) (*gojenkins.Build, error) {
	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return nil, errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}
	containsSlash := strings.Contains(jobName, "/")
	if containsSlash {
		jobName = strings.ReplaceAll(jobName, "/", "/job/")
	}
	buildQueueID, buildErr := p.buildJenkinsJob(jenkins, userID, channelID, jobName, parameters)
	if buildErr != nil {
		return nil, buildErr
	}
	build, err := p.checkIfJobHasStarted(jenkins, jobName, buildQueueID)
	if err != nil {
		return nil, err
	}
	return build, nil
}

// buildJenkinsJob starts a given Jenkins build and
// creates an ephemeral post once the build has been successfully triggered.
func (p *Plugin) buildJenkinsJob(jenkins *gojenkins.Jenkins, userID, channelID, jobName string, parameters map[string]string) (int64, error) {
	buildQueueID, buildErr := jenkins.BuildJob(jobName, parameters)
	if buildErr != nil {
		return -1, errors.Wrap(buildErr, "Error building job")
	}

	if buildQueueID == 0 {
		p.createEphemeralPost(userID, channelID, "A build of this job is still in queue.\n Please trigger the job after the job's build queue is free.")
		return -1, errors.Wrap(buildErr, "error building the job as a previous build is still in queue")
	}

	p.createPost(userID, channelID, fmt.Sprintf("Job '%s' has been triggered and is in queue.", strings.ReplaceAll(jobName, "/job/", "/")))
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
		if _, err := task.Poll(); err != nil {
			p.API.LogWarn("Error polling jenkins job to check the build status", "err", err)
		}
	}
	buildInfo, buildErr := jenkins.GetBuild(jobName, task.Raw.Executable.Number)
	if buildErr != nil {
		return nil, errors.Wrap(buildErr, "Error gettting job details")
	}

	return buildInfo, nil
}

// fetchAndUploadArtifactsOfABuild checks if the specified job and build has artifacts and
// uploads them to MM server if artifacts are present.
// If build number is not specified, the method checks the last build of the job for artifacts.
func (p *Plugin) fetchAndUploadArtifactsOfABuild(userID, channelID, jobName, buildID string) error {
	config := p.API.GetConfig()
	build, buildErr := p.getBuild(jobName, userID, buildID)
	if buildErr != nil {
		return buildErr
	}

	artifacts := build.GetArtifacts()
	if len(artifacts) == 0 {
		p.createPost(userID, channelID, fmt.Sprintf("No artifacts found in the build #%d of the job '%s'", build.GetBuildNumber(), jobName))
	} else {
		p.createPost(userID, channelID, fmt.Sprintf("%d Artifact(s) found in the build #%d of the job '%s'", len(artifacts), build.GetBuildNumber(), jobName))
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

// getBuildTestResultsURL checks if the specified job and build has test results and
// creates a post with the test results URL if the  build has test results.
// If build number is not specified, the method checks the last build of the job for test results.
func (p *Plugin) getBuildTestResultsURL(userID, channelID, jobName, buildID string) error {
	build, buildErr := p.getBuild(jobName, userID, buildID)
	if buildErr != nil {
		return buildErr
	}

	hasTestResults, hasTestErr := build.HasTestResults()
	if hasTestErr != nil {
		return errors.Wrap(hasTestErr, "Error checking for test results")
	}
	msg := ""
	if hasTestResults {
		job, jobErr := p.getJob(userID, jobName)
		if jobErr != nil {
			return jobErr
		}
		testReportsURL := fmt.Sprintf("%s%d/testReport", job.Raw.URL, build.GetBuildNumber())
		msg = fmt.Sprintf("Test reports for the build #%d of the job '%s': %s", build.GetBuildNumber(), jobName, testReportsURL)
	} else {
		msg = fmt.Sprintf("Build #%d of the job '%s' doesn't have test reports.", build.GetBuildNumber(), jobName)
	}
	p.createPost(userID, channelID, msg)
	return nil
}

// disableJob disables a given job.
// Returns an error if the operation is not successful.
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

// enableJob enables a given job.
// Returns an error if the operation is not successful.
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

// checkIfJobAcceptsParameters checks if a given job accepts parameters to be able to be triggered.
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

// createDialogForParameters creates an interactive dialog for the user to input build parameters.
func (p *Plugin) createDialogForParameters(userID, triggerID, jobName, channelID string) error {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return errors.Wrap(jobErr, "Error fetching job")
	}

	jobParameters, err := job.GetParameters()
	if err != nil {
		return errors.Wrap(err, "Error fetching job parameters")
	}

	var dialogElementArr []model.DialogElement

	for i := 0; i < len(jobParameters); i++ {
		d := model.DialogElement{DisplayName: jobParameters[i].Name, Name: jobParameters[i].Name, Type: "text", SubType: "text"}
		dialogElementArr = append(dialogElementArr, d)
	}
	siteURL := *p.API.GetConfig().ServiceSettings.SiteURL
	encodedJobName, _ := url.Parse(jobName)
	dialog := model.OpenDialogRequest{
		TriggerId: triggerID,
		URL:       fmt.Sprintf("%s/plugins/jenkins/triggerBuild?jobName=%s", siteURL, encodedJobName),
		Dialog: model.Dialog{
			Title:       fmt.Sprintf("Parameters of %s", jobName),
			CallbackId:  userID,
			SubmitLabel: "Trigger job",
			Elements:    dialogElementArr,
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
	build, buildErr := p.getBuild(jobName, userID, buildID)
	if buildErr != nil {
		return buildErr
	}

	consoleOutput := build.GetConsoleOutput()
	filename := fmt.Sprintf("%s-%d", jobName, build.GetBuildNumber())
	fileInfo, fileUploadErr := p.API.UploadFile([]byte(consoleOutput), channelID, filename)
	if fileUploadErr != nil {
		return errors.Wrap(fileUploadErr, "Error uploading file")
	}

	msg := fmt.Sprintf("Console log of the build #%d of the job '%s'", build.GetBuildNumber(), jobName)
	p.createPost(userID, channelID, msg, fileInfo.Id)
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

// deleteJob deletes a given job.
// Returns an error if the operation fails.
func (p *Plugin) deleteJob(userID, jobName string) error {
	job, jobErr := p.getJob(userID, jobName)
	if jobErr != nil {
		return jobErr
	}
	_, err := job.Delete()
	if err != nil {
		return err
	}
	return nil
}

// safeRestart safe restarts the Jenkins server.
// Returns an error if the operation fails.
func (p *Plugin) safeRestart(userID string) error {
	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}
	if err := jenkins.SafeRestart(); err != nil {
		return err
	}
	return nil
}

// getListOfInstalledPlugins fetches the list of installed plugins on the Jenkins server.
func (p *Plugin) getListOfInstalledPlugins(userID, channelID string) error {
	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}
	plugins, pluginsErr := jenkins.GetPlugins(1)
	if pluginsErr != nil {
		return pluginsErr
	}
	msg := ""
	for k, v := range plugins.Raw.Plugins {
		status := "Disabled"
		if v.Enabled {
			status = "Enabled"
		}
		msg += fmt.Sprintf("%d. %s - %s - %s\n", k+1, v.LongName, v.Version, status)
	}
	p.createPost(userID, channelID, msg)
	return nil
}

func (p *Plugin) createJob(userID, channelID, triggerID string) error {
	if err := p.createDialogForJobCreation(userID, channelID, triggerID); err != nil {
		return err
	}
	return nil
}

// createDialogForJobCreation creates an interactive dialog
// for the user to input job name and the content of config.xml
func (p *Plugin) createDialogForJobCreation(userID, channelID, triggerID string) error {
	config := p.API.GetConfig()
	dialog := model.OpenDialogRequest{
		TriggerId: triggerID,
		URL:       fmt.Sprintf("%s/plugins/jenkins/createJob", *config.ServiceSettings.SiteURL),
		Dialog: model.Dialog{
			Title:       "Please paste the contents of config.xml file here",
			CallbackId:  userID,
			SubmitLabel: "Create job",
			Elements: []model.DialogElement{{
				DisplayName: "Job name",
				Name:        "JobName",
				Type:        "text",
				SubType:     "text",
				HelpText:    "Please use double quotes if the job name has spaces in it.",
				MaxLength:   10000, // Should revist this?
			}, {
				DisplayName: "Config.xml",
				Name:        "ConfigXml",
				Type:        "textarea",
				SubType:     "text",
			},
			},
		},
	}
	dialogErr := p.API.OpenInteractiveDialog(dialog)
	if dialogErr != nil {
		return errors.Wrap(dialogErr, "Error opening the interactive dialog")
	}
	return nil
}

// sendJobCreateRequest first parses the job name to analyze the folder and job names to be created
// and triggers a job creation request using the contents of config.xml pasted in the dialog.
func (p *Plugin) sendJobCreateRequest(userID, channelID string, parameters map[string]string) error {
	jobName := parameters["JobName"]
	configXML := parameters["ConfigXml"]

	jenkins, jenkinsErr := p.getJenkinsClient(userID)
	if jenkinsErr != nil {
		return errors.Wrap(jenkinsErr, "Error creating Jenkins client")
	}

	jobName, extraParam, _, ok := parseBuildParameters(strings.Split(jobName, " "))
	if !ok || extraParam != "" {
		p.createEphemeralPost(userID, channelID, "Please check `/jenkins help` to find help on how to create a job.")
		return errors.New("error while creating the job")
	}
	if strings.Contains(jobName, "/") {
		splitString := strings.Split(jobName, "/")
		jobName = splitString[len(splitString)-1]
		folderList := splitString[:len(splitString)-1]
		parentFolders := []string{}
		for _, v := range folderList {
			_, fErr := jenkins.GetFolder(v, parentFolders...)
			if fErr != nil {
				_, err := jenkins.CreateFolder(v, parentFolders...)
				if err != nil {
					p.createEphemeralPost(userID, channelID, "Error creating the job.")
					return err
				}
			}
			parentFolders = append(parentFolders, v)
		}

		job, jobErr := jenkins.CreateJobInFolder(configXML, jobName, folderList...)
		if jobErr != nil {
			p.createEphemeralPost(userID, channelID, "Error creating the job.")
			return jobErr
		}
		p.createPost(userID, channelID, fmt.Sprintf("Job '%s' has been created.", job.GetName()))
		return nil
	}
	job, err := jenkins.CreateJob(configXML, jobName)
	if err != nil {
		p.createEphemeralPost(userID, channelID, "Error creating the job.")
		return err
	}
	p.createPost(userID, channelID, fmt.Sprintf("Job '%s' has been created", job.GetName()))

	return nil
}
