package main

import (
	"fmt"
	"strings"

	"github.com/mattermost/mattermost-server/v5/model"
	"github.com/mattermost/mattermost-server/v5/plugin"
)

const helpText = `
###### Connect and disconnect with Jenkins server
* |/jenkins connect username APIToken| - Connect your Mattermost account to Jenkins.
* |/jenkins disconnect| - Disconnect your Mattermost account with Jenkins.

###### Interact with Jenkins jobs
* |/jenkins createjob| - Create a job using config.xml.
* |/jenkins build jobname| - Trigger a build for the given job.
  * If the job resides in a folder, specify the job as |folder1/jobname|. Note the slash character.
  * If the folder name or job name has spaces in it, wrap the jobname in double quotes as |"job name with space"| or |"folder with space/jobname"|.
  * Follow similar patterns for all commands which takes jobname as input.
  * Use double quotes only when there are spaces in the job name or folder name.
* |/jenkins abort jobname <build number>| - Abort the build of a given job.
  * If build number is not specified, the command aborts the last running build.
* |/jenkins enable jobname| - Enanble a given job.
* |/jenkins disable jobname| - Disable a given job.
* |/jenkins delete jobname| - Deletes a given job.
* |/jenkins get-artifacts jobname| - Get artifacts of the last build of the given job.
* |/jenkins test-results jobname| - Get test results of the last build of the given job.
* |/jenkins get-log jobname <build number>| - Get build log of a given job. Build number is optional.
  * If build number is not specified, the command fetches the log of the last build.

###### Interact with Plugins
* |/jenkins plugins| - Get a list of installed plugins on the Jenkins server.

###### Adhoc Commands
* |/jenkins safe-restart| - Safe restarts the Jenkins server.
* |/jenkins me| - Display the connected Jenkins account.
* |/jenkins help| - Find help related to the syntax of the slash commands.
`
const jobNotSpecifiedResponse = "Please specify a job name to build."
const pollingSleepTime = 10

func getCommand() *model.Command {
	return &model.Command{
		Trigger:          "jenkins",
		Description:      "A Mattermost plugin to interact with Jenkins",
		DisplayName:      "Jenkins",
		AutoComplete:     true,
		AutoCompleteDesc: "Available commands: connect, disconnect, me, build, get-artifacts, test-results, get-log, abort, disable, enable, delete, safe-restart, plugins, createjob, help",
		AutoCompleteHint: "[command]",
	}
}

func (p *Plugin) postCommandResponse(args *model.CommandArgs, text string) {
	botUserID := p.botUserID
	post := &model.Post{
		UserId:    botUserID,
		ChannelId: args.ChannelId,
		Message:   text,
	}
	_ = p.API.SendEphemeralPost(args.UserId, post)
}

