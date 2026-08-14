package main

import (
	"context"
	"math/rand/v2"
	"ride-sharing/shared/logger"
	pb "ride-sharing/shared/proto/driver"
	"ride-sharing/shared/util"
	"sync"

	"github.com/mmcloughlin/geohash"
	"go.uber.org/zap"
)

type Service struct {
	drivers []*driverInMap
	mu      sync.RWMutex
}

type driverInMap struct {
	Driver *pb.Driver
	// Index int
	// TODO: route
}

func NewService() *Service {
	return &Service{
		drivers: make([]*driverInMap, 0),
	}
}

func (s *Service) RegisterDriver(ctx context.Context, driverId string, packageSlug string) (*pb.Driver, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	randomIndex := rand.IntN(len(PredefinedRoutes))
	randomRoute := PredefinedRoutes[randomIndex]

	randomPlate := GenerateRandomPlate()
	randomAvatar := util.GetRandomAvatar(randomIndex)

	// we can ignore this property for now, but it must be sent to the frontend.
	geohash := geohash.Encode(randomRoute[0][0], randomRoute[0][1])

	driver := &pb.Driver{
		Id:             driverId,
		Geohash:        geohash,
		Location:       &pb.Location{Latitude: randomRoute[0][0], Longitude: randomRoute[0][1]},
		Name:           "Lando Norris",
		PackageSlug:    packageSlug,
		ProfilePicture: randomAvatar,
		CarPlate:       randomPlate,
	}

	// Add driver to list
	s.drivers = append(s.drivers, &driverInMap{
		Driver: driver,
	})

	logger.WithTrace(ctx).Info("driver registered", zap.String("driver_id", driverId), zap.String("plate", randomPlate))
	return driver, nil
}

func (s *Service) UnregisterDriver(driverId string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Filter driver from list
	for i, driver := range s.drivers {
		if driver.Driver.Id == driverId {
			s.drivers = append(s.drivers[:i], s.drivers[i+1:]...)
		}
	}
}

func (s *Service) FindAvailableDrivers(packageType string) []string {
	var matchingDrivers []string

	for _, driver := range s.drivers {
		if driver.Driver.PackageSlug == packageType {
			matchingDrivers = append(matchingDrivers, driver.Driver.Id)
		}
	}

	if len(matchingDrivers) == 0 {
		return []string{}
	}

	return matchingDrivers
}
