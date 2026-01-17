package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBuildParameters(t *testing.T) {
	type ExpectedResponse struct {
		JobName     string
		BuildNumber string
		ExtraParams map[string]string
		Valid       bool
	}

	for name, tc := range map[string]struct {
		Input    []string
		Expected ExpectedResponse
	}{
		"job name": {
			Input:    []string{"jobname"},
			Expected: ExpectedResponse{"jobname", "", nil, true},
		},
		"job name with folder": {
			Input:    []string{"folder/jobname"},
			Expected: ExpectedResponse{"folder/jobname", "", nil, true},
		},
		"with build number": {
			Input:    []string{"jobname", "22"},
			Expected: ExpectedResponse{"jobname", "22", nil, true},
		},
		"with build number and folder": {
			Input:    []string{"folder/jobname", "22"},
			Expected: ExpectedResponse{"folder/jobname", "22", nil, true},
		},
		"with quotes": {
			Input:    []string{`"jobname"`},
			Expected: ExpectedResponse{"jobname", "", nil, true},
		},
		"with quotes and folder": {
			Input:    []string{`"folder/jobname"`, ""},
			Expected: ExpectedResponse{"folder/jobname", "", nil, true},
		},
		"with quotes and build number": {
			Input:    []string{`"jobname"`, "22"},
			Expected: ExpectedResponse{"jobname", "22", nil, true},
		},
		"with quotes, build number and folder": {
			Input:    []string{`"folder/jobname"`, "22"},
			Expected: ExpectedResponse{"folder/jobname", "22", nil, true},
		},
		"with spaces": {
			Input:    []string{`"jobname`, `with`, `spaces"`},
			Expected: ExpectedResponse{"jobname with spaces", "", nil, true},
		},
		"with spaces and folder": {
			Input:    []string{`"folder`, "with", "spaces/and", `job"`},
			Expected: ExpectedResponse{"folder with spaces/and job", "", nil, true},
		},
		"with spaces and build number": {
			Input:    []string{`"jobname`, `with`, `spaces"`, "22"},
			Expected: ExpectedResponse{"jobname with spaces", "22", nil, true},
		},
		"with spaces, folder, and build number": {
			Input:    []string{`"folder`, "with", "spaces/and", `job"`, "22"},
			Expected: ExpectedResponse{"folder with spaces/and job", "22", nil, true},
		},
		"no args": {
			Input:    []string{},
			Expected: ExpectedResponse{"", "", nil, false},
		},
		"with not well structured parameters": {
			Input:    []string{"jobname", "22", "extra-data"},
			Expected: ExpectedResponse{"jobname", "22", nil, true},
		},
		"with well structured parameters": {
			Input:    []string{"jobname", "22", "param1=value1", "param2=value2"},
			Expected: ExpectedResponse{"jobname", "22", map[string]string{"param1": "value1", "param2": "value2"}, true},
		},
		"with well structured parameters and no build number": {
			Input:    []string{"jobname", "param1=value1", "param2=value2"},
			Expected: ExpectedResponse{"jobname", "", map[string]string{"param1": "value1", "param2": "value2"}, true},
		},
	} {
		t.Run(name, func(t *testing.T) {
			job, buildNo, extraParams, valid := parseBuildParameters(tc.Input)
			assert.Equal(t, tc.Expected.JobName, job)
			assert.Equal(t, tc.Expected.BuildNumber, buildNo)
			assert.Equal(t, tc.Expected.ExtraParams, extraParams)
			assert.Equal(t, tc.Expected.Valid, valid)
		})
	}
}
