package api

import (
	"time"

	"github.com/satori/go.uuid"

	"github.com/jmoiron/sqlx"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/loraserver/internal/downlink/data/classb"
	proprietarydown "github.com/brocaar/loraserver/internal/downlink/proprietary"
	"github.com/brocaar/loraserver/internal/framelog"
	"github.com/brocaar/loraserver/internal/gps"
	"github.com/brocaar/loraserver/internal/storage"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
	"github.com/brocaar/lorawan/band"
)

var rfRegionMapping = map[band.Name]backend.RFRegion{
	band.AS_923:     backend.AS923,
	band.AU_915_928: backend.Australia915,
	band.CN_470_510: backend.China470,
	band.CN_779_787: backend.China779,
	band.EU_433:     backend.EU433,
	band.EU_863_870: backend.EU868,
	band.IN_865_867: backend.RFRegion("India865"),      // ? is not defined
	band.KR_920_923: backend.RFRegion("SouthKorea920"), // ? is not defined
	band.US_902_928: backend.US902,
}

// defaultCodeRate defines the default code rate
const defaultCodeRate = "4/5"

// classBScheduleMargin contains a Class-B scheduling margin to make sure
// there is enough time between scheduling and the actual Class-B ping-slot.
const classBScheduleMargin = 5 * time.Second

// NetworkServerAPI defines the nework-server API.
type NetworkServerAPI struct{}

// NewNetworkServerAPI returns a new NetworkServerAPI.
func NewNetworkServerAPI() *NetworkServerAPI {
	return &NetworkServerAPI{}
}

// CreateServiceProfile creates the given service-profile.
func (n *NetworkServerAPI) CreateServiceProfile(ctx context.Context, req *ns.CreateServiceProfileRequest) (*ns.CreateServiceProfileResponse, error) {
	if req.ServiceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "service_profile must not be nil")
	}

	var spID uuid.UUID
	copy(spID[:], req.ServiceProfile.Id)

	sp := storage.ServiceProfile{
		ID:                     spID,
		ULRate:                 int(req.ServiceProfile.UlRate),
		ULBucketSize:           int(req.ServiceProfile.UlBucketSize),
		DLRate:                 int(req.ServiceProfile.DlRate),
		DLBucketSize:           int(req.ServiceProfile.DlBucketSize),
		AddGWMetadata:          req.ServiceProfile.AddGwMetadata,
		DevStatusReqFreq:       int(req.ServiceProfile.DevStatusReqFreq),
		ReportDevStatusBattery: req.ServiceProfile.ReportDevStatusBattery,
		ReportDevStatusMargin:  req.ServiceProfile.ReportDevStatusMargin,
		DRMin:          int(req.ServiceProfile.DrMin),
		DRMax:          int(req.ServiceProfile.DrMax),
		ChannelMask:    req.ServiceProfile.ChannelMask,
		PRAllowed:      req.ServiceProfile.PrAllowed,
		HRAllowed:      req.ServiceProfile.HrAllowed,
		RAAllowed:      req.ServiceProfile.RaAllowed,
		NwkGeoLoc:      req.ServiceProfile.NwkGeoLoc,
		TargetPER:      int(req.ServiceProfile.TargetPer),
		MinGWDiversity: int(req.ServiceProfile.MinGwDiversity),
	}

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ULRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.ULRatePolicy = storage.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.DLRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.DLRatePolicy = storage.Drop
	}

	if err := storage.CreateServiceProfile(config.C.PostgreSQL.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateServiceProfileResponse{
		Id: sp.ID.Bytes(),
	}, nil
}

// GetServiceProfile returns the service-profile matching the given id.
func (n *NetworkServerAPI) GetServiceProfile(ctx context.Context, req *ns.GetServiceProfileRequest) (*ns.GetServiceProfileResponse, error) {
	var spID uuid.UUID
	copy(spID[:], req.Id)

	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, spID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	resp := ns.GetServiceProfileResponse{
		CreatedAtUnixNs: sp.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: sp.UpdatedAt.UnixNano(),
		ServiceProfile: &ns.ServiceProfile{
			Id:                     sp.ID.Bytes(),
			UlRate:                 uint32(sp.ULRate),
			UlBucketSize:           uint32(sp.ULBucketSize),
			DlRate:                 uint32(sp.DLRate),
			DlBucketSize:           uint32(sp.DLBucketSize),
			AddGwMetadata:          sp.AddGWMetadata,
			DevStatusReqFreq:       uint32(sp.DevStatusReqFreq),
			ReportDevStatusBattery: sp.ReportDevStatusBattery,
			ReportDevStatusMargin:  sp.ReportDevStatusMargin,
			DrMin:          uint32(sp.DRMin),
			DrMax:          uint32(sp.DRMax),
			ChannelMask:    sp.ChannelMask,
			PrAllowed:      sp.PRAllowed,
			HrAllowed:      sp.HRAllowed,
			RaAllowed:      sp.RAAllowed,
			NwkGeoLoc:      sp.NwkGeoLoc,
			TargetPer:      uint32(sp.TargetPER),
			MinGwDiversity: uint32(sp.MinGWDiversity),
		},
	}

	switch sp.ULRatePolicy {
	case storage.Mark:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_MARK
	case storage.Drop:
		resp.ServiceProfile.UlRatePolicy = ns.RatePolicy_DROP
	}

	switch sp.DLRatePolicy {
	case storage.Mark:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_MARK
	case storage.Drop:
		resp.ServiceProfile.DlRatePolicy = ns.RatePolicy_DROP
	}

	return &resp, nil
}