func (p *Plugin) getCommandResponse(args *model.CommandArgs, text string) *model.CommandResponse {
	p.postCommandResponse(args, text)
	return &model.CommandResponse{}
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
			return p.getCommandResponse(args, "Please specify both username and API token."), nil
		} else if len(parameters) == 2 {
			p.createEphemeralPost(args.UserId, args.ChannelId, "Validating Jenkins credentials...")
			_, verifyErr := p.verifyJenkinsCredentials(parameters[0], parameters[1])
			if verifyErr != nil {
				p.API.LogError("Error connecting to Jenkins", "user_id", args.UserId, "Err", verifyErr.Error())
				return p.getCommandResponse(args, "Error connecting to Jenkins."), nil
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

			return p.getCommandResponse(args, "Your Jenkins account has been successfully connected to Mattermost."), nil
		}
	case "build":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, extraParam, ok := parseBuildParameters(parameters)
			if !ok || extraParam != "" {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to get trigger a job."), nil
			}

			hasParameters, paramErr := p.checkIfJobAcceptsParameters(args.UserId, jobName)
			if paramErr != nil {
				p.API.LogError("Error checking for parameters", "err", paramErr.Error())
				return p.getCommandResponse(args, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
			}

			if hasParameters {
				err := p.createDialogForParameters(args.UserId, args.TriggerId, jobName, args.ChannelId)
				if err != nil {
					p.API.LogError("Error creating dialogue", "err", err.Error())
					return p.getCommandResponse(args, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
				}
			} else {
				build, err := p.triggerJenkinsJob(args.UserId, args.ChannelId, jobName, nil)
				if err != nil {
					p.API.LogError("Error triggering build", "job_name", jobName, "err", err.Error())
					return p.getCommandResponse(args, fmt.Sprintf("Error triggering build for the job '%s'.", jobName)), nil
				}
				p.createPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' - #%d has been started\nBuild URL : %s", jobName, build.GetBuildNumber(), build.GetUrl()))
			}
		}
	case "get-artifacts":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, buildNumber, ok := parseBuildParameters(parameters)
			if !ok {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to get artifacts of a build."), nil
			}
			msg := ""
			if buildNumber == "" {
				msg = fmt.Sprintf("Fetching artifacts of the last build of the job '%s'...", jobName)
				p.createEphemeralPost(args.UserId, args.ChannelId, msg)
			} else {
				msg = fmt.Sprintf("Fetching artifacts of the build #%s of the job '%s'...", buildNumber, jobName)
				p.createEphemeralPost(args.UserId, args.ChannelId, msg)
			}

			if err := p.fetchAndUploadArtifactsOfABuild(args.UserId, args.ChannelId, jobName, buildNumber); err != nil {
				p.API.LogError("Error fetching artifacts", "job_name", parameters[0], "err", err.Error())
				return p.getCommandResponse(args, "Error fetching artifacts."), nil
			}
		}
	case "test-results":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, jobNotSpecifiedResponse), nil
		} else if len(parameters) >= 1 {
			jobName, buildNumber, ok := parseBuildParameters(parameters)
			if !ok {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to get test results of a build."), nil
			}
			msg := ""
			if buildNumber == "" {
				msg = fmt.Sprintf("Fetching test results of the last build of the job '%s'...", jobName)
				p.createEphemeralPost(args.UserId, args.ChannelId, msg)
			} else {
				msg = fmt.Sprintf("Fetching test results of the build #%s of the job '%s'...", buildNumber, jobName)
				p.createEphemeralPost(args.UserId, args.ChannelId, msg)
			}

			if err := p.getBuildTestResultsURL(args.UserId, args.ChannelId, jobName, buildNumber); err != nil {
				p.API.LogError("Error fetching test results", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Error fetching test results."), nil
			}
		}
	case "disable":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, "Please specify a job to disable."), nil
		} else if len(parameters) >= 1 {
			jobName, extraParam, ok := parseBuildParameters(parameters)
			if !ok || extraParam != "" {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to disable a job."), nil
			}

			if err := p.disableJob(args.UserId, jobName); err != nil {
				p.API.LogError("Error disabling the job.", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Error disabling the job."), nil
			}
			p.createPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' has been disabled", jobName))
		}
	case "enable":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, "Please specify a job to enable."), nil
		} else if len(parameters) >= 1 {
			jobName, extraParam, ok := parseBuildParameters(parameters)
			if !ok || extraParam != "" {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to enable a job."), nil
			}
			if err := p.enableJob(args.UserId, jobName); err != nil {
				p.API.LogError("Error enabling the job.", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Error enabling the job."), nil
			}
			p.createPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' has been enabled", jobName))
		}
	case "help":
		text := "###### Mattermost Jenkins Plugin - Slash Command Help\n" + strings.Replace(helpText, "|", "`", -1)
		return p.getCommandResponse(args, text), nil
	case "":
		text := "###### Mattermost Jenkins Plugin - Slash Command Help\n" + strings.Replace(helpText, "|", "`", -1)
		return p.getCommandResponse(args, text), nil
	case "me":
		userInfo, err := p.getJenkinsUserInfo(args.UserId)
		if err != nil {
			p.API.LogError("Error fetching Jenkins user details", "err", err.Error())
			return p.getCommandResponse(args, "Encountered an error getting your Jenkins user information."), nil
		}
		return p.getCommandResponse(args, fmt.Sprintf("You are connected to Jenkins as: %s", userInfo.Username)), nil
	case "disconnect":
		userInfo, err := p.getJenkinsUserInfo(args.UserId)
		if err != nil {
			p.API.LogError("Error fetching Jenkins user details", "err", err.Error())
			return p.getCommandResponse(args, "Encountered an error getting your Jenkins user information."), nil
		}

		if err := p.API.KVDelete(args.UserId + jenkinsTokenKey); err != nil {
			p.API.LogError("Error disconnecting the user", "err", err.Error())
			return p.getCommandResponse(args, "Encountered an error while disconnecting the user from Jenkins."), nil
		}
		return p.getCommandResponse(args, fmt.Sprintf("User '%s' has been disconnected.", userInfo.Username)), nil
	case "get-log":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, "Please specify a job name or jobname and build number."), nil
		} else if len(parameters) >= 1 {
			jobName, buildNumber, ok := parseBuildParameters(parameters)
			if !ok {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to get log of a build."), nil
			}
			p.createEphemeralPost(args.UserId, args.ChannelId, fmt.Sprintf("Fetching logs of job '%s'...", jobName))

			if err := p.fetchAndUploadBuildLog(args.UserId, args.ChannelId, jobName, buildNumber); err != nil {
				p.API.LogError("Error fetching logs", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Encountered an error fetching logs."), nil
			}
		}
	case "abort":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, "Please specify a job name or jobname and build number."), nil
		} else if len(parameters) >= 1 {
			jobName, buildNumber, ok := parseBuildParameters(parameters)
			if !ok {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to abort a build."), nil
			}

			if err := p.abortBuild(args.UserId, jobName, buildNumber); err != nil {
				p.API.LogError("Error aborting Jenkins build", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Encountered an error in aborting the build."), nil
			}
			msg := ""
			if buildNumber == "" {
				msg = fmt.Sprintf("Last build of the job '%s' has been aborted.", jobName)
			} else {
				msg = fmt.Sprintf("Build #%s of the job '%s' has been aborted.", buildNumber, jobName)
			}

			p.createPost(args.UserId, args.ChannelId, msg)
		}
	case "delete":
		if len(parameters) == 0 {
			return p.getCommandResponse(args, "Please specify a job name or jobname and build number."), nil
		} else if len(parameters) >= 1 {
			jobName, extraParam, ok := parseBuildParameters(parameters)
			if !ok || extraParam != "" {
				return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to delete a job."), nil
			}

			if err := p.deleteJob(args.UserId, jobName); err != nil {
				p.API.LogError("Error deleting the job", "job_name", jobName, "err", err.Error())
				return p.getCommandResponse(args, "Encountered an error while deleting the job."), nil
			}

			p.createPost(args.UserId, args.ChannelId, fmt.Sprintf("Job '%s' has been deleted.", jobName))
		}
	case "safe-restart":
		if len(parameters) != 0 {
			return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to safe restart Jenkins."), nil
		}
		if err := p.safeRestart(args.UserId); err != nil {
			p.API.LogError("Error while safe restarting the Jenkins server", err.Error())
			return p.getCommandResponse(args, "Encountered an error while safe restarting the Jenkins server."), nil
		}
		p.createPost(args.UserId, args.ChannelId, "Safe restart of Jenkins server has been triggered.")
	case "plugins":
		if len(parameters) != 0 {
			return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to get a list of plugins."), nil
		}
		if err := p.getListOfInstalledPlugins(args.UserId, args.ChannelId); err != nil {
			p.API.LogError("Error while fetching list of installed plugins", err.Error())
			return p.getCommandResponse(args, "Encountered an error while fetching list of installed plugins"), nil
		}
	case "createjob":
		if len(parameters) != 0 {
			return p.getCommandResponse(args, "Please check `/jenkins help` to find help on how to create a job."), nil
		}
		if err := p.createJob(args.UserId, args.ChannelId, args.TriggerId); err != nil {
			p.API.LogError("Error while creating the job.", err.Error())
			return p.getCommandResponse(args, "Encountered an error while creating the job"), nil
		}
	default:
		text := "###### Unknown Command: " + action + "\n" + "###### Mattermost Jenkins Plugin - Slash Command Help\n" + strings.Replace(helpText, "|", "`", -1)
		return p.getCommandResponse(args, text), nil
	}
	return &model.CommandResponse{}, nil
}
