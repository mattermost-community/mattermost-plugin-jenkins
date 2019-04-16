# mattermost-plugin-jenkins
A Mattermost plugin to interact with Jenkins

### Mattermost Jenkins Plugin - Slash Commands
- `/jenkins connect username APIToken` - Connect your Mattermost account to Jenkins.

- `/jenkins build jobname` - Trigger a job build without parameters. (Triggering jobs with parameters will be added in further releases.)
  
  - If the job resides in a folder, specify the job as `folder1/jobname`. Note the slash character.
  
  - If the folder name or job name has spaces in it, wrap the jobname in double quotes as `"job name with space"` or `"folder with space/jobname"`.
  
  - Follow similar pattern for all commands which takes jobname as input.
  
- `/jenkins get-artifacts jobname` - Get artifacts of the last build of the given job.

- `/jenkins test-results jobname` - Get test results of the last build of the given job.

- `/jenkins disable jobname` - Disable a given job.

- `/jenkins enable jobname` - Enanble a given job.

- `/jenkins me` - Display the connected Jenkins account.