// UpdateServiceProfile updates the given service-profile.
func (n *NetworkServerAPI) UpdateServiceProfile(ctx context.Context, req *ns.UpdateServiceProfileRequest) (*ns.UpdateServiceProfileResponse, error) {
	if req.ServiceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "service_profile must not be nil")
	}

	var spID uuid.UUID
	copy(spID[:], req.ServiceProfile.Id)

	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, spID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp.ULRate = int(req.ServiceProfile.UlRate)
	sp.ULBucketSize = int(req.ServiceProfile.UlBucketSize)
	sp.DLRate = int(req.ServiceProfile.DlRate)
	sp.DLBucketSize = int(req.ServiceProfile.DlBucketSize)
	sp.AddGWMetadata = req.ServiceProfile.AddGwMetadata
	sp.DevStatusReqFreq = int(req.ServiceProfile.DevStatusReqFreq)
	sp.ReportDevStatusBattery = req.ServiceProfile.ReportDevStatusBattery
	sp.ReportDevStatusMargin = req.ServiceProfile.ReportDevStatusMargin
	sp.DRMin = int(req.ServiceProfile.DrMin)
	sp.DRMax = int(req.ServiceProfile.DrMax)
	sp.ChannelMask = backend.HEXBytes(req.ServiceProfile.ChannelMask)
	sp.PRAllowed = req.ServiceProfile.PrAllowed
	sp.HRAllowed = req.ServiceProfile.HrAllowed
	sp.RAAllowed = req.ServiceProfile.RaAllowed
	sp.NwkGeoLoc = req.ServiceProfile.NwkGeoLoc
	sp.TargetPER = int(req.ServiceProfile.TargetPer)
	sp.MinGWDiversity = int(req.ServiceProfile.MinGwDiversity)

	switch req.ServiceProfile.UlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.ULRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.ULRatePolicy = storage.Drop
	}

	switch req.ServiceProfile.DlRatePolicy {
	case ns.RatePolicy_MARK:
		sp.DLRatePolicy = storage.Mark
	case ns.RatePolicy_DROP:
		sp.DLRatePolicy = storage.Drop
	}

	if err := storage.FlushServiceProfileCache(config.C.Redis.Pool, sp.ID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateServiceProfile(config.C.PostgreSQL.DB, &sp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateServiceProfileResponse{}, nil
}

// DeleteServiceProfile deletes the service-profile matching the given id.
func (n *NetworkServerAPI) DeleteServiceProfile(ctx context.Context, req *ns.DeleteServiceProfileRequest) (*ns.DeleteServiceProfileResponse, error) {
	var spID uuid.UUID
	copy(spID[:], req.Id)

	if err := storage.FlushServiceProfileCache(config.C.Redis.Pool, spID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteServiceProfile(config.C.PostgreSQL.DB, spID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteServiceProfileResponse{}, nil
}

// CreateRoutingProfile creates the given routing-profile.
func (n *NetworkServerAPI) CreateRoutingProfile(ctx context.Context, req *ns.CreateRoutingProfileRequest) (*ns.CreateRoutingProfileResponse, error) {
	if req.RoutingProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "routing_profile must not be nil")
	}

	var rpID uuid.UUID
	copy(rpID[:], req.RoutingProfile.Id)

	rp := storage.RoutingProfile{
		ID:      rpID,
		ASID:    req.RoutingProfile.AsId,
		CACert:  req.RoutingProfile.CaCert,
		TLSCert: req.RoutingProfile.TlsCert,
		TLSKey:  req.RoutingProfile.TlsKey,
	}
	if err := storage.CreateRoutingProfile(config.C.PostgreSQL.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateRoutingProfileResponse{
		Id: rp.ID.Bytes(),
	}, nil
}

// GetRoutingProfile returns the routing-profile matching the given id.
func (n *NetworkServerAPI) GetRoutingProfile(ctx context.Context, req *ns.GetRoutingProfileRequest) (*ns.GetRoutingProfileResponse, error) {
	var rpID uuid.UUID
	copy(rpID[:], req.Id)

	rp, err := storage.GetRoutingProfile(config.C.PostgreSQL.DB, rpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRoutingProfileResponse{
		CreatedAtUnixNs: rp.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: rp.UpdatedAt.UnixNano(),
		RoutingProfile: &ns.RoutingProfile{
			Id:      rp.ID.Bytes(),
			AsId:    rp.ASID,
			CaCert:  rp.CACert,
			TlsCert: rp.TLSCert,
		},
	}, nil
}

// UpdateRoutingProfile updates the given routing-profile.
func (n *NetworkServerAPI) UpdateRoutingProfile(ctx context.Context, req *ns.UpdateRoutingProfileRequest) (*ns.UpdateRoutingProfileResponse, error) {
	if req.RoutingProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "routing_profile must not be nil")
	}

	var rpID uuid.UUID
	copy(rpID[:], req.RoutingProfile.Id)

	rp, err := storage.GetRoutingProfile(config.C.PostgreSQL.DB, rpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	rp.ASID = req.RoutingProfile.AsId
	rp.CACert = req.RoutingProfile.CaCert
	rp.TLSCert = req.RoutingProfile.TlsCert

	if req.RoutingProfile.TlsKey != "" {
		rp.TLSKey = req.RoutingProfile.TlsKey
	}

	if rp.TLSCert == "" {
		rp.TLSKey = ""
	}

	if err := storage.UpdateRoutingProfile(config.C.PostgreSQL.DB, &rp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateRoutingProfileResponse{}, nil
}

// DeleteRoutingProfile deletes the routing-profile matching the given id.
func (n *NetworkServerAPI) DeleteRoutingProfile(ctx context.Context, req *ns.DeleteRoutingProfileRequest) (*ns.DeleteRoutingProfileResponse, error) {
	var rpID uuid.UUID
	copy(rpID[:], req.Id)

	if err := storage.DeleteRoutingProfile(config.C.PostgreSQL.DB, rpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteRoutingProfileResponse{}, nil
}

// CreateDeviceProfile creates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) CreateDeviceProfile(ctx context.Context, req *ns.CreateDeviceProfileRequest) (*ns.CreateDeviceProfileResponse, error) {
	if req.DeviceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_profile must not be nil")
	}

	var dpID uuid.UUID
	copy(dpID[:], req.DeviceProfile.Id)

	var factoryPresetFreqs []int
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, int(f))
	}

	dp := storage.DeviceProfile{
		ID:                 dpID,
		SupportsClassB:     req.DeviceProfile.SupportsClassB,
		ClassBTimeout:      int(req.DeviceProfile.ClassBTimeout),
		PingSlotPeriod:     int(req.DeviceProfile.PingSlotPeriod),
		PingSlotDR:         int(req.DeviceProfile.PingSlotDr),
		PingSlotFreq:       int(req.DeviceProfile.PingSlotFreq),
		SupportsClassC:     req.DeviceProfile.SupportsClassC,
		ClassCTimeout:      int(req.DeviceProfile.ClassCTimeout),
		MACVersion:         req.DeviceProfile.MacVersion,
		RegParamsRevision:  req.DeviceProfile.RegParamsRevision,
		RXDelay1:           int(req.DeviceProfile.RxDelay_1),
		RXDROffset1:        int(req.DeviceProfile.RxDrOffset_1),
		RXDataRate2:        int(req.DeviceProfile.RxDatarate_2),
		RXFreq2:            int(req.DeviceProfile.RxFreq_2),
		FactoryPresetFreqs: factoryPresetFreqs,
		MaxEIRP:            int(req.DeviceProfile.MaxEirp),
		MaxDutyCycle:       int(req.DeviceProfile.MaxDutyCycle),
		SupportsJoin:       req.DeviceProfile.SupportsJoin,
		Supports32bitFCnt:  req.DeviceProfile.Supports_32BitFCnt,
	}

	rfRegion, ok := rfRegionMapping[config.C.NetworkServer.Band.Name]
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.RFRegion = string(config.C.NetworkServer.Band.Name)
	}
	dp.RFRegion = string(rfRegion)

	if err := storage.CreateDeviceProfile(config.C.PostgreSQL.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceProfileResponse{
		Id: dp.ID.Bytes(),
	}, nil
}

