package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/fleetdm/fleet/v4/orbit/pkg/packaging"
)

func handler(ctx context.Context, event events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	response := events.APIGatewayProxyResponse{
		StatusCode: 200,
		Body:       "\"Hello from Lambda!\"",
	}

	// TODO figure out how we're going to interface with lambda (API GW, direct Invoke, etc.)
	fmt.Println("you are in the packager")
	options := packaging.Options{
		FleetURL:            fmt.Sprintf("https://%s.%s", "abc123", "fleetdm.com"),
		EnrollSecret:        "test123",
		UpdateURL:           "https://tuf.fleetctl.com",
		Identifier:          "com.fleetdm.orbit",
		StartService:        true,
		NativeTooling:       true,
		OrbitChannel:        "stable",
		OsquerydChannel:     "stable",
		DesktopChannel:      "stable",
		OrbitUpdateInterval: 15 * time.Minute,
	}
	deb, err := packaging.BuildDeb(options)
	if err != nil {
		log.Panic(err)
	}
	fmt.Printf("deb built: %s", deb)

	return response, nil
}

func main() {
	lambda.Start(handler)
}
