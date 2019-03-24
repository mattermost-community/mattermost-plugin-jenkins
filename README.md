# mattermost-plugin-jenkins
A Mattermost plugin to interact with Jenkins

# Plugin commands
* `/jenkins connect <Jenkins username> <Jenkins API token>` - Use this command to connect your Mattermost account to Jenkins.
Please [follow this link](https://stackoverflow.com/a/45466184/6852930) to find help regarding API token creation.
* `/jenkins build jobName` - Use this command to trigger a job with the name jobName.
* `/jenkins build jobName &param1=value1 &param2=value2` - Use this command to trigger a parameterized job. 
* `/jenkins build "job name with space"` - Use double quotes to specify a job with spaces in it.
* `/jenkins build folderName/jobName` - Use this command to specify to specify a job which resides inside folders.
* `/jenkins build "folder name/ job name"` - Use double quotes when specifying a job with spaces.