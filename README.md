# Mattermost Jenkins Plugin

[![Build Status](https://img.shields.io/circleci/project/github/mattermost/mattermost-plugin-jenkins/master.svg)](https://circleci.com/gh/mattermost/mattermost-plugin-jenkins)
[![Code Coverage](https://img.shields.io/codecov/c/github/mattermostmattermost-plugin-jenkins/master.svg)](https://codecov.io/gh/mattermost/mattermost-plugin-jenkins)

A Jenkins plugin to interact with jobs and builds with slash commands in Mattermost. The plugin is currently in beta.

Originally developed by [Wasim Thabraze](https://github.com/waseem18).

<img src="https://raw.githubusercontent.com/waseem18/mattermost-plugin-jenkins/dev/screenshots/Screen%20Shot%202019-05-07%20at%203.30.22%20PM.png" alt="drawing" width="1000"/>



For a Jenkins integration that sends webhook notifications from Jenkins to Mattermost, see this repository: https://github.com/jenkinsci/mattermost-plugin

## Features

This plugin enables you to interact with jobs via slash commands in Mattermost. The supported slash commands are listed below:

#### Connect and disconnect with Jenkins server
* __Connect to Jenkins server__ - `/jenkins connect username APIToken` - Connect your Mattermost account to Jenkins.
* __Disconnect from Jenkins server__ - `/jenkins disconnect` - Disconnect your Mattermost account from Jenkins.

#### Interact with Jenkins jobs
* __Create a Jenkins job__  - `/jenkins createjob` - Create a Jenkins job using contents of `config.xml`. The slash command opens an interactive dialog for the user to input the job name and paste the contents of `config.xml`.
* __Trigger a Jenkins job__ -  `/jenkins build jobname` - Trigger a build for the given job. If the job accepts parameters, an interactive dialog pops up for the user to input the required parameters.
  
  * If the job resides in a folder, specify the job as `folder1/jobname`. Note the slash character.
  * If the folder name or job name has spaces in it, wrap the jobname in double quotes as `"job name with space"` or `"folder with space/jobname"`.
  * Follow similar pattern for all commands which takes jobname as input.

* __Abort a build__ - `/jenkins abort jobname <build number>` - Abort the given build of the specified job. If `build number` is not specified, the command aborts the last build of the job.
* __Enable a job__ -  `/jenkins enable jobname` - Enable a given Jenkins job.
* __Disable a job__ -  `/jenkins disable jobname` - Disable a given Jenkins job.
* __Delete a job__ - `/jenkins delete jobname` - Delete a given job.
* __Get artifacts__ -  `/jenkins get-artifacts jobname` - Get artifacts of the last build of the given job.
* __Get test results__ -  `/jenkins test-results jobname` - Get test results of the last build of the given job.
* __Get build log__ - `/jenkins get-log jobname <build number>` - Get log of a given build of the specified job as a file attachment to the channel. If `build number` is not specified, the command fetches the log of the last build of the job.

#### Interact with Plugins
* __List of installed plugins__ - `/jenkins plugins` - Get a list of installed plugins on Jenkins server along with the version of the plugin.

#### Adhoc commands
* __Safe restart Jenkins server__ - `/jenkins safe-restart` - Safe restart the Jenkins server.
* __Find connected Jenkins account__ -  `/jenkins me` - Display the connected Jenkins account.
* __Get help__ - `/jenkins help` - Find help related to the syntax of the slash commands.

### Installation
1. Install the plugin
    1. Download the latest version of the plugin from the GitHub releases page
    2. In Mattermost, go to **System Console -> Plugins -> Management**
    3. Upload the plugin
2. Enter Jenkins server URL
    1. Go to the **System Console -> Plugins -> Jenkins**
    2. Set the Jenkins server URL along with the protocol. Example: http://jenkins.example.com, https://jenkins.example.com
    3. Save the settings
3. Configure a bot account
    1. Create a new Mattermost user, through the regular UI or the CLI with the username "Jenkins"
    2. Go to the **System Console -> Plugins -> Jenkins** and select this user in the User setting
    3. Save the settings
4. Generate an at rest encryption key
    1. Go to the **System Console -> Plugins -> Jenkins** and click "Regenerate" under "At Rest Encryption Key"
    2. Save the settings
5. Enable the plugin
    1. Go to System Console -> Plugins -> Management and click "Enable" underneath the Jenkins plugin
6. Test it out
    1. In Mattermost, run the slash command `/jenkins connect <Jenkins Username> <Jenkins API Token>`

### Development
```
make
```
This will produce a single plugin file (with support for multiple architectures) that can be uploaded to your Mattermost server:
```
dist/jenkins-0.0.x.tar.gz
```
After the plugin is build, deploy it using Mattermost system console and test it out.

### FAQ
**How do I generate API Token for a given Jenkins user?**

Since Jenkins 2.129 the API token configuration has changed:

You can now have multiple tokens and name them. They can be revoked individually.

 1. Log in to Jenkins.
 2. Click you name (upper-right corner).
 3. Click **Configure** (left-side menu).
 4. Use "Add new Token" button to generate a new one then name it.
 5. You must copy the token when you generate it as you cannot view the token afterwards.
 6. Revoke old tokens when no longer needed.

Before Jenkins 2.129: Show the API token as follows:

1. Log in to Jenkins.
2. Click your name (upper-right corner).
3. Click **Configure** (left-side menu).
4. Click **Show API Token**.

_Source : https://stackoverflow.com/a/45466184/6852930_

### License
MIT
