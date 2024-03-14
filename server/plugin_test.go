package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
)

func TestGetJob(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(http.StatusOK)
	}))
	assert.NotNil(t, testServer)
	defer testServer.Close()

	p := &Plugin{}
	api := &plugintest.API{}
	p.SetAPI(api)

	userInfo := &JenkinsUserInfo{
		UserID:   "user1",
		Username: "username1",
		Token:    "i1BmOxqUYk_6MtXJNTUtJIQbH2VikZkGPPycfIJhAaY=",
	}

	kvData, err := json.Marshal(userInfo)
	assert.Nil(t, err)

	api.On("KVGet", "user1"+jenkinsTokenKey).Return(kvData, nil)

	conf := &configuration{
		JenkinsURL:    testServer.URL,
		EncryptionKey: "enckeyenckeyenckeyenckey",
	}
	serverConf := &model.Config{}
	p.setConfiguration(conf, serverConf)

	c, err := p.getJenkinsClient("user1")
	assert.Nil(t, err)
	assert.NotNil(t, c)

	j, err := p.getJob("user1", "job1")
	assert.Nil(t, err)
	assert.NotNil(t, j)
	assert.Equal(t, "/job/job1", j.Base)
}
