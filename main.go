package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/fleetdm/fleet/v4/orbit/pkg/packaging"
	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/service"
)

type CreateInstallersRequest struct {
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
	response := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "\"Hello from Lambda!\"",
	}

	// parse the APIGateway event body
	installersRequest, err := parseEventBody(event)
	if err != nil {
		return respondError(fmt.Errorf("failed to parse generate installer request: %w", err))
	}

	// create a new fleet client
	fleetClient, err := service.NewClient(os.Getenv("FLEET_URL"), false, "", "")
	if err != nil {
		return respondError(fmt.Errorf("failed to create fleet server client: %w", err))
	}
	// set up the fleet client authentication
	fleetClient.SetToken(os.Getenv("FLEET_API_ONLY_USER_TOKEN"))

	// get the current server enroll secrets
	enrollSecretSpec, err := fleetClient.GetEnrollSecretSpec()
	if err != nil {
		return respondError(fmt.Errorf("failed to obtain secret spec: %w", err))
	}

	// add the secret coming in from the APIGateway request to the current list of secrets and apply
	enrollSecretSpec.Secrets = append(enrollSecretSpec.Secrets, &fleet.EnrollSecret{Secret: installersRequest.EnrollSecret})
	err = fleetClient.ApplyEnrollSecretSpec(enrollSecretSpec)
	if err != nil {
		return respondError(fmt.Errorf("fialed to apply new enroll secret spec: %w", err))
	}

	// default packaging options
	options := packaging.Options{
		FleetURL:            os.Getenv("FLEET_SERVER_URL"),
		EnrollSecret:        installersRequest.EnrollSecret, // create the installers with the new enroll secret
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

	for _, i := range installers {
		log.Printf("built %s", i)
	}

	return response, nil
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
	buf, err := json.Marshal(respBody)
	if err != nil {
		// If marshalling failed, return an error response with an appropriate message
		return events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError, Body: "[\"error\":\"failed to marshal err response\"}"}, err
	}

	// If marshalling was successful, return an error response with the original error message
	errResponse := events.APIGatewayProxyResponse{StatusCode: http.StatusInternalServerError, Body: string(buf)}
	return errResponse, err
}

func main() {
	lambda.Start(handler)
}
