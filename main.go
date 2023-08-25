package main

import (
	"fmt"
	"log"
	"time"

	"github.com/fleetdm/fleet/v4/orbit/pkg/packaging"
)

func main() {

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

}
