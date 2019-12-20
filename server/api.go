package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

func (p *Plugin) InitAPI() *mux.Router {
	r := mux.NewRouter()
	r.HandleFunc("/triggerBuild", p.handleBuildTrigger).Methods("POST")
	r.HandleFunc("/createJob", p.handleJobCreation).Methods("POST")
	r.HandleFunc("/assets/jenkins.png", p.handleProfileImage).Methods("GET")
	return r
}

func (p *Plugin) ServeHTTP(c *plugin.Context, w http.ResponseWriter, r *http.Request) {
	config := p.getConfiguration()

	if err := p.IsValid(config); err != nil {
		http.Error(w, "This plugin is not configured.", http.StatusNotImplemented)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	p.API.LogDebug("New request:", "Host", r.Host, "RequestURI", r.RequestURI, "Method", r.Method)
	p.router.ServeHTTP(w, r)
}

func (p *Plugin) handleBuildTrigger(w http.ResponseWriter, r *http.Request) {
	jobName := r.FormValue("jobName")
	decodedJobName, _ := url.QueryUnescape(jobName)

	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	body, _ := ioutil.ReadAll(r.Body)
	bodyString := string(body)

	request := model.SubmitDialogRequestFromJson(strings.NewReader(bodyString))
	if request == nil {
		p.API.LogError("failed to decode request")
		return
	}

	parameters := make(map[string]string)
	for k, v := range request.Submission {
		parameters[k] = v.(string)
	}

	build, err := p.triggerJenkinsJob(userID, request.ChannelId, jobName, parameters)
	if err != nil {
		p.API.LogError("Error triggering build", "job_name", jobName, "err", err.Error())
		return
	}
	p.createPost(userID, request.ChannelId, fmt.Sprintf("Job '%s' - #%d has been started\nBuild URL : %s", decodedJobName, build.GetBuildNumber(), build.GetUrl()))
}

func (p *Plugin) handleJobCreation(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("Mattermost-User-ID")
	if userID == "" {
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	body, _ := ioutil.ReadAll(r.Body)
	bodyString := string(body)

	request := model.SubmitDialogRequestFromJson(strings.NewReader(bodyString))
	if request == nil {
		p.API.LogError("failed to decode request")
		return
	}

	jobInputs := make(map[string]string)
	for k, v := range request.Submission {
		jobInputs[k] = v.(string)
	}
	p.sendJobCreateRequest(userID, request.ChannelId, jobInputs)
}

func (p *Plugin) handleProfileImage(w http.ResponseWriter, r *http.Request) {
	config := p.getConfiguration()

	img, err := os.Open(filepath.Join(config.PluginsDirectory, manifest.Id, "assets", "jenkins.png"))
	if err != nil {
		http.NotFound(w, r)
		p.API.LogError("unable to read Jenkins profile image", "err", err.Error())
		return
	}
	defer func() {
		if err = img.Close(); err != nil {
			p.API.LogError("can't close img", "err", err.Error())
		}
	}()

	w.Header().Set("Content-Type", "image/png")
	_, err = io.Copy(w, img)
	if err != nil {
		p.API.LogError("can't copy image profile to http response writer", "err", err.Error())
	}
}
