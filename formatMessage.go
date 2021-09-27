package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	html "html/template"
	"io"
	"os"
	"strconv"
	text "text/template"
	"time"

	"github.com/cyverse-de/logcabin"
	"github.com/mitchellh/mapstructure"
)

// email request received
type EmailRequest struct {
	FromAddr string
	To       string
	Template string
	Subject  string
	Values   json.RawMessage
}

type Templater interface {
	Execute(io.Writer, interface{}) error
}

// VICERequestCompleteDetails contains the request detail fields that we need to extract when a VICE access request is
// marked as complete.
type VICERequestCompleteDetails struct {
	ConcurrentJobs int64  `mapstructure:"concurrent_jobs"`
	UseCase        string `mapstructure:"intended_use"`
}

// ToolRequestDetails contains the request detail fields that we need to extract for tool requests.
type ToolRequestDetails struct {
	Description   string `mapstructure:"description"`
	Documentation string `mapstructure:"documentation_url"`
	Source        string `mapstructure:"source_url"`
	Name          string `mapstructure:"name"`
	TestData      string `mapstructure:"test_data_path"`
	SubmittedBy   string `mapstructure:"submitted_by"`
}

// RequestSubmittedDetails contains the request detail fields that we need to extract for request submissions.
type RequestSubmittedDetails struct {
	Name           string `mapstructure:"name"`
	Email          string `mapstructure:"email"`
	UseCase        string `mapstructure:"intended_use"`
	ConcurrentJobs int64  `mapstructure:"concurrent_jobs"`
}

// ExtractDetails extracts fields from a nested object in the payload.
func ExtractDetails(payload map[string]interface{}, dest interface{}, fieldNames ...string) error {
	for _, fieldName := range fieldNames {
		source, ok := payload[fieldName]
		if ok && source != nil {
			return mapstructure.Decode(source, dest)
		}
	}

	return fmt.Errorf("required payload field not found: %s", fieldNames)
}

//format email message using templates
func FormatMessage(emailReq EmailRequest, payload map[string](interface{}), deSettings DESettings) (bytes.Buffer, bool, error) {
	logcabin.Info.Println("Received formatting request with template " + emailReq.Template)
	var template_output bytes.Buffer

	payload["DELink"] = deSettings.base
	payload["DETeamsLink"] = deSettings.base + deSettings.teams
	payload["DEAdminDoiRequestLink"] = deSettings.base + deSettings.admin + deSettings.doi
	payload["DEToolsLink"] = deSettings.base + deSettings.tools
	payload["DECollectionsLink"] = deSettings.base + deSettings.collections
	payload["DEAppsLink"] = deSettings.base + deSettings.apps
	payload["DEPublicationRequestsLink"] = deSettings.base + deSettings.admin + deSettings.apps
	payload["DEPidRequestLink"] = deSettings.base + deSettings.admin + deSettings.doi

	var isHtml bool = false
	var tmpl Templater
	var err error

	if _, err = os.Stat("./templates/html/" + emailReq.Template + ".tmpl"); err == nil {
		isHtml = true
		tmpl, err = html.ParseFiles("./templates/html/"+emailReq.Template+".tmpl", "./templates/html/header.tmpl", "./templates/html/footer.tmpl")
	} else if _, err = os.Stat("./templates/text/" + emailReq.Template + ".tmpl"); err == nil {
		tmpl, err = text.ParseFiles("./templates/text/" + emailReq.Template + ".tmpl")
	}

	// this will catch errors thrown by the if conditions or within the code blocks
	if err != nil {
		logcabin.Error.Println(err)
		return template_output, isHtml, err
	}

	switch emailReq.Template {
	case "analysis_status_change":
		var startDateText, resultFolderPath string

		// Format the analysis start date.
		err = ExtractDetails(payload, &startDateText, "startdate")
		if err != nil {
			logcabin.Error.Printf("unable to extract the analysis start date: %s", err)
			startDateText = ""
		}
		mill_sec, parse_err := strconv.ParseInt(startDateText, 10, 64)
		if parse_err != nil {
			logcabin.Error.Println(parse_err)
		}
		start_date := time.Unix(0, mill_sec*int64(time.Millisecond))
		payload["startdate"] = start_date

		// Format the link to the analysis result folder.
		err = ExtractDetails(payload, &resultFolderPath, "analysisresultsfolder", "result_folder_path")
		if err != nil {
			logcabin.Error.Printf("unable to extract the analysis result folder path: %s", err)
		}
		payload["DEOutputFolderLink"] = deSettings.base + deSettings.data + resultFolderPath

	case "added_to_team":
		var teamName string
		err = ExtractDetails(payload, &teamName, "team_name")
		if err != nil {
			logcabin.Error.Printf("unable to extract the team name: %s", err)
		}

		payload["DETeamsLink"] = deSettings.base + deSettings.teams + "/" + payload["team_name"].(string)

	case "request_complete", "request_rejected":
		if payload["request_type"].(string) == "vice" {
			var viceRequestDetails VICERequestCompleteDetails
			err := ExtractDetails(payload, &viceRequestDetails, "request_details")
			if err != nil {
				return template_output, isHtml, err
			}
			payload["ConcurrentJobs"] = viceRequestDetails.ConcurrentJobs
			payload["UseCase"] = viceRequestDetails.UseCase
			payload["DEAppsLink"] = deSettings.base + deSettings.apps + "?selectedFilter={\"value\":\"Interactive\",\"display\":\"VICE\"}&selectedCategory={\"name\":\"Browse All Apps\",\"id\":\"pppppppp-pppp-pppp-pppp-pppppppppppp\"}"
		}
	case "tool_request":
		var reqDetails ToolRequestDetails
		err := ExtractDetails(payload, &reqDetails, "toolrequestdetails")
		if err != nil {
			return template_output, isHtml, err
		}
		payload["user"] = "Admin"
		payload["Description"] = reqDetails.Description
		payload["Documentation"] = reqDetails.Documentation
		payload["Source"] = reqDetails.Source
		payload["Name"] = reqDetails.Name
		payload["TestData"] = reqDetails.TestData
		payload["SubmittedBy"] = reqDetails.SubmittedBy
		payload["DEToolRequestLink"] = deSettings.base + deSettings.admin + deSettings.tools

	case "request_submitted":
		//reqDetails := payload["request_details"].(map[string]interface{})
		var reqDetails RequestSubmittedDetails
		err := ExtractDetails(payload, &reqDetails, "request_details")
		if err != nil {
			return template_output, isHtml, err
		}
		payload["Name"] = reqDetails.Name
		payload["Email"] = reqDetails.Email
		payload["UseCase"] = reqDetails.UseCase
		payload["ConcurrentJobs"] = reqDetails.ConcurrentJobs
		payload["user"] = "Admin"
	}

	tmpl_err := tmpl.Execute(&template_output, payload)
	if tmpl_err != nil {
		logcabin.Error.Println(tmpl_err)
	}
	return template_output, isHtml, tmpl_err

}
