package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/fleetdm/fleet/v4/orbit/pkg/packaging"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service"
	"github.com/go-resty/resty/v2"
)

var s3Client *s3.Client

type CreateInstallersRequest struct {
	TeamName     string   `json:"team_name"`
	EnrollSecret string   `json:"enroll_secret"`
	Packages     []string `json:"packages"`
}

// The 'handler' function is the primary entry-point for the AWS Lambda function
// It takes a request event from AWS API Gateway and a context object,
// and returns a response event with proper HTTP Status Codes.
//
// The function parses the request event into an installers request, initializes a new Fleet server client,
// retrieves and modifies the Enroll Secret Specification from the Fleet server, defines the options for building the packages,
// builds the different packages types as requested, logs all built package identifiers and finally returns an HTTP response.
func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	log.Printf("hello lambda handler")
	// parse the APIGateway event body
	installersRequest, err := parseEventBody(event)
	if err != nil {
		return respondError(fmt.Errorf("failed to parse generate installer request: %w", err))
	}
	response, err := invoke(installersRequest)
	if err != nil {
		return respondError(err)
	}
	return response, nil
}

func invoke(installersRequest CreateInstallersRequest) (events.APIGatewayProxyResponse, error) {
	response := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "\"Hello from Lambda!\"",
	}
	// create a new fleet client
	fleetClient, err := service.NewClient(os.Getenv("FLEET_URL"), false, "", "")
	if err != nil {
		return respondError(fmt.Errorf("failed to create fleet server client: %w", err))
	}
	// set up the fleet client authentication
	fleetClient.SetToken(os.Getenv("FLEET_API_ONLY_USER_TOKEN"))

	restClient := resty.New().SetBaseURL(os.Getenv("FLEET_URL")).SetAuthToken(os.Getenv("FLEET_API_ONLY_USER_TOKEN"))

	type fleetTeam struct {
		Team fleet.Team `json:"team"`
	}
	var team fleetTeam
	var apiErr *apiError
	resp, err := restClient.R().
		SetHeader("Accept", "application/json").
		SetBody(fleet.Team{Name: installersRequest.TeamName}).
		SetError(&apiErr).
		SetResult(&team).
		Post("/api/latest/fleet/teams")
	if err != nil {
		return respondError(err)
	}
	if apiErr != nil {
		return respondError(errorFromAPIError(apiErr))
	}
	// todo make this less lazy
	if resp.StatusCode() != http.StatusOK {
		return respondError(fmt.Errorf("unexpected api response status code: %d", resp.StatusCode()))
	}

	err = os.Mkdir("/tmp/build", 0755)
	if err != nil {
		log.Printf("/tmp/build already exists")
	}

	// default packaging options
	options := packaging.Options{
		FleetURL:            os.Getenv("FLEET_SERVER_URL"),
		EnrollSecret:        team.Team.Secrets[0].Secret, // create the installers with the new enroll secret
		UpdateURL:           "https://tuf.fleetctl.com",
		Identifier:          "com.fleetdm.orbit",
		StartService:        true,
		NativeTooling:       true,
		OrbitChannel:        "stable",
		OsquerydChannel:     "stable",
		DesktopChannel:      "stable",
		OrbitUpdateInterval: 15 * time.Minute,
	}

	var installers []string
	for _, packageType := range installersRequest.Packages {
		switch packageType {
		case "deb":
			pkg, err := buildPackage(packageType, packaging.BuildDeb, options)
			if err != nil {
				return respondError(err)
			}
			installers = append(installers, pkg)
		case "rpm":
			pkg, err := buildPackage(packageType, packaging.BuildRPM, options)
			if err != nil {
				return respondError(err)
			}
			installers = append(installers, pkg)
		case "pkg":
			pkg, err := buildPackage(packageType, packaging.BuildPkg, options)
			if err != nil {
				return respondError(err)
			}
			installers = append(installers, pkg)
		case "msi":
			pkg, err := buildPackage(packageType, packaging.BuildMSI, options)
			if err != nil {
				return respondError(err)
			}
			installers = append(installers, pkg)
		}
	}

	wg := sync.WaitGroup{}
	for _, i := range installers {
		i := i // needed to capture current value of i during for loop fixed in Go 1.22
		go func() {
			wg.Add(1)
			defer wg.Done()
			log.Printf("built %s", i)
			info, err := os.Stat(i)
			if err != nil {
				log.Printf("error getting file info %s: %s\n", i, err)
				return
			}
			log.Printf("file info: %+v\n", info)

			// upload results to S3
			err = uploadArtifact(i, installersRequest.TeamName)
			if err != nil {
				log.Printf("failed to upload to s3: %s", err)
			}
		}()
	}
	wg.Wait()

	return response, nil
}

