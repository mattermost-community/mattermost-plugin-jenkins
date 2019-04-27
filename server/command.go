package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/plugin"
)

const helpText = `* |/jenkins connect username APIToken| - Connect your Mattermost account to Jenkins.
* |/jenkins build jobname| - Trigger a build for the given job.
  * If the job resides in a folder, specify the job as |folder1/jobname|. Note the slash character.
  * If the folder name or job name has spaces in it, wrap the jobname in double quotes as |"job name with space"| or |"folder with space/jobname"|.
  * Follow similar patterns for all commands which takes jobname as input.
  * Use double quotes only when there are spaces in the job name or folder name.
* |/jenkins get-artifacts jobname| - Get artifacts of the last build of the given job.
* |/jenkins test-results jobname| - Get test results of the last build of the given job.
* |/jenkins disable jobname| - Disable a given job.
* |/jenkins enable jobname| - Enanble a given job.
* |/jenkins me| - Display the connected Jenkins account.
`
const jobNotSpecifiedResponse = "Please specify a job name to build."
const jenkinsConnectedResponse = "Jenkins has been connected."
const pollingSleepTime = 10

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "jenkins",
		Description:      "A Mattermost plugin to interact with Jenkins",
		DisplayName:      "Jenkins",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: connect, build, get-artifacts, test-results, disable, enable, me, help",
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
			p.createEphemeralPost(args.UserId, args.ChannelId, "Validating Jenkins credentials...")
			verify, verifyErr := p.verifyJenkinsCredentials(parameters[0], parameters[1])
			if verifyErr != nil {
				p.API.LogError("Error connecting to Jenkins", "user_id", args.UserId, "Err", verifyErr.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Error connecting to Jenkins."), nil
			}

			if !verify {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Incorrect username or token"), nil
			}

			jenkinsUserInfo := &JenkinsUserInfo{
				UserID:   args.UserId,
				Username: parameters[0],
				Token:    parameters[1],
			}

			err := p.storeJenkinsUserInfo(jenkinsUserInfo)
			if err != nil {
				p.API.LogError("Error saving Jenkins user information to KV store", "Err", err.Error())
				return &model.CommandResponse{}, nil
			}

			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, jenkinsConnectedResponse), nil
		}
	case "build":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, ok := parseJobName(parameters)
			if !ok {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please check `/jenkins help` to find help on how to get trigger a job."), nil
			}

			containsSlash := strings.Contains(jobName, "/")
			if containsSlash {
				jobName = strings.Replace(jobName, "/", "/job/", -1)
			}

			hasParemeters, paramErr := p.checkIfJobAcceptsParameters(args.UserId, jobName)
			if paramErr != nil {
				p.API.LogError("Error checking for parameters", "err", paramErr.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
			}

			if hasParemeters {
				err := p.createDialogueForParameters(args.UserId, args.TriggerId, jobName, args.ChannelId)
				if err != nil {
					p.API.LogError("Error creating dialogue", "err", err.Error())
					return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
				}
			} else {
				buildURL, err := p.triggerJenkinsJob(args.UserId, args.ChannelId, jobName, nil)
				if err != nil {
					p.API.LogError("Error triggering build", "job_name", jobName, "err", err.Error())
					return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
				}
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, fmt.Sprintf("Build for the job '%s' has been started.\nHere's the build URL : %s", jobName, buildURL)), nil
			}
		}
	case "get-artifacts":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, ok := parseJobName(parameters)
			if !ok {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please check `/jenkins help` to find help on how to get test results of a build."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Fetching build artifacts of '%s'...", jobName))
			err := p.fetchAndUploadArtifactsOfABuild(args.UserId, args.ChannelId, jobName)
			if err != nil {
				p.API.LogError("Error fetching artifacts", "job_name", parameters[0], "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Error fetching artifacts."), nil
			}
		}
	case "test-results":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, ok := parseJobName(parameters)
			if !ok {
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please check `/jenkins help` to find help on how to get test results of a build."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Fetching test results of '%s'...", jobName))
			testReportMsg, err := p.fetchTestReportsLinkOfABuild(args.UserId, args.ChannelId, jobName)
			if err != nil {
				p.API.LogError("Error fetching test results", "job_name", parameters[0], "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Error fetching test results."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, testReportMsg)
		}
	case "disable":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify a job to disable."), nil
		} else if len(parameters) == 1 {
			err := p.disableJob(args.UserId, parameters[0])
			if err != nil {
				p.API.LogError("Error disabling the job.", "job_name", parameters[0], "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Error disabling the job."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' has been disabled.", parameters[0]))
		}
	case "enable":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify a job to enable."), nil
		} else if len(parameters) == 1 {
			err := p.disableJob(args.UserId, parameters[0])
			if err != nil {
				p.API.LogError("Error enabling the job.", "job_name", parameters[0], "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Error enabling the job."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' has been enabled.", parameters[0]))
		}
	case "help":
		text := "###### Mattermost Jenkins Plugin - Slash Command Help\n" + strings.Replace(helpText, "|", "`", -1)
		return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, text), nil
	case "":
		text := "###### Mattermost Jenkins Plugin - Slash Command Help\n" + strings.Replace(helpText, "|", "`", -1)
		return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, text), nil
	case "me":
		userInfo, err := p.getJenkinsUserInfo(args.UserId)
		if err != nil {
			p.API.LogError("Error fetching Jenkins user details", "err", err.Error())
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Encountered an error getting your Jenkins user information."), nil
		}

		text := fmt.Sprintf("You are connected to Jenkins as: %s", userInfo.Username)
		return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, text), nil
	case "get-log":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify a job name or jobname and build number."), nil
		} else if len(parameters) == 1 {
			jobName := parameters[0]
			if err := p.fetchAndUploadBuildLog(args.UserId, args.ChannelId, jobName, ""); err != nil {
				p.API.LogError("Error fetching logs", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Encountered an error fetching logs."), nil
			}
		} else if len(parameters) == 2 {
			jobName := parameters[0]
			buildNumber := parameters[1]
			if err := p.fetchAndUploadBuildLog(args.UserId, args.ChannelId, jobName, buildNumber); err != nil {
				p.API.LogError("Error fetching logs", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Encountered an error fetching logs."), nil
			}
		}
	case "abort":
		if len(parameters) == 0 {
			return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Please specify a job name or jobname and build number."), nil
		} else if len(parameters) == 1 {
			jobName := parameters[0]
			if err := p.abortBuild(args.UserId, jobName, ""); err != nil {
				p.API.LogError("Error aborting Jenkins build", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Encountered an error in aborting the build."), nil
			}

			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Last build of the job %s has been aborted.", jobName))
		} else if len(parameters) == 2 {
			jobName := parameters[0]
			buildNumber := parameters[1]
			if err := p.abortBuild(args.UserId, jobName, buildNumber); err != nil {
				p.API.LogError("Error aborting Jenkins build", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(model.COMMAND_RESPONSE_TYPE_EPHEMERAL, "Encountered an error in aborting the build."), nil
			}

			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Build %s #%s has been aborted.", jobName, buildNumber))
		}
	}
	return &model.CommandResponse{}, nil
}