// GetDeviceProfile returns the device-profile matching the given id.
func (n *NetworkServerAPI) GetDeviceProfile(ctx context.Context, req *ns.GetDeviceProfileRequest) (*ns.GetDeviceProfileResponse, error) {
	var dpID uuid.UUID
	copy(dpID[:], req.Id)

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, dpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []uint32
	for _, f := range dp.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, uint32(f))
	}

	resp := ns.GetDeviceProfileResponse{
		CreatedAtUnixNs: dp.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: dp.UpdatedAt.UnixNano(),
		DeviceProfile: &ns.DeviceProfile{
			Id:                 dp.ID.Bytes(),
			SupportsClassB:     dp.SupportsClassB,
			ClassBTimeout:      uint32(dp.ClassBTimeout),
			PingSlotPeriod:     uint32(dp.PingSlotPeriod),
			PingSlotDr:         uint32(dp.PingSlotDR),
			PingSlotFreq:       uint32(dp.PingSlotFreq),
			SupportsClassC:     dp.SupportsClassC,
			ClassCTimeout:      uint32(dp.ClassCTimeout),
			MacVersion:         dp.MACVersion,
			RegParamsRevision:  dp.RegParamsRevision,
			RxDelay_1:          uint32(dp.RXDelay1),
			RxDrOffset_1:       uint32(dp.RXDROffset1),
			RxDatarate_2:       uint32(dp.RXDataRate2),
			RxFreq_2:           uint32(dp.RXFreq2),
			FactoryPresetFreqs: factoryPresetFreqs,
			MaxEirp:            uint32(dp.MaxEIRP),
			MaxDutyCycle:       uint32(dp.MaxDutyCycle),
			SupportsJoin:       dp.SupportsJoin,
			RfRegion:           string(dp.RFRegion),
			Supports_32BitFCnt: dp.Supports32bitFCnt,
		},
	}

	return &resp, nil
}

// UpdateDeviceProfile updates the given device-profile.
// The RFRegion field will get set automatically according to the configured band.
func (n *NetworkServerAPI) UpdateDeviceProfile(ctx context.Context, req *ns.UpdateDeviceProfileRequest) (*ns.UpdateDeviceProfileResponse, error) {
	if req.DeviceProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_profile must not be nil")
	}

	var dpID uuid.UUID
	copy(dpID[:], req.DeviceProfile.Id)

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, dpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var factoryPresetFreqs []int
	for _, f := range req.DeviceProfile.FactoryPresetFreqs {
		factoryPresetFreqs = append(factoryPresetFreqs, int(f))
	}

	dp.SupportsClassB = req.DeviceProfile.SupportsClassB
	dp.ClassBTimeout = int(req.DeviceProfile.ClassBTimeout)
	dp.PingSlotPeriod = int(req.DeviceProfile.PingSlotPeriod)
	dp.PingSlotDR = int(req.DeviceProfile.PingSlotDr)
	dp.PingSlotFreq = int(req.DeviceProfile.PingSlotFreq)
	dp.SupportsClassC = req.DeviceProfile.SupportsClassC
	dp.ClassCTimeout = int(req.DeviceProfile.ClassCTimeout)
	dp.MACVersion = req.DeviceProfile.MacVersion
	dp.RegParamsRevision = req.DeviceProfile.RegParamsRevision
	dp.RXDelay1 = int(req.DeviceProfile.RxDelay_1)
	dp.RXDROffset1 = int(req.DeviceProfile.RxDrOffset_1)
	dp.RXDataRate2 = int(req.DeviceProfile.RxDatarate_2)
	dp.RXFreq2 = int(req.DeviceProfile.RxFreq_2)
	dp.FactoryPresetFreqs = factoryPresetFreqs
	dp.MaxEIRP = int(req.DeviceProfile.MaxEirp)
	dp.MaxDutyCycle = int(req.DeviceProfile.MaxDutyCycle)
	dp.SupportsJoin = req.DeviceProfile.SupportsJoin
	dp.Supports32bitFCnt = req.DeviceProfile.Supports_32BitFCnt

	rfRegion, ok := rfRegionMapping[config.C.NetworkServer.Band.Name]
	dp.RFRegion = string(rfRegion)
	if !ok {
		// band name has not been specified by the LoRaWAN backend interfaces
		// specification. use the internal BandName for now so that when these
		// values are specified in a next version, this can be fixed in a db
		// migration
		dp.RFRegion = string(config.C.NetworkServer.Band.Name)
	}

	if err := storage.FlushDeviceProfileCache(config.C.Redis.Pool, dp.ID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.UpdateDeviceProfile(config.C.PostgreSQL.DB, &dp); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceProfileResponse{}, nil
}

