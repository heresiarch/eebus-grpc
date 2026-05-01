// SPDX-License-Identifier: Apache-2.0
// Copyright Pionix GmbH and Contributors to EVerest

package control_service_server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/enbility/eebus-go/api"
	"github.com/enbility/eebus-go/service"
	"github.com/enbility/eebus-grpc/eebus_service"
	"github.com/enbility/eebus-grpc/rpc_server"
	"github.com/enbility/eebus-grpc/rpc_server/usecase_server"
	cs_lpc_server "github.com/enbility/eebus-grpc/rpc_server/usecase_server/cs/lpc"
	eg_lpc_server "github.com/enbility/eebus-grpc/rpc_server/usecase_server/eg/lpc"
	"github.com/enbility/eebus-grpc/rpc_services/control_service"
	shipapi "github.com/enbility/ship-go/api"
	"github.com/enbility/spine-go/model"
	"github.com/enbility/spine-go/spine"
	"github.com/looplab/fsm"
	"google.golang.org/grpc"

	// Use cases - CEM

	// Use cases - CS

	// Use cases - EG

	// Use cases - MA

	// Conversion functions

	log "github.com/enbility/eebus-grpc/utils/logging"
	"github.com/enbility/eebus-grpc/utils/type_conversions"
)

type useCaseEntryType struct {
	entityAddress []model.AddressEntityType
	useCase       *control_service.UseCase
	useCaseServer usecase_server.UseCaseServer
}

func (h *useCaseEntryType) Equals(other *useCaseEntryType) bool {
	// compare entity address
	if len(h.entityAddress) != len(other.entityAddress) {
		return false
	}
	for i, address := range h.entityAddress {
		if address != other.entityAddress[i] {
			return false
		}
	}
	// compare use case
	if h.useCase.GetActor() != other.useCase.GetActor() {
		return false
	}
	if h.useCase.GetName() != other.useCase.GetName() {
		return false
	}
	return true
}

type Server struct {
	control_service.UnsafeControlServiceServer
	rpc_server.RPCServer
	eebusService *eebus_service.Service
	stateMachine *fsm.FSM
	grpcServer   *grpc.Server
	certificate  *tls.Certificate

	useCaseRegistry []useCaseEntryType

	log.Logger
}

func NewServer(eebusService *eebus_service.Service, certificate *tls.Certificate) *Server {
	return &Server{
		eebusService: eebusService,
		stateMachine: NewStateMachine(),
		grpcServer:   nil,
		certificate:  certificate,
	}
}

func (h *Server) StartService(_ context.Context, _ *control_service.EmptyRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received start command")
	if h.eebusService.IsRunning() {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is already running")
	}
	if h.stateMachine.Cannot(EventStart) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not ready")
	}
	h.eebusService.Start()
	if !h.eebusService.IsRunning() {
		return &control_service.EmptyResponse{}, fmt.Errorf("Failed to start service")
	}
	h.stateMachine.Event(context.Background(), EventStart)
	return &control_service.EmptyResponse{}, nil
}

func (h *Server) StopService(_ context.Context, _ *control_service.EmptyRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received stop command")
	if !h.eebusService.IsRunning() {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not running")
	}
	if h.stateMachine.Cannot(EventStop) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not running")
	}
	h.eebusService.Shutdown()
	if h.eebusService.IsRunning() {
		return &control_service.EmptyResponse{}, fmt.Errorf("Failed to stop service")
	}
	h.stateMachine.Event(context.Background(), EventStop)
	return &control_service.EmptyResponse{}, nil
}

func (h *Server) ResetService(_ context.Context, _ *control_service.EmptyRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received reset command")
	h.eebusService.Shutdown()

	h.stateMachine = NewStateMachine()

	return &control_service.EmptyResponse{}, nil
}

func (h *Server) SetConfig(_ context.Context, req *control_service.SetConfigRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received set config command")
	if !h.stateMachine.Is(StateSetup) {
		h.Error("Service is not in setup state")
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not in setup state")
	}
	var device_categories []shipapi.DeviceCategoryType
	for _, category := range req.GetDeviceCategories() {
		device_categories = append(device_categories, shipapi.DeviceCategoryType(category))
	}

	var entity_types []model.EntityTypeType
	for _, entity_type := range req.GetEntityTypes() {
		entity_types = append(entity_types, model.EntityTypeType(entity_type.String()))
	}
	configuration, err := api.NewConfiguration(
		req.GetVendorCode(),
		req.GetDeviceBrand(),
		req.GetDeviceModel(),
		req.GetSerialNumber(),
		device_categories,
		model.DeviceTypeType(req.DeviceType.String()),
		entity_types,
		int(req.GetPort()),
		*h.certificate,
		time.Duration(req.GetHeartbeatTimeoutSeconds())*time.Second,
	)
	if err != nil {
		h.Errorf("Failed to create configuration: %v", err)
		return &control_service.EmptyResponse{}, fmt.Errorf("Failed to create configuration: %v", err)
	}
	h.eebusService.Service = *service.NewService(configuration, h.eebusService)
	h.eebusService.SetLogging(h)

	h.Debug("Successfully set configuration")
	return &control_service.EmptyResponse{}, nil
}