func uploadArtifact(file string, name string) error {
	bucket := os.Getenv("ARTIFACT_BUCKET")
	if bucket == "" {
		return errors.New("bucket name cannot be empty")
	}
	objectKey := fmt.Sprintf("teamName=%s/%s", name, file)
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	params := &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &objectKey,
		Body:   f,
	}
	_, err = s3Client.PutObject(context.Background(), params)
	if err != nil {
		return err
	}
	log.Println("successfully uploaded to bucket")
	return nil
}

func errorFromAPIError(err *apiError) error {
	if err != nil {
		if len(err.Errors) > 0 {
			messages := make([]string, len(err.Errors))
			for _, msg := range err.Errors {
				messages = append(messages, fmt.Sprintf("name: %s reason: %s", msg.Name, msg.Reason))
			}
			return fmt.Errorf("api error: %s messages: %s", err.Message, strings.Join(messages, ", "))
		}
	}
	return errors.New("no api error defined")
}

// buildPackage is a function that takes a packageType string, a packagerFunc function, and options packaging.Options
// to build a package using the provided packager function with the given options.
// It returns the path of the built package and an error if the packaging process fails.
func buildPackage(packageType string, packagerFunc func(opt packaging.Options) (string, error), options packaging.Options) (string, error) {
	// Invoke the packagerFunc to build the package with the provided options.
	pkg, err := packagerFunc(options)
	if err != nil {
		// If an error occurs during the packaging process, return an error with an informative message.
		return "", fmt.Errorf("failed to package %s: %w", packageType, err)
	}

	// Return the path of the built package and nil for the error.
	return pkg, nil
}

// parseEventBody is a function that takes an event of type events.APIGatewayProxyRequest
// and parses its body into a CreateInstallersRequest struct.
func parseEventBody(event events.APIGatewayProxyRequest) (CreateInstallersRequest, error) {
	// Use json.Unmarshal to unmarshal the event body into a CreateInstallersRequest struct.
	request := CreateInstallersRequest{}
	if err := json.Unmarshal([]byte(event.Body), &request); err != nil {
		return CreateInstallersRequest{}, fmt.Errorf("failed to parse event body: %w", err)
	}

	// Return the parsed request struct and nil for the error.
	return request, nil
}

// The 'respondError' function takes an error as input, formats it as a JSON string, and returns an API Gateway
// proxy response with a status code of 500 (Internal Server Error). If the JSON marshalling of the error fails,
// it returns a predefined error response indicating that marshalling failed.
func respondError(err error) (events.APIGatewayProxyResponse, error) {
	// Initialise an empty map for the response body
	respBody := map[string]string{}

	// Set the error message in the response body
	respBody["error"] = err.Error()

	// Attempt to marshal the response body into JSON
	buf, marshalErr := json.Marshal(respBody)
	if marshalErr != nil {
		// If marshalling failed, return an error response with an appropriate message
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError, Body: "[\"error\":\"failed to marshal err response\"}"}, err
	}

	// If marshalling was successful, return an error response with the original error message
	errResponse := events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError, Body: string(buf)}
	return errResponse, err
}

func main() {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion(os.Getenv("AWS_REGION")))
	if err != nil {
		log.Fatalf("unable to load SDK config, %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)
	if os.Getenv("LOCAL") != "" {
		createInstallersRequest := CreateInstallersRequest{TeamName: "bentestteam", EnrollSecret: "test123", Packages: []string{"deb", "rpm"}}
		buf, _ := json.Marshal(createInstallersRequest)
		fmt.Println(string(buf))
		_, err := invoke(createInstallersRequest)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		lambda.Start(handler)
	}
}

type apiError struct {
	Message string `json:"message"`
	Errors  []struct {
		Name   string `json:"name"`
		Reason string `json:"reason"`
	} `json:"errors"`
}
