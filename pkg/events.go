package decoder

type EventType struct {
	RunNumber        uint32
	PmtWaveforms     map[uint16][]int16
	BlrWaveforms     map[uint16][]int16
	SipmWaveforms    map[uint16][]int16
	FibersLG         map[uint16][]int16
	FibersHG         map[uint16][]int16
	Baselines        map[uint16]uint16
	BlrBaselines     map[uint16]uint16
	FiberBaselinesLG map[uint16]uint16
	FiberBaselinesHG map[uint16]uint16
	EventID          uint32
	Timestamp        uint64
	TriggerConfig    TriggerData
	// Trigger type is not written correctly in the trigger FEC
	// the value has to be retrieved from the NEXT headers from PMT or SiPM
	TriggerType    uint16
	PmtConfig      PmtConfig
	FiberConfig    FiberConfig
	ExtTrgWaveform *[]int16
	PmtSumWaveform *[]int16
	PmtSumBaseline uint16
	Error          bool
}

type SensorsMap struct {
	Pmts   SensorMapping
	Fibers SensorMapping
	Sipms  SensorMapping
	// In DEMOPP the sensor ID for PMTs are 2,3,4...
	PmtIDOffset uint16
}

type SensorMapping struct {
	ToElecID   map[uint16]uint16
	ToSensorID map[uint16]uint16
}

type PmtConfig struct {
	Baselines  bool
	DualMode   bool
	ChannelsHG bool
}

type FiberConfig struct {
	Baselines  bool
	ChannelsHG bool
}