func (h *Server) StartSetup(_ context.Context, _ *control_service.EmptyRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received start setup command")
	if h.stateMachine.Cannot(EventReady) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not in setup state")
	}
	var err error
	err = h.eebusService.Setup()
	if err != nil {
		return &control_service.EmptyResponse{}, fmt.Errorf("Failed to setup service: %v", err)
	}
	h.stateMachine.Event(context.Background(), EventReady)
	return &control_service.EmptyResponse{}, nil
}

func (h *Server) AddEntity(_ context.Context, req *control_service.AddEntityRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received add entity command")

	if h.stateMachine.Is(StateSetup) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not in ready state")
	}

	entityAddress := type_conversions.ConvertRPCEntityAddress(req.GetAddress())
	res := h.eebusService.LocalDevice().Entity(entityAddress)
	if res != nil {
		return &control_service.EmptyResponse{}, fmt.Errorf("Entity address already used")
	}

	entity := spine.NewEntityLocal(
		h.eebusService.LocalDevice(),
		model.EntityTypeType(req.GetType().String()),
		entityAddress,
		h.eebusService.Configuration().HeartbeatTimeout(),
	)
	h.eebusService.LocalDevice().AddEntity(entity)

	return &control_service.EmptyResponse{}, nil
}

func (h *Server) RemoveEntity(_ context.Context, req *control_service.RemoveEntityRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received remove entity command")
	if h.stateMachine.Is(StateSetup) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service is not in ready state")
	}

	entityAddress := type_conversions.ConvertRPCEntityAddress(req.GetAddress())
	entity := h.eebusService.LocalDevice().Entity(entityAddress)
	if entity == nil {
		return &control_service.EmptyResponse{}, fmt.Errorf("Entity not found")
	}
	h.eebusService.LocalDevice().RemoveEntity(entity)

	return &control_service.EmptyResponse{}, nil
}

func (h *Server) RegisterRemoteSki(_ context.Context, req *control_service.RegisterRemoteSkiRequest) (*control_service.EmptyResponse, error) {
	h.Debug("Received register remote SKI command")

	if !h.stateMachine.Is(StateReady) && !h.stateMachine.Is(StateRunning) {
		return &control_service.EmptyResponse{}, fmt.Errorf("Service must be in ready or running state")
	}

	h.eebusService.RegisterRemoteSKI(req.GetRemoteSki())

	return &control_service.EmptyResponse{}, nil
}