// DeleteDeviceProfile deletes the device-profile matching the given id.
func (n *NetworkServerAPI) DeleteDeviceProfile(ctx context.Context, req *ns.DeleteDeviceProfileRequest) (*ns.DeleteDeviceProfileResponse, error) {
	var dpID uuid.UUID
	copy(dpID[:], req.Id)

	if err := storage.FlushDeviceProfileCache(config.C.Redis.Pool, dpID); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.DeleteDeviceProfile(config.C.PostgreSQL.DB, dpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteDeviceProfileResponse{}, nil
}

// CreateDevice creates the given device.
func (n *NetworkServerAPI) CreateDevice(ctx context.Context, req *ns.CreateDeviceRequest) (*ns.CreateDeviceResponse, error) {
	if req.Device == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device must not be nil")
	}

	var devEUI lorawan.EUI64
	var dpID, spID, rpID uuid.UUID

	copy(devEUI[:], req.Device.DevEui)
	copy(dpID[:], req.Device.DeviceProfileId)
	copy(rpID[:], req.Device.RoutingProfileId)
	copy(spID[:], req.Device.ServiceProfileId)

	d := storage.Device{
		DevEUI:           devEUI,
		DeviceProfileID:  dpID,
		ServiceProfileID: spID,
		RoutingProfileID: rpID,
		SkipFCntCheck:    req.Device.SkipFCntCheck,
	}
	if err := storage.CreateDevice(config.C.PostgreSQL.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceResponse{}, nil
}

// GetDevice returns the device matching the given DevEUI.
func (n *NetworkServerAPI) GetDevice(ctx context.Context, req *ns.GetDeviceRequest) (*ns.GetDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetDeviceResponse{
		CreatedAtUnixNs: d.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: d.UpdatedAt.UnixNano(),
		Device: &ns.Device{
			DevEui:           d.DevEUI[:],
			SkipFCntCheck:    d.SkipFCntCheck,
			DeviceProfileId:  d.DeviceProfileID[:],
			ServiceProfileId: d.ServiceProfileID[:],
			RoutingProfileId: d.RoutingProfileID[:],
		},
	}, nil
}

// UpdateDevice updates the given device.
func (n *NetworkServerAPI) UpdateDevice(ctx context.Context, req *ns.UpdateDeviceRequest) (*ns.UpdateDeviceResponse, error) {
	if req.Device == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device must not be nil")
	}

	var devEUI lorawan.EUI64
	var dpID, spID, rpID uuid.UUID

	copy(devEUI[:], req.Device.DevEui)
	copy(dpID[:], req.Device.DeviceProfileId)
	copy(rpID[:], req.Device.RoutingProfileId)
	copy(spID[:], req.Device.ServiceProfileId)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	d.DeviceProfileID = dpID
	d.ServiceProfileID = spID
	d.RoutingProfileID = rpID
	d.SkipFCntCheck = req.Device.SkipFCntCheck

	if err := storage.UpdateDevice(config.C.PostgreSQL.DB, &d); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateDeviceResponse{}, nil
}

