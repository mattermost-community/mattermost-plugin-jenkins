# mattermost-plugin-jenkins
A Mattermost plugin to interact with Jenkins

## Features


#### Slash commands to connect and disconnect with Jenkins server
* __Connect to Jenkins server__ - `/jenkins connect username APIToken` - Connect your Mattermost account to Jenkins.
* __Disconnect from Jenkins server__ - `/jenkins disconnect` - Disconnect your Mattermost account with Jenkins.


#### Slash Commands to interact with Jobs
* __Create a Jenkins job__  - `/jenkins createjob` - Create a Jenkins job using contents of `config.xml`. The slash command opens an interactive dialog for the user to input the job name and paste the contents of `config.xml`.
* __Trigger a Jenkins job__ -  `/jenkins build jobname` - Trigger a build for the given job. If the job accepts parameters, an interactive dialog pops up for the user to input the required parameters.
  
  * If the job resides in a folder, specify the job as `folder1/jobname`. Note the slash character.
  
  * If the folder name or job name has spaces in it, wrap the jobname in double quotes as `"job name with space"` or `"folder with space/jobname"`.
  
  * Follow similar pattern for all commands which takes jobname as input.

* __Delete  a job__ - `/jenkins delete jobname` - Deletes a given job.
* __Get artifacts__ -  `/jenkins get-artifacts jobname` - Get artifacts of the last build of the given job.

* __Get test results__ -  `/jenkins test-results jobname` - Get test results of the last build of the given job.

* __Get build log__ - `/jenkins get-log jobname <build number>` - Get log of a given build of the specified job as a file attachment to the channel. If `build number` is not specified, the command fetches the log of the last build of the job.
* __Abort a build__ - `/jenkins abort jobname <build number>` - Aborts the given build of the specified job. If `build number` is not specified, the command aborts the last build of the job.

* __Disable a Jenkins job__ -  `/jenkins disable jobname` - Disable a given Jenkins job.

* __Enable a Jenkins job__ -  `/jenkins enable jobname` - Enanble a given Jenkins job.

#### Slash Command to interact with Plugins
* __List of installed plugins__ - `/jenkins plugins` - Get a list of installed plugins on Jenkins server along with the version of the plugin.

#### Adhoc commands
* __Safe restart Jenkins server__ - `/jenkins safe-restart` - Safe restart the Jenkins server.
* __Find connected Jenkins account__ -  `/jenkins me` - Display the connected Jenkins account.
* __Get help__ - `/jenkins help` - Find help related to the syntax of the slash commands.

### Installation
1. Install the plugin
    1. Download the latest version of the plugin from the GitHub releases page.
    2. In Mattermost, go the System Console -> Plugins -> Management.
    3. Upload the plugin.
2. Enter Jenkins server URL
    1. Go to the System Console -> Plugins -> Jenkins
    2. Set the Jenkins server URL along with the protocol. Example: http://jenkins.example.com, https://jenkins.example.com
    3. Save the settings
3. Configure a bot account
    1. Create a new Mattermost user, through the regular UI or the CLI with the username "Jenkins"
    2. Go to the System Console -> Plugins -> Jenkins and select this user in the User setting
    3. Save the settings
4. Generate an at rest encryption key
    1. Go to the System Console -> Plugins -> Jenkins and click "Regenerate" under "At Rest Encryption Key"
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

### License
MIT