func (h *Server) AddUseCase(_ context.Context, req *control_service.AddUseCaseRequest) (*control_service.AddUseCaseResponse, error) {
	h.Debug("Received add use case command")

	entityAddress := type_conversions.ConvertRPCEntityAddress(req.GetEntityAddress())
	h.Debugf("h.eebusService: %v", h.eebusService)
	h.Debugf("h.eebusService.LocalDevice(): %v", h.eebusService.LocalDevice())
	h.Debugf("entityAddress: %v", entityAddress)
	entity := h.eebusService.LocalDevice().Entity(entityAddress)
	h.Debugf("entity: %v", entity)

	actor := model.UseCaseActorType(req.GetUseCase().GetActor().String())
	useCaseName := model.UseCaseNameType(req.GetUseCase().GetName().String())

	filter := model.UseCaseFilterType{
		Actor:       actor,
		UseCaseName: useCaseName,
	}

	if entity.HasUseCaseSupport(filter) {
		return &control_service.AddUseCaseResponse{}, fmt.Errorf("Use case already supported")
	}

	var useCaseEntry useCaseEntryType
	var useCaseServer usecase_server.UseCaseServer

	switch actor {
	case model.UseCaseActorTypeCEM:
		switch useCaseName {
		case model.UseCaseNameTypeCoordinatedEVCharging:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeEVCommissioningAndConfiguration:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeMeasurementOfElectricityDuringEVCharging:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeEVSECommissioningAndConfiguration:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeEVStateOfCharge:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeOverloadProtectionByEVChargingCurrentCurtailment:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeOptimizationOfSelfConsumptionDuringEVCharging:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeVisualizationOfAggregatedBatteryData:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeVisualizationOfAggregatedPhotovoltaicData:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		default:
			return &control_service.AddUseCaseResponse{}, fmt.Errorf("Unknown use case '%v' for role CEM", req.GetUseCase().GetName().String())
		}
	case model.UseCaseActorTypeControllableSystem:
		switch useCaseName {
		case model.UseCaseNameTypeLimitationOfPowerConsumption:
			useCaseServer = cs_lpc_server.NewServer(
				&h.eebusService.Service,
				entity,
			)
			useCaseEntry = useCaseEntryType{
				entityAddress: entityAddress,
				useCase:       req.GetUseCase(),
				useCaseServer: useCaseServer,
			}

		case model.UseCaseNameTypeLimitationOfPowerProduction:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		default:
			return &control_service.AddUseCaseResponse{}, fmt.Errorf("Unknown use case '%v' for role CS", req.GetUseCase().GetName().String())
		}
	case model.UseCaseActorTypeEnergyGuard:
		switch useCaseName {
		case model.UseCaseNameTypeLimitationOfPowerConsumption:
			useCaseServer = eg_lpc_server.NewServer(
				&h.eebusService.Service,
				entity,
			)
			useCaseEntry = useCaseEntryType{
				entityAddress: entityAddress,
				useCase:       req.GetUseCase(),
				useCaseServer: useCaseServer,
			}
		case model.UseCaseNameTypeLimitationOfPowerProduction:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		default:
			return &control_service.AddUseCaseResponse{}, fmt.Errorf("Unknown use case '%v' for role EG", req.GetUseCase().GetName().String())
		}
	case model.UseCaseActorTypeMonitoringAppliance:
		switch useCaseName {
		case model.UseCaseNameTypeMonitoringOfGridConnectionPoint:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		case model.UseCaseNameTypeMonitoringOfPowerConsumption:
			return &control_service.AddUseCaseResponse{}, errors.New("Not implemented")
		default:
			return &control_service.AddUseCaseResponse{}, fmt.Errorf("Unknown use case '%v' for role MA", req.GetUseCase().GetName().String())
		}
	default:
		return &control_service.AddUseCaseResponse{}, fmt.Errorf("Unknown role '%v'", req.GetUseCase().GetActor().String())
	}
	h.useCaseRegistry = append(h.useCaseRegistry, useCaseEntry)

	port, err := useCaseServer.Start(nil)
	if err != nil {
		return &control_service.AddUseCaseResponse{}, fmt.Errorf("Failed to start use case server: %v", err)
	}

	return &control_service.AddUseCaseResponse{
		Endpoint: fmt.Sprintf("localhost:%v", port),
	}, nil
}

func (h *Server) SubscribeUseCaseEvents(req *control_service.SubscribeUseCaseEventsRequest, stream control_service.ControlService_SubscribeUseCaseEventsServer) error {
	entityAddress := type_conversions.ConvertRPCEntityAddress(req.GetEntityAddress())

	var useCaseServer usecase_server.UseCaseServer = nil
	referenceEntry := useCaseEntryType{
		entityAddress: entityAddress,
		useCase:       req.GetUseCase(),
		useCaseServer: nil,
	}
	for _, entry := range h.useCaseRegistry {
		if entry.Equals(&referenceEntry) {
			useCaseServer = entry.useCaseServer
			break
		}
	}
	if useCaseServer == nil {
		return fmt.Errorf("Use case not found")
	}
	for event := range useCaseServer.GetEvents() {
		err := stream.Send(event)
		if err != nil {
			return fmt.Errorf("Failed to send event: %v", err)
		}
	}
	return nil
}

func (h *Server) SubscribeDiscoveryEvents(
	_ *control_service.SubscribeDiscoveryEventsRequest,
	stream control_service.ControlService_SubscribeDiscoveryEventsServer,
) error {
	if !h.stateMachine.Is(StateReady) && !h.stateMachine.Is(StateRunning) {
		return fmt.Errorf("Service must be in ready or running state")
	}

	snapshot, ch, cancel := h.eebusService.SubscribeDiscoveries()
	defer cancel()

	for _, evt := range snapshot {
		if err := stream.Send(evt); err != nil {
			return err
		}
	}

	for {
		select {
		case <-stream.Context().Done():
			return stream.Context().Err()
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			if err := stream.Send(evt); err != nil {
				return err
			}
		}
	}
}

func (h *Server) Start(port *int) (int, error) {
	if port == nil {
		return -1, fmt.Errorf("no port specified")
	}
	if h.grpcServer != nil {
		return -1, fmt.Errorf("grpc server is already running")
	}
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		return -1, fmt.Errorf("failed to listen: %v", err)
	}
	h.grpcServer = grpc.NewServer()
	control_service.RegisterControlServiceServer(h.grpcServer, h)
	err = h.grpcServer.Serve(lis)
	if err != nil {
		return -1, fmt.Errorf("failed to serve: %v", err)
	}

	return *port, nil
}

func (h *Server) Stop() error {
	if h.grpcServer == nil {
		return fmt.Errorf("grpc server is not running")
	}
	h.grpcServer.Stop()
	h.grpcServer = nil

	return nil
}