// DeleteDevice deletes the device matching the given DevEUI.
func (n *NetworkServerAPI) DeleteDevice(ctx context.Context, req *ns.DeleteDeviceRequest) (*ns.DeleteDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		if err := storage.DeleteDevice(tx, devEUI); err != nil {
			return errToRPCError(err)
		}

		if err := storage.DeleteDeviceSession(config.C.Redis.Pool, devEUI); err != nil && err != storage.ErrDoesNotExist {
			return errToRPCError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ns.DeleteDeviceResponse{}, nil
}

// ActivateDevice activates a device (ABP).
func (n *NetworkServerAPI) ActivateDevice(ctx context.Context, req *ns.ActivateDeviceRequest) (*ns.ActivateDeviceResponse, error) {
	if req.DeviceActivation == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "device_activation must not be nil")
	}

	var devEUI lorawan.EUI64
	var devAddr lorawan.DevAddr
	var sNwkSIntKey, fNwkSIntKey, nwkSEncKey lorawan.AES128Key

	copy(devEUI[:], req.DeviceActivation.DevEui)
	copy(devAddr[:], req.DeviceActivation.DevAddr)
	copy(sNwkSIntKey[:], req.DeviceActivation.SNwkSIntKey)
	copy(fNwkSIntKey[:], req.DeviceActivation.FNwkSIntKey)
	copy(nwkSEncKey[:], req.DeviceActivation.NwkSEncKey)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	sp, err := storage.GetServiceProfile(config.C.PostgreSQL.DB, d.ServiceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	ds := storage.DeviceSession{
		DeviceProfileID:  d.DeviceProfileID,
		ServiceProfileID: d.ServiceProfileID,
		RoutingProfileID: d.RoutingProfileID,

		DevEUI:             devEUI,
		DevAddr:            devAddr,
		SNwkSIntKey:        sNwkSIntKey,
		FNwkSIntKey:        fNwkSIntKey,
		NwkSEncKey:         nwkSEncKey,
		FCntUp:             req.DeviceActivation.FCntUp,
		NFCntDown:          req.DeviceActivation.NFCntDown,
		AFCntDown:          req.DeviceActivation.AFCntDown,
		SkipFCntValidation: req.DeviceActivation.SkipFCntCheck || d.SkipFCntCheck,

		RXWindow:       storage.RX1,
		MaxSupportedDR: sp.DRMax,

		MACVersion: dp.MACVersion,
	}

	// reset the device-session to the device boot parameters
	ds.ResetToBootParameters(dp)

	if err := storage.SaveDeviceSession(config.C.Redis.Pool, ds); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, d.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushMACCommandQueue(config.C.Redis.Pool, ds.DevEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.ActivateDeviceResponse{}, nil
}

// DeactivateDevice de-activates a device.
func (n *NetworkServerAPI) DeactivateDevice(ctx context.Context, req *ns.DeactivateDeviceRequest) (*ns.DeactivateDeviceResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	if err := storage.DeleteDeviceSession(config.C.Redis.Pool, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	if err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, devEUI); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeactivateDeviceResponse{}, nil
}

// GetDeviceActivation returns the device activation details.
func (n *NetworkServerAPI) GetDeviceActivation(ctx context.Context, req *ns.GetDeviceActivationRequest) (*ns.GetDeviceActivationResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetDeviceActivationResponse{
		DeviceActivation: &ns.DeviceActivation{
			DevEui:        ds.DevEUI[:],
			DevAddr:       ds.DevAddr[:],
			SNwkSIntKey:   ds.SNwkSIntKey[:],
			FNwkSIntKey:   ds.FNwkSIntKey[:],
			NwkSEncKey:    ds.NwkSEncKey[:],
			FCntUp:        ds.FCntUp,
			NFCntDown:     ds.NFCntDown,
			AFCntDown:     ds.AFCntDown,
			SkipFCntCheck: ds.SkipFCntValidation,
		},
	}, nil
}

// GetRandomDevAddr returns a random DevAddr.
func (n *NetworkServerAPI) GetRandomDevAddr(ctx context.Context, req *ns.GetRandomDevAddrRequest) (*ns.GetRandomDevAddrResponse, error) {
	devAddr, err := storage.GetRandomDevAddr(config.C.Redis.Pool, config.C.NetworkServer.NetID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.GetRandomDevAddrResponse{
		DevAddr: devAddr[:],
	}, nil
}

// CreateMACCommandQueueItem adds a data down MAC command to the queue.
// It replaces already enqueued mac-commands with the same CID.
func (n *NetworkServerAPI) CreateMACCommandQueueItem(ctx context.Context, req *ns.CreateMACCommandQueueItemRequest) (*ns.CreateMACCommandQueueItemResponse, error) {
	var commands []lorawan.MACCommand
	var devEUI lorawan.EUI64

	copy(devEUI[:], req.DevEui)

	for _, b := range req.Commands {
		var mac lorawan.MACCommand
		if err := mac.UnmarshalBinary(false, b); err != nil {
			return nil, grpc.Errorf(codes.InvalidArgument, err.Error())
		}
		commands = append(commands, mac)
	}

	block := storage.MACCommandBlock{
		CID:         lorawan.CID(req.Cid),
		External:    true,
		MACCommands: commands,
	}

	if err := storage.CreateMACCommandQueueItem(config.C.Redis.Pool, devEUI, block); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateMACCommandQueueItemResponse{}, nil
}

// SendProprietaryPayload send a payload using the 'Proprietary' LoRaWAN message-type.
func (n *NetworkServerAPI) SendProprietaryPayload(ctx context.Context, req *ns.SendProprietaryPayloadRequest) (*ns.SendProprietaryPayloadResponse, error) {
	var mic lorawan.MIC
	var gwMACs []lorawan.EUI64

	copy(mic[:], req.Mic)
	for i := range req.GatewayMacs {
		var mac lorawan.EUI64
		copy(mac[:], req.GatewayMacs[i])
		gwMACs = append(gwMACs, mac)
	}

	err := proprietarydown.Handle(req.MacPayload, mic, gwMACs, req.PolarizationInversion, int(req.Frequency), int(req.Dr))
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.SendProprietaryPayloadResponse{}, nil
}

// CreateGateway creates the given gateway.
func (n *NetworkServerAPI) CreateGateway(ctx context.Context, req *ns.CreateGatewayRequest) (*ns.CreateGatewayResponse, error) {
	if req.Gateway == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway must not be nil")
	}

	var mac lorawan.EUI64
	var gpID uuid.UUID
	copy(mac[:], req.Gateway.Id)
	copy(gpID[:], req.Gateway.GatewayProfileId)

	gw := storage.Gateway{
		MAC:         mac,
		Name:        req.Gateway.Name,
		Description: req.Gateway.Description,
		Location: storage.GPSPoint{
			Latitude:  req.Gateway.Latitude,
			Longitude: req.Gateway.Longitude,
		},
		Altitude: req.Gateway.Altitude,
	}
	if len(req.Gateway.GatewayProfileId) != 0 {
		gw.GatewayProfileID = &gpID
	}

	err := storage.CreateGateway(config.C.PostgreSQL.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayResponse{}, nil
}

// GetGateway returns data for a particular gateway.
func (n *NetworkServerAPI) GetGateway(ctx context.Context, req *ns.GetGatewayRequest) (*ns.GetGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Id)

	gw, err := storage.GetGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return gwToResp(gw), nil
}

// UpdateGateway updates an existing gateway.
func (n *NetworkServerAPI) UpdateGateway(ctx context.Context, req *ns.UpdateGatewayRequest) (*ns.UpdateGatewayResponse, error) {
	if req.Gateway == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway must not be nil")
	}

	var mac lorawan.EUI64
	var gpID uuid.UUID

	copy(mac[:], req.Gateway.Id)
	copy(gpID[:], req.Gateway.GatewayProfileId)

	gw, err := storage.GetGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if len(req.Gateway.GatewayProfileId) != 0 {
		gw.GatewayProfileID = &gpID
	} else {
		gw.GatewayProfileID = nil
	}

	gw.Name = req.Gateway.Name
	gw.Description = req.Gateway.Description
	gw.Location = storage.GPSPoint{
		Latitude:  req.Gateway.Latitude,
		Longitude: req.Gateway.Longitude,
	}
	gw.Altitude = req.Gateway.Altitude

	err = storage.UpdateGateway(config.C.PostgreSQL.DB, &gw)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayResponse{}, nil
}

// DeleteGateway deletes a gateway.
func (n *NetworkServerAPI) DeleteGateway(ctx context.Context, req *ns.DeleteGatewayRequest) (*ns.DeleteGatewayResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.Id)

	err := storage.DeleteGateway(config.C.PostgreSQL.DB, mac)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteGatewayResponse{}, nil
}

// GetGatewayStats returns stats of an existing gateway.
func (n *NetworkServerAPI) GetGatewayStats(ctx context.Context, req *ns.GetGatewayStatsRequest) (*ns.GetGatewayStatsResponse, error) {
	var mac lorawan.EUI64
	copy(mac[:], req.GatewayId)

	start := time.Unix(0, req.StartTimestampUnixNs)
	end := time.Unix(0, req.EndTimestampUnixNs)

	stats, err := storage.GetGatewayStats(config.C.PostgreSQL.DB, mac, req.Interval.String(), start, end)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var resp ns.GetGatewayStatsResponse

	for _, stat := range stats {
		resp.Result = append(resp.Result, &ns.GatewayStats{
			TimestampUnixNs:     stat.Timestamp.UnixNano(),
			RxPacketsReceived:   int32(stat.RXPacketsReceived),
			RxPacketsReceivedOk: int32(stat.RXPacketsReceivedOK),
			TxPacketsReceived:   int32(stat.TXPacketsReceived),
			TxPacketsEmitted:    int32(stat.TXPacketsEmitted),
		})
	}

	return &resp, nil
}

