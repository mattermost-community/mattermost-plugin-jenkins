package main

import (
	"fmt"
	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
	"net/http"
	"strings"
)

const helpText = `* |/jenkins connect <username> <API Token>| - Connect your Mattermost account to Jenkins`

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "jenkins",
		Description:      "A Mattermost plugin to interact with Jenkins",
		DisplayName:      "Jenkins",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: connect, build, help",
		AutoCompleteHint: "[command]",
	}
}

func (p *Plugin) getCommandResponse(responseType, text string) *model.CommandResponse {
	return &model.CommandResponse{
		ResponseType: responseType,
		Username:     jenkinsUsername,
		IconURL:      p.getConfiguration().ProfileImageURL,
		Text:         text,
		Type:         model.POST_DEFAULT,
	}
}

func (p *Plugin) ExecuteCommand(c *plugin.Context, args *model.CommandArgs) (*model.CommandResponse, *model.AppError) {
	pluginConfig := p.getConfiguration()

	split := strings.Fields(args.Command)
	command := split[0]
	parameters := []string{}
	action := ""
	if len(split) > 1 {
		action = split[1]
	}
	if len(split) > 2 {
		parameters = split[2:]
	}

	if command != "/jenkins" {
		return &model.CommandResponse{}, nil
	}

	switch action {
	case "connect":
		if len(parameters) == 0 || len(parameters) == 1 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify both username and API token."), nil
		} else if len(parameters) == 2 {
			verify := p.verifyJenkinsCredentials(parameters[0], parameters[1])
			if verify == false {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Incorrect username or token"), nil
			}
			jenkinsUserInfo := &JenkinsUserInfo{
				UserID:   args.UserId,
				Username: parameters[0],
				Token:    parameters[1],
			}
			err := p.storeJenkinsUserInfo(jenkinsUserInfo)
			if err != nil {
				mlog.Error("Error saving Jenkins user information to KV store" + err.Error())
				return &model.CommandResponse{}, nil
			}
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Jenkins has been connected."), nil
		}
	case "build":
		userInfo, err := p.getJenkinsUserInfo(args.UserId)
		if err != nil {
			mlog.Error(err.Error())
			return &model.CommandResponse{}, nil
		}
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify the job name."), nil
		} else if len(parameters) == 1 {
			jenkinsBaseURL := fmt.Sprintf("http://%s:%s@%s", userInfo.Username, userInfo.Token, pluginConfig.JenkinsURL)
			jobName := parameters[0]
			jobURL := fmt.Sprintf("%s/job/%s/build", jenkinsBaseURL, jobName)
			response, err := http.Post(jobURL, "", nil)
			if err != nil {
				mlog.Error(err.Error())
			}
			if response.StatusCode == 401 {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Your Jenkins account doesn't have enough permissions to build the job."), nil
			} else if response.StatusCode == 404 {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Job doesn't exist."), nil
			} else if response.StatusCode == 201 {
				locationHeader := response.Header.Get("Location")
				splitBuildQueueURL := strings.Split(locationHeader, "/")
				buildNumber := splitBuildQueueURL[len(splitBuildQueueURL)-2]
				buildURL := fmt.Sprintf("http://%s/job/%s/%s", pluginConfig.JenkinsURL, jobName, buildNumber)
				message := fmt.Sprintf("Started job '%s' - %s", jobName, buildURL)
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, message), nil
			}
		}
	}
	return &model.CommandResponse{}, nil
}
