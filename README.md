# mattermost-plugin-jenkins
A Mattermost plugin to interact with Jenkins

### Slash Commands
* __Connect to Jenkins server__ - `/jenkins connect username APIToken` - Connect your Mattermost account to Jenkins.

* __Connect to Jenkins server__ -  `/jenkins build jobname` - Trigger a job build without parameters. (Triggering jobs with parameters will be added in further releases.)
  
  * If the job resides in a folder, specify the job as `folder1/jobname`. Note the slash character.
  
  * If the folder name or job name has spaces in it, wrap the jobname in double quotes as `"job name with space"` or `"folder with space/jobname"`.
  
  * Follow similar pattern for all commands which takes jobname as input.
  
* __Get artifacts__ -  `/jenkins get-artifacts jobname` - Get artifacts of the last build of the given job.

* __Get test results__ -  `/jenkins test-results jobname` - Get test results of the last build of the given job.

* __Disable a Jenkins job__ -  `/jenkins disable jobname` - Disable a given job.

* __Enable a Jenkins job__ -  `/jenkins enable jobname` - Enanble a given job.

* __Find connected Jenkins account__ -  `/jenkins me` - Display the connected Jenkins account.


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
