package main

import (
	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/plugin"
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
			jenkinsUserInfo := &JenkinsUserInfo{
				Username: parameters[0],
				Token:    parameters[1],
			}
			err := p.storeJenkinsUserInfo(jenkinsUserInfo)
			if err != nil {
				mlog.Error("Unable to save Jenkins user information to KV store" + err.Error())
				return &model.CommandResponse{}, nil
			}
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Jenkins has been connected."), nil
		}
	}
	return &model.CommandResponse{}, nil
}
