package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseBuildParameters(t *testing.T) {
	type ExpectedResponse struct {
		JobName     string
		BuildNumber string
		Valid       bool
	}

	for name, tc := range map[string]struct {
		Input    []string
		Expected ExpectedResponse
	}{
		"job name": {
			Input:    []string{"jobname"},
			Expected: ExpectedResponse{"jobname", "", true},
		},
		"job name with folder": {
			Input:    []string{"folder/jobname"},
			Expected: ExpectedResponse{"folder/jobname", "", true},
		},
		"with build number": {
			Input:    []string{"jobname", "22"},
			Expected: ExpectedResponse{"jobname", "22", true},
		},
		"with build number and folder": {
			Input:    []string{"folder/jobname", "22"},
			Expected: ExpectedResponse{"folder/jobname", "22", true},
		},
		"with quotes": {
			Input:    []string{`"jobname"`},
			Expected: ExpectedResponse{"jobname", "", true},
		},
		"with quotes and folder": {
			Input:    []string{`"folder/jobname"`, ""},
			Expected: ExpectedResponse{"folder/jobname", "", true},
		},
		"with quotes and build number": {
			Input:    []string{`"jobname"`, "22"},
			Expected: ExpectedResponse{"jobname", "22", true},
		},
		"with quotes, build number and folder": {
			Input:    []string{`"folder/jobname"`, "22"},
			Expected: ExpectedResponse{"folder/jobname", "22", true},
		},
		"with spaces": {
			Input:    []string{`"jobname`, `with`, `spaces"`},
			Expected: ExpectedResponse{"jobname with spaces", "", true},
		},
		"with spaces and folder": {
			Input:    []string{`"folder`, "with", "spaces/and", `job"`},
			Expected: ExpectedResponse{"folder with spaces/and job", "", true},
		},
		"with spaces and build number": {
			Input:    []string{`"jobname`, `with`, `spaces"`, "22"},
			Expected: ExpectedResponse{"jobname with spaces", "22", true},
		},
		"with spaces, folder, and build number": {
			Input:    []string{`"folder`, "with", "spaces/and", `job"`, "22"},
			Expected: ExpectedResponse{"folder with spaces/and job", "22", true},
		},
		"no args": {
			Input:    []string{},
			Expected: ExpectedResponse{"", "", false},
		},
		"too many args": {
			Input:    []string{"jobname", "22", "extra-data"},
			Expected: ExpectedResponse{"", "", false},
		},
	} {
		t.Run(name, func(t *testing.T) {
			job, buildNo, valid := parseBuildParameters(tc.Input)
			assert.Equal(t, tc.Expected.JobName, job)
			assert.Equal(t, tc.Expected.BuildNumber, buildNo)
			assert.Equal(t, tc.Expected.Valid, valid)
		})
	}
}