// StreamFrameLogsForGateway returns a stream of frames seen by the given gateway.
func (n *NetworkServerAPI) StreamFrameLogsForGateway(req *ns.StreamFrameLogsForGatewayRequest, srv ns.NetworkServerService_StreamFrameLogsForGatewayServer) error {
	frameLogChan := make(chan framelog.FrameLog)
	var mac lorawan.EUI64
	copy(mac[:], req.GatewayId)

	go func() {
		err := framelog.GetFrameLogForGateway(srv.Context(), mac, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for gateway error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		up, down, err := frameLogToUplinkAndDownlinkFrameLog(fl)
		if err != nil {
			log.WithError(err).Error("frame-log to uplink and downlink frame-log error")
			continue
		}

		var resp ns.StreamFrameLogsForGatewayResponse
		if up != nil {
			resp.UplinkFrames = append(resp.UplinkFrames, up)
		}

		if down != nil {
			resp.DownlinkFrames = append(resp.DownlinkFrames, down)
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// StreamFrameLogsForDevice returns a stream of frames seen by the given device.
func (n *NetworkServerAPI) StreamFrameLogsForDevice(req *ns.StreamFrameLogsForDeviceRequest, srv ns.NetworkServerService_StreamFrameLogsForDeviceServer) error {
	frameLogChan := make(chan framelog.FrameLog)
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	go func() {
		err := framelog.GetFrameLogForDevice(srv.Context(), devEUI, frameLogChan)
		if err != nil {
			log.WithError(err).Error("get frame-log for device error")
		}
		close(frameLogChan)
	}()

	for fl := range frameLogChan {
		up, down, err := frameLogToUplinkAndDownlinkFrameLog(fl)
		if err != nil {
			log.WithError(err).Error("frame-log to uplink and downlink frame-log error")
			continue
		}

		var resp ns.StreamFrameLogsForDeviceResponse
		if up != nil {
			resp.UplinkFrames = append(resp.UplinkFrames, up)
		}

		if down != nil {
			resp.DownlinkFrames = append(resp.DownlinkFrames, down)
		}

		if err := srv.Send(&resp); err != nil {
			log.WithError(err).Error("error sending frame-log response")
		}
	}

	return nil
}

// CreateGatewayProfile creates the given gateway-profile.
func (n *NetworkServerAPI) CreateGatewayProfile(ctx context.Context, req *ns.CreateGatewayProfileRequest) (*ns.CreateGatewayProfileResponse, error) {
	if req.GatewayProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway_profile must not be nil")
	}

	var gpID uuid.UUID
	copy(gpID[:], req.GatewayProfile.Id)

	gc := storage.GatewayProfile{
		ID: gpID,
	}

	for _, c := range req.GatewayProfile.Channels {
		gc.Channels = append(gc.Channels, int64(c))
	}

	for _, ec := range req.GatewayProfile.ExtraChannels {
		c := storage.ExtraChannel{
			Frequency: int(ec.Frequency),
			Bandwidth: int(ec.Bandwidth),
			Bitrate:   int(ec.Bitrate),
		}

		switch ec.Modulation {
		case ns.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err := storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		return storage.CreateGatewayProfile(tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateGatewayProfileResponse{Id: gc.ID.Bytes()}, nil
}

// GetGatewayProfile returns the gateway-profile given an id.
func (n *NetworkServerAPI) GetGatewayProfile(ctx context.Context, req *ns.GetGatewayProfileRequest) (*ns.GetGatewayProfileResponse, error) {
	var gpID uuid.UUID
	copy(gpID[:], req.Id)

	gc, err := storage.GetGatewayProfile(config.C.PostgreSQL.DB, gpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	out := ns.GetGatewayProfileResponse{
		CreatedAtUnixNs: gc.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: gc.UpdatedAt.UnixNano(),
		GatewayProfile: &ns.GatewayProfile{
			Id: gc.ID.Bytes(),
		},
	}

	for _, c := range gc.Channels {
		out.GatewayProfile.Channels = append(out.GatewayProfile.Channels, uint32(c))
	}

	for _, ec := range gc.ExtraChannels {
		c := ns.GatewayProfileExtraChannel{
			Frequency: uint32(ec.Frequency),
			Bandwidth: uint32(ec.Bandwidth),
			Bitrate:   uint32(ec.Bitrate),
		}

		switch ec.Modulation {
		case storage.ModulationFSK:
			c.Modulation = ns.Modulation_FSK
		default:
			c.Modulation = ns.Modulation_LORA
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, uint32(sf))
		}

		out.GatewayProfile.ExtraChannels = append(out.GatewayProfile.ExtraChannels, &c)
	}

	return &out, nil
}

// UpdateGatewayProfile updates the given gateway-profile.
func (n *NetworkServerAPI) UpdateGatewayProfile(ctx context.Context, req *ns.UpdateGatewayProfileRequest) (*ns.UpdateGatewayProfileResponse, error) {
	if req.GatewayProfile == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "gateway_profile must not be nil")
	}

	var gpID uuid.UUID
	copy(gpID[:], req.GatewayProfile.Id)

	gc, err := storage.GetGatewayProfile(config.C.PostgreSQL.DB, gpID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	gc.Channels = []int64{}
	for _, c := range req.GatewayProfile.Channels {
		gc.Channels = append(gc.Channels, int64(c))
	}

	gc.ExtraChannels = []storage.ExtraChannel{}
	for _, ec := range req.GatewayProfile.ExtraChannels {
		c := storage.ExtraChannel{
			Frequency: int(ec.Frequency),
			Bandwidth: int(ec.Bandwidth),
			Bitrate:   int(ec.Bitrate),
		}

		switch ec.Modulation {
		case ns.Modulation_FSK:
			c.Modulation = storage.ModulationFSK
		default:
			c.Modulation = storage.ModulationLoRa
		}

		for _, sf := range ec.SpreadingFactors {
			c.SpreadingFactors = append(c.SpreadingFactors, int64(sf))
		}

		gc.ExtraChannels = append(gc.ExtraChannels, c)
	}

	err = storage.Transaction(config.C.PostgreSQL.DB, func(tx sqlx.Ext) error {
		return storage.UpdateGatewayProfile(tx, &gc)
	})
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.UpdateGatewayProfileResponse{}, nil
}

// DeleteGatewayProfile deletes the gateway-profile matching a given id.
func (n *NetworkServerAPI) DeleteGatewayProfile(ctx context.Context, req *ns.DeleteGatewayProfileRequest) (*ns.DeleteGatewayProfileResponse, error) {
	var gpID uuid.UUID
	copy(gpID[:], req.Id)

	if err := storage.DeleteGatewayProfile(config.C.PostgreSQL.DB, gpID); err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.DeleteGatewayProfileResponse{}, nil
}

// CreateDeviceQueueItem creates the given device-queue item.
func (n *NetworkServerAPI) CreateDeviceQueueItem(ctx context.Context, req *ns.CreateDeviceQueueItemRequest) (*ns.CreateDeviceQueueItemResponse, error) {
	if req.Item == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "item must not be nil")
	}

	var devEUI lorawan.EUI64
	copy(devEUI[:], req.Item.DevEui)

	d, err := storage.GetDevice(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	dp, err := storage.GetDeviceProfile(config.C.PostgreSQL.DB, d.DeviceProfileID)
	if err != nil {
		return nil, errToRPCError(err)
	}

	qi := storage.DeviceQueueItem{
		DevEUI:     d.DevEUI,
		FRMPayload: req.Item.FrmPayload,
		FCnt:       req.Item.FCnt,
		FPort:      uint8(req.Item.FPort),
		Confirmed:  req.Item.Confirmed,
	}

	// When the device is operating in Class-B and has a beacon lock, calculate
	// the next ping-slot.
	if dp.SupportsClassB {
		// check if device is currently active and is operating in Class-B mode
		ds, err := storage.GetDeviceSession(config.C.Redis.Pool, d.DevEUI)
		if err != nil && err != storage.ErrDoesNotExist {
			return nil, errToRPCError(err)
		}

		if err == nil && ds.BeaconLocked {
			scheduleAfterGPSEpochTS, err := storage.GetMaxEmitAtTimeSinceGPSEpochForDevEUI(config.C.PostgreSQL.DB, d.DevEUI)
			if err != nil {
				return nil, errToRPCError(err)
			}

			if scheduleAfterGPSEpochTS == 0 {
				scheduleAfterGPSEpochTS = gps.Time(time.Now()).TimeSinceGPSEpoch()
			}

			// take some margin into account
			scheduleAfterGPSEpochTS += classBScheduleMargin

			gpsEpochTS, err := classb.GetNextPingSlotAfter(scheduleAfterGPSEpochTS, ds.DevAddr, ds.PingSlotNb)
			if err != nil {
				return nil, errToRPCError(err)
			}

			timeoutTime := time.Time(gps.NewFromTimeSinceGPSEpoch(gpsEpochTS)).Add(time.Second * time.Duration(dp.ClassBTimeout))
			qi.EmitAtTimeSinceGPSEpoch = &gpsEpochTS
			qi.TimeoutAfter = &timeoutTime
		}
	}

	err = storage.CreateDeviceQueueItem(config.C.PostgreSQL.DB, &qi)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.CreateDeviceQueueItemResponse{}, nil
}

// FlushDeviceQueueForDevEUI flushes the device-queue for the given DevEUI.
func (n *NetworkServerAPI) FlushDeviceQueueForDevEUI(ctx context.Context, req *ns.FlushDeviceQueueForDevEUIRequest) (*ns.FlushDeviceQueueForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	err := storage.FlushDeviceQueueForDevEUI(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	return &ns.FlushDeviceQueueForDevEUIResponse{}, nil
}

// GetDeviceQueueItemsForDevEUI returns all device-queue items for the given DevEUI.
func (n *NetworkServerAPI) GetDeviceQueueItemsForDevEUI(ctx context.Context, req *ns.GetDeviceQueueItemsForDevEUIRequest) (*ns.GetDeviceQueueItemsForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEui)

	items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	var out ns.GetDeviceQueueItemsForDevEUIResponse
	for i := range items {
		qi := ns.DeviceQueueItem{
			DevEui:     items[i].DevEUI[:],
			FrmPayload: items[i].FRMPayload,
			FCnt:       items[i].FCnt,
			FPort:      uint32(items[i].FPort),
			Confirmed:  items[i].Confirmed,
		}

		out.Items = append(out.Items, &qi)
	}

	return &out, nil
}

// GetNextDownlinkFCntForDevEUI returns the next FCnt that must be used.
// This also takes device-queue items for the given DevEUI into consideration.
// In case the device is not activated, this will return an error as no
// device-session exists.
func (n *NetworkServerAPI) GetNextDownlinkFCntForDevEUI(ctx context.Context, req *ns.GetNextDownlinkFCntForDevEUIRequest) (*ns.GetNextDownlinkFCntForDevEUIResponse, error) {
	var devEUI lorawan.EUI64
	var resp ns.GetNextDownlinkFCntForDevEUIResponse

	copy(devEUI[:], req.DevEui)

	ds, err := storage.GetDeviceSession(config.C.Redis.Pool, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}

	if ds.GetMACVersion() == lorawan.LoRaWAN1_0 {
		resp.FCnt = ds.NFCntDown
	} else {
		resp.FCnt = ds.AFCntDown
	}

	items, err := storage.GetDeviceQueueItemsForDevEUI(config.C.PostgreSQL.DB, devEUI)
	if err != nil {
		return nil, errToRPCError(err)
	}
	if count := len(items); count != 0 {
		resp.FCnt = items[count-1].FCnt + 1 // we want the next usable frame-counter
	}

	return &resp, nil
}

// GetVersion returns the LoRa Server version.
func (n *NetworkServerAPI) GetVersion(ctx context.Context, req *ns.GetVersionRequest) (*ns.GetVersionResponse, error) {
	region, ok := map[band.Name]ns.Region{
		band.AS_923:     ns.Region_AS923,
		band.AU_915_928: ns.Region_AU915,
		band.CN_470_510: ns.Region_CN470,
		band.CN_779_787: ns.Region_CN779,
		band.EU_433:     ns.Region_EU433,
		band.EU_863_870: ns.Region_EU868,
		band.IN_865_867: ns.Region_IN865,
		band.KR_920_923: ns.Region_KR920,
		band.RU_864_870: ns.Region_RU864,
		band.US_902_928: ns.Region_US915,
	}[config.C.NetworkServer.Band.Name]

	if !ok {
		log.WithFields(log.Fields{
			"band_name": config.C.NetworkServer.Band.Name,
		}).Warning("unknown band to common name mapping")
	}

	return &ns.GetVersionResponse{
		Region:  region,
		Version: config.Version,
	}, nil
}

func gwToResp(gw storage.Gateway) *ns.GetGatewayResponse {
	resp := ns.GetGatewayResponse{
		Gateway: &ns.Gateway{
			Id:          gw.MAC[:],
			Name:        gw.Name,
			Description: gw.Description,
			Latitude:    gw.Location.Latitude,
			Longitude:   gw.Location.Longitude,
			Altitude:    gw.Altitude,
		},
		CreatedAtUnixNs: gw.CreatedAt.UnixNano(),
		UpdatedAtUnixNs: gw.UpdatedAt.UnixNano(),
	}

	if gw.FirstSeenAt != nil {
		resp.FirstSeenAtUnixNs = gw.FirstSeenAt.UnixNano()
	}

	if gw.LastSeenAt != nil {
		resp.LastSeenAtUnixNs = gw.LastSeenAt.UnixNano()
	}

	if gw.GatewayProfileID != nil {
		resp.Gateway.GatewayProfileId = gw.GatewayProfileID.Bytes()
	}

	return &resp
}

func frameLogToUplinkAndDownlinkFrameLog(fl framelog.FrameLog) (*ns.UplinkFrameLog, *ns.DownlinkFrameLog, error) {
	var up *ns.UplinkFrameLog
	var down *ns.DownlinkFrameLog

	if fl.UplinkFrame != nil {
		b, err := fl.UplinkFrame.PHYPayload.MarshalBinary()
		if err != nil {
			return nil, nil, errors.Wrap(err, "marshal phypayload error")
		}

		up = &ns.UplinkFrameLog{
			TxInfo: &ns.UplinkTXInfo{
				Frequency: uint32(fl.UplinkFrame.TXInfo.Frequency),
				DataRate: &ns.DataRate{
					Modulation:      string(fl.UplinkFrame.TXInfo.DataRate.Modulation),
					Bandwidth:       uint32(fl.UplinkFrame.TXInfo.DataRate.Bandwidth),
					SpreadingFactor: uint32(fl.UplinkFrame.TXInfo.DataRate.SpreadFactor),
					Bitrate:         uint32(fl.UplinkFrame.TXInfo.DataRate.BitRate),
				},
				CodeRate: fl.UplinkFrame.TXInfo.CodeRate,
			},
			PhyPayload: b,
		}

		for i := range fl.UplinkFrame.RXInfoSet {
			rxInfo := ns.UplinkRXInfo{
				GatewayId: fl.UplinkFrame.RXInfoSet[i].MAC[:],
				Timestamp: fl.UplinkFrame.RXInfoSet[i].Timestamp,
				Rssi:      int32(fl.UplinkFrame.RXInfoSet[i].RSSI),
				LoraSnr:   float32(fl.UplinkFrame.RXInfoSet[i].LoRaSNR),
				Board:     uint32(fl.UplinkFrame.RXInfoSet[i].Board),
				Antenna:   uint32(fl.UplinkFrame.RXInfoSet[i].Antenna),
			}

			if fl.UplinkFrame.RXInfoSet[i].Time != nil {
				rxInfo.TimeUnixNs = fl.UplinkFrame.RXInfoSet[i].Time.UnixNano()
			}

			if fl.UplinkFrame.RXInfoSet[i].TimeSinceGPSEpoch != nil {
				rxInfo.NsSinceGpsEpoch = int64(*fl.UplinkFrame.RXInfoSet[i].TimeSinceGPSEpoch)
			}

			up.RxInfo = append(up.RxInfo, &rxInfo)
		}
	}

	if fl.DownlinkFrame != nil {
		b, err := fl.DownlinkFrame.PHYPayload.MarshalBinary()
		if err != nil {
			return nil, nil, errors.Wrap(err, "marshal phypayload error")
		}

		down = &ns.DownlinkFrameLog{
			TxInfo: &ns.DownlinkTXInfo{
				GatewayId:   fl.DownlinkFrame.TXInfo.MAC[:],
				Immediately: fl.DownlinkFrame.TXInfo.Immediately,
				Frequency:   uint32(fl.DownlinkFrame.TXInfo.Frequency),
				Power:       int32(fl.DownlinkFrame.TXInfo.Power),
				DataRate: &ns.DataRate{
					Modulation:      string(fl.DownlinkFrame.TXInfo.DataRate.Modulation),
					Bandwidth:       uint32(fl.DownlinkFrame.TXInfo.DataRate.Bandwidth),
					SpreadingFactor: uint32(fl.DownlinkFrame.TXInfo.DataRate.SpreadFactor),
					Bitrate:         uint32(fl.DownlinkFrame.TXInfo.DataRate.BitRate),
				},
				CodeRate: fl.DownlinkFrame.TXInfo.CodeRate,
				Board:    uint32(fl.DownlinkFrame.TXInfo.Board),
				Antenna:  uint32(fl.DownlinkFrame.TXInfo.Antenna),
			},
			PhyPayload: b,
		}

		if fl.DownlinkFrame.TXInfo.Timestamp != nil {
			down.TxInfo.Timestamp = uint32(*fl.DownlinkFrame.TXInfo.Timestamp)
		}

		if fl.DownlinkFrame.TXInfo.TimeSinceGPSEpoch != nil {
			down.TxInfo.NsSinceGpsEpoch = int64(*fl.DownlinkFrame.TXInfo.TimeSinceGPSEpoch)
		}

		if fl.DownlinkFrame.TXInfo.IPol != nil {
			down.TxInfo.PolarizationInversion = *fl.DownlinkFrame.TXInfo.IPol
		} else {
			down.TxInfo.PolarizationInversion = true
		}
	}

	return up, down, nil
}
