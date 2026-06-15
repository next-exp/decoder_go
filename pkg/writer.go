package decoder

import (
	"errors"
	"fmt"
	"reflect"
	"sort"

	hdf5 "github.com/next-exp/hdf5-go"
	"golang.org/x/exp/maps"
)

type Writer struct {
	File                *hdf5.File
	Filename            string
	FirstEvt            bool
	RunGroup            *hdf5.Group
	RDGroup             *hdf5.Group
	SensorsGroup        *hdf5.Group
	TriggerGroup        *hdf5.Group
	EventTable          *hdf5.Dataset
	RunInfoTable        *hdf5.Dataset
	TriggerParamsTable  *hdf5.Dataset
	TriggerTypeTable    *hdf5.Dataset
	TriggerLostTable    *hdf5.Dataset
	TriggerChannels     *hdf5.Dataset
	PmtMappingTable     *hdf5.Dataset
	BlrMappingTable     *hdf5.Dataset
	SipmMappingTable    *hdf5.Dataset
	PmtWaveforms        *hdf5.Dataset
	BlrWaveforms        *hdf5.Dataset
	ExtTrgWaveform      *hdf5.Dataset
	PmtSumWaveform      *hdf5.Dataset
	PmtSumBaseline      *hdf5.Dataset
	SipmWaveforms       *hdf5.Dataset
	FiberWaveformsLG    *hdf5.Dataset
	FiberWaveformsHG    *hdf5.Dataset
	Baselines           *hdf5.Dataset
	BlrBaselines        *hdf5.Dataset
	FiberBaselinesLG    *hdf5.Dataset
	FiberBaselinesHG    *hdf5.Dataset
	FiberMappingTableLG *hdf5.Dataset
	FiberMappingTableHG *hdf5.Dataset
	EvtCounter          int
}

const N_TRG_CH = 48

func NewWriter(filename string) (*Writer, error) {
	// Set string size for HDF5
	hdf5.SetStringLength(STRLEN)

	// So far we are not using Blosc
	if configuration.UseBlosc {
		blosc_version, blosc_date, err := hdf5.RegisterBlosc()
		_ = blosc_version
		_ = blosc_date
		//fmt.Println("Blosc version: ", blosc_version, " date: ", blosc_date)
		if err != nil {
			logger.Error(err.Error())
		}
	}

	var err error
	writer := &Writer{}
	writer.File, err = openFile(filename)
	if err != nil {
		return nil, err
	}
	writer.Filename = filename

	errs := make([]error, 0)
	writer.RunGroup, err = createGroup(writer.File, "Run")
	if err != nil {
		errs = append(errs, err)
	}
	writer.RDGroup, err = createGroup(writer.File, "RD")
	if err != nil {
		errs = append(errs, err)
	}
	writer.SensorsGroup, err = createGroup(writer.File, "Sensors")
	if err != nil {
		errs = append(errs, err)
	}
	writer.TriggerGroup, err = createGroup(writer.File, "Trigger")
	if err != nil {
		errs = append(errs, err)
	}
	writer.EventTable, err = createTable(writer.RunGroup, "events", EventDataHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.RunInfoTable, err = createTable(writer.RunGroup, "runInfo", RunInfoHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.TriggerParamsTable, err = createTable(writer.TriggerGroup, "configuration", TriggerParamsHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.TriggerLostTable, err = createTable(writer.TriggerGroup, "triggerLost", TriggerLostHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.TriggerTypeTable, err = createTable(writer.TriggerGroup, "trigger", TriggerTypeHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.PmtMappingTable, err = createTable(writer.SensorsGroup, "DataPMT", SensorMappingHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.SipmMappingTable, err = createTable(writer.SensorsGroup, "DataSiPM", SensorMappingHDF5{})
	if err != nil {
		errs = append(errs, err)
	}
	writer.EvtCounter = 0
	return writer, err
}

func sortSensorsBySensorID(sensorsFromElecIDToSensorID map[uint16]uint16) []SensorMappingHDF5 {
	// The array MUST be allocated at creation, if not, HDF5 will panic
	// doing appends will not work
	sorted := make([]SensorMappingHDF5, len(sensorsFromElecIDToSensorID))
	count := 0
	for elecID, sensorID := range sensorsFromElecIDToSensorID {
		sensor := SensorMappingHDF5{
			channel:  int32(elecID),
			sensorID: int32(sensorID),
		}
		sorted[count] = sensor
		count++
	}

	// Sort by sensorID
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].sensorID < sorted[j].sensorID
	})
	return sorted
}

func sortSensorsBySensorIDForWaveforms(dbMap map[uint16]uint16, waveforms map[uint16][]int16) []SensorMappingHDF5 {
	// The array MUST be allocated at creation, if not, HDF5 will panic
	// doing appends will not work
	sorted := make([]SensorMappingHDF5, len(waveforms))
	count := 0
	for elecID := range waveforms {
		sensorID, exists := dbMap[elecID]
		if !exists {
			sensorID = uint16(0xFFFF)
		}
		sorted[count] = SensorMappingHDF5{
			channel:  int32(elecID),
			sensorID: int32(sensorID),
		}
		count++
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].sensorID < sorted[j].sensorID
	})
	return sorted
}

func sortSensorsByElecID(sensors map[uint16][]int16) []SensorMappingHDF5 {
	// The array MUST be allocated at creation, if not, HDF5 will panic
	// doing appends will not work
	nSensors := len(sensors)
	sorted := make([]SensorMappingHDF5, nSensors)
	count := 0
	for elecID, _ := range sensors {
		sensor := SensorMappingHDF5{
			channel:  int32(elecID),
			sensorID: -1,
		}
		sorted[count] = sensor
		count++
	}

	// Sort by sensorID
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].channel < sorted[j].channel
	})
	return sorted
}

func buildSortedElecIDs(pmtWaveforms map[uint16][]int16, fiberWaveforms map[uint16][]int16) []uint16 {
	all := make([]uint16, 0, len(pmtWaveforms)+len(fiberWaveforms))
	for elecID := range pmtWaveforms {
		all = append(all, elecID)
	}
	for elecID := range fiberWaveforms {
		all = append(all, elecID)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	return all
}

func buildSortedSensorIDs(pmtSensors map[uint16]uint16, fiberSensors map[uint16]uint16) []uint16 {
	all := make([]uint16, 0, len(pmtSensors)+len(fiberSensors))
	for _, sensorID := range pmtSensors {
		all = append(all, sensorID)
	}
	for _, sensorID := range fiberSensors {
		all = append(all, sensorID)
	}
	sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
	return all
}

func (w *Writer) WriteEvent(event *EventType) {
	// Write event data
	evtTimestamp := EventDataHDF5{
		timestamp:  event.Timestamp,
		evt_number: int32(event.EventID),
	}

	writeEntryToTable(w.TriggerLostTable, TriggerLostHDF5{
		triggerLost1: int32(event.TriggerConfig.TriggerLost1),
		triggerLost2: int32(event.TriggerConfig.TriggerLost2),
	}, w.EvtCounter)

	writeEntryToTable(w.TriggerTypeTable, TriggerTypeHDF5{
		trigger_type: int32(event.TriggerType),
	}, w.EvtCounter)

	var pmtSorted, blrSorted, sipmSorted, fiberLGSorted, fiberHGSorted []SensorMappingHDF5
	var nPmts, nBlrs, nSipms, nFibersLG, nFibersHG int
	var pmtSamples, sipmSamples, fiberSamples int
	var nTrgChs int

	if configuration.NoDB {
		pmtSorted = sortSensorsByElecID(event.PmtWaveforms)
		blrSorted = sortSensorsByElecID(event.BlrWaveforms)
		sipmSorted = sortSensorsByElecID(event.SipmWaveforms)
		fiberLGSorted = sortSensorsByElecID(event.FibersLG)
		fiberHGSorted = sortSensorsByElecID(event.FibersHG)
		nPmts = len(event.PmtWaveforms)
		nSipms = len(event.SipmWaveforms)
		nTrgChs = len(buildSortedElecIDs(event.PmtWaveforms, event.FibersLG))
	} else {
		pmtSorted = sortSensorsBySensorID(sensorsMap.Pmts.ToSensorID)
		sipmSorted = sortSensorsBySensorID(sensorsMap.Sipms.ToSensorID)
		fiberLGSorted = sortSensorsBySensorIDForWaveforms(sensorsMap.Fibers.ToSensorID, event.FibersLG)
		fiberHGSorted = sortSensorsBySensorIDForWaveforms(sensorsMap.Fibers.ToSensorID, event.FibersHG)
		nPmts = len(pmtSorted)
		nSipms = len(sipmSorted)
		nTrgChs = len(buildSortedSensorIDs(sensorsMap.Pmts.ToSensorID, sensorsMap.Fibers.ToSensorID))
	}
	nBlrs = len(event.BlrWaveforms)
	nFibersLG = len(event.FibersLG)
	nFibersHG = len(event.FibersHG)

	if nPmts > 0 {
		if len(event.PmtWaveforms) > 0 {
			randomPmt := maps.Values(event.PmtWaveforms)[0]
			pmtSamples = len(randomPmt)
		} else {
			pmtSamples = 1
		}
	}

	if nSipms > 0 {
		if len(event.SipmWaveforms) > 0 {
			randomSipm := maps.Values(event.SipmWaveforms)[0]
			sipmSamples = len(randomSipm)
		} else {
			sipmSamples = 1
		}
	}

	if nFibersLG > 0 || nFibersHG > 0 {
		if len(event.FibersLG) > 0 {
			randomFiber := maps.Values(event.FibersLG)[0]
			fiberSamples = len(randomFiber)
		} else if len(event.FibersHG) > 0 {
			randomFiber := maps.Values(event.FibersHG)[0]
			fiberSamples = len(randomFiber)
		} else {
			fiberSamples = 1
		}
	}

	if !w.FirstEvt {
		writeEntryToTable(w.RunInfoTable, RunInfoHDF5{run_number: int32(event.RunNumber)}, w.EvtCounter)
		writeArrayToTable(w.PmtMappingTable, &pmtSorted, w.EvtCounter)
		writeArrayToTable(w.SipmMappingTable, &sipmSorted, w.EvtCounter)
		w.writeTriggerConfiguration(event.TriggerConfig)

		w.TriggerChannels = create2dArray(w.TriggerGroup, "events", nTrgChs)

		if nPmts > 0 {
			w.PmtWaveforms = create3dArray(w.RDGroup, "pmtrwf", nPmts, pmtSamples)
			w.Baselines = create2dArray(w.RDGroup, "pmt_baselines", nPmts)
		}
		if nSipms > 0 {
			w.SipmWaveforms = create3dArray(w.RDGroup, "sipmrwf", nSipms, sipmSamples)
		}

		if event.ExtTrgWaveform != nil {
			samples := len(*event.ExtTrgWaveform)
			w.ExtTrgWaveform = create2dArray(w.RDGroup, "ext_pmt", samples)
		}

		if event.PmtSumWaveform != nil {
			samples := len(*event.PmtSumWaveform)
			w.PmtSumWaveform = create2dArray(w.RDGroup, "pmt_sum", samples)
			w.PmtSumBaseline = create2dArray(w.RDGroup, "pmt_sum_baseline", 1)
		}

		if len(event.BlrWaveforms) > 0 {
			w.BlrWaveforms = create3dArray(w.RDGroup, "pmtblr", nPmts, pmtSamples)
			w.BlrBaselines = create2dArray(w.RDGroup, "blr_baselines", nPmts)
			w.BlrMappingTable, _ = createTable(w.SensorsGroup, "DataBLR", SensorMappingHDF5{})
			writeArrayToTable(w.BlrMappingTable, &blrSorted, w.EvtCounter)
		}

		if len(event.FibersLG) > 0 {
			w.FiberWaveformsLG = create3dArray(w.RDGroup, "fiberrwf_lg", nFibersLG, fiberSamples)
			w.FiberBaselinesLG = create2dArray(w.RDGroup, "fiber_baselines_lg", nFibersLG)
			w.FiberMappingTableLG, _ = createTable(w.SensorsGroup, "DataFiberLG", SensorMappingHDF5{})
			writeArrayToTable(w.FiberMappingTableLG, &fiberLGSorted, w.EvtCounter)
		}

		if len(event.FibersHG) > 0 {
			w.FiberWaveformsHG = create3dArray(w.RDGroup, "fiberrwf_hg", nFibersHG, fiberSamples)
			w.FiberBaselinesHG = create2dArray(w.RDGroup, "fiber_baselines_hg", nFibersHG)
			w.FiberMappingTableHG, _ = createTable(w.SensorsGroup, "DataFiberHG", SensorMappingHDF5{})
			writeArrayToTable(w.FiberMappingTableHG, &fiberHGSorted, w.EvtCounter)
		}

		w.FirstEvt = true
	}

	writeEntryToTable(w.EventTable, evtTimestamp, w.EvtCounter)

	// Write waveforms
	if nPmts > 0 {
		writeWaveforms(w.PmtWaveforms, event.PmtWaveforms, pmtSorted, w.EvtCounter, nPmts, pmtSamples)
		writeBaselines(w.Baselines, event.Baselines, pmtSorted, w.EvtCounter, nPmts)
	}
	if nBlrs > 0 {
		// This uses the same channel order as the PMTs
		// it works well when reading the channel map from DB
		// in no-DB mode, if there is a dual channel of a missing normal channel,
		// it will not be written.
		writeWaveforms(w.BlrWaveforms, event.BlrWaveforms, blrSorted, w.EvtCounter, nBlrs, pmtSamples)
		writeBaselines(w.BlrBaselines, event.BlrBaselines, pmtSorted, w.EvtCounter, nPmts)
	}
	if nSipms > 0 {
		writeWaveforms(w.SipmWaveforms, event.SipmWaveforms, sipmSorted, w.EvtCounter, nSipms, sipmSamples)
	}
	if nFibersLG > 0 {
		writeWaveforms(w.FiberWaveformsLG, event.FibersLG, fiberLGSorted, w.EvtCounter, nFibersLG, fiberSamples)
		writeBaselines(w.FiberBaselinesLG, event.FiberBaselinesLG, fiberLGSorted, w.EvtCounter, nFibersLG)
	}
	if nFibersHG > 0 {
		writeWaveforms(w.FiberWaveformsHG, event.FibersHG, fiberHGSorted, w.EvtCounter, nFibersHG, fiberSamples)
		writeBaselines(w.FiberBaselinesHG, event.FiberBaselinesHG, fiberHGSorted, w.EvtCounter, nFibersHG)
	}
	if event.ExtTrgWaveform != nil {
		writeSingleWaveform(w.ExtTrgWaveform, event.ExtTrgWaveform, w.EvtCounter)
	}
	if event.PmtSumWaveform != nil {
		writeSingleWaveform(w.PmtSumWaveform, event.PmtSumWaveform, w.EvtCounter)
		pmtSumBaseline := []int16{int16(event.PmtSumBaseline)}
		writeSingleWaveform(w.PmtSumBaseline, &pmtSumBaseline, w.EvtCounter)
	}

	if configuration.NoDB {
		sortedElecIDs := buildSortedElecIDs(event.PmtWaveforms, event.FibersLG)
		writeTriggerChannelsNoDB(w.TriggerChannels, event.TriggerConfig.TrgChannels, sortedElecIDs, w.EvtCounter)
	} else {
		sortedSensorIDs := buildSortedSensorIDs(sensorsMap.Pmts.ToSensorID, sensorsMap.Fibers.ToSensorID)
		writeTriggerChannels(w.TriggerChannels, event.TriggerConfig.TrgChannels,
			sensorsMap.Pmts.ToSensorID, sensorsMap.Fibers.ToSensorID, sortedSensorIDs, w.EvtCounter)
	}

	w.EvtCounter++
}

func writeTriggerChannels(dset *hdf5.Dataset, channels []uint16,
	pmtSensors map[uint16]uint16, fiberSensors map[uint16]uint16,
	sortedSensorIDs []uint16, evtCounter int) {
	nTrgChs := len(sortedSensorIDs)
	posMap := make(map[uint16]int, nTrgChs)
	for i, sensorID := range sortedSensorIDs {
		posMap[sensorID] = i
	}
	trgChannels := make([]int16, nTrgChs)
	for _, elecid := range channels {
		sensorID, exists := pmtSensors[elecid]
		if !exists {
			// Normalize odd (HG) fiber elecIDs to even before lookup
			lookupID := elecid
			if elecid%2 == 1 {
				lookupID = elecid - 1
			}
			sensorID, exists = fiberSensors[lookupID]
		}
		if exists {
			if pos, ok := posMap[sensorID]; ok {
				trgChannels[pos] = 1
			}
		} else {
			fmt.Println("Trigger channel not found in mapping: ", elecid)
		}
	}
	write2dArray(dset, &trgChannels, evtCounter, nTrgChs)
}

func writeTriggerChannelsNoDB(dset *hdf5.Dataset, channels []uint16, sortedElecIDs []uint16, evtCounter int) {
	nTrgChs := len(sortedElecIDs)
	posMap := make(map[uint16]int, nTrgChs)
	for i, elecID := range sortedElecIDs {
		posMap[elecID] = i
	}
	trgChannels := make([]int16, nTrgChs)
	for _, elecid := range channels {
		lookupID := elecid
		// Normalize odd (HG) fiber elecIDs to even before lookup
		if elecid%2 == 1 {
			if _, ok := posMap[elecid-1]; ok {
				lookupID = elecid - 1
			}
		}
		if pos, ok := posMap[lookupID]; ok {
			trgChannels[pos] = 1
		} else {
			fmt.Println("Trigger channel not found: ", elecid)
		}
	}
	write2dArray(dset, &trgChannels, evtCounter, nTrgChs)
}

func writeWaveforms(dset *hdf5.Dataset, waveforms map[uint16][]int16,
	order []SensorMappingHDF5, evtCounter int, nSensors int, nSamples int) {
	data := make([]int16, nSensors*nSamples)
	for i, sensor := range order {
		// Write only if the corresponding sensor has data
		// if not, the data array will be filled with zeros for that sensor
		if _, ok := waveforms[uint16(sensor.channel)]; !ok {
			continue
		}
		for j, sample := range waveforms[uint16(sensor.channel)] {
			data[i*nSamples+j] = int16(sample)
		}
	}
	write3dArray(dset, &data, evtCounter, nSensors, nSamples)
}

func writeSingleWaveform(dset *hdf5.Dataset, waveform *[]int16, evtCounter int) {
	nSamples := len(*waveform)
	data := make([]int16, nSamples)
	for i, value := range *waveform {
		data[i] = value
	}
	write2dArray(dset, &data, evtCounter, nSamples)
}

func writeBaselines(dset *hdf5.Dataset, baselines map[uint16]uint16,
	order []SensorMappingHDF5, evtCounter int, nSensors int) {
	data := make([]int16, nSensors)
	for i, sensor := range order {
		// Write only if the corresponding sensor has data
		// if not, the baseline will be zero
		if _, ok := baselines[uint16(sensor.channel)]; !ok {
			continue
		}
		data[i] = int16(baselines[uint16(sensor.channel)])
	}
	write2dArray(dset, &data, evtCounter, nSensors)
}

func (w *Writer) Close() error {
	var errs []error

	if w.EventTable != nil {
		if err := w.EventTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing event table: %w", err))
		}
	}
	if w.RunInfoTable != nil {
		if err := w.RunInfoTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing run info table: %w", err))
		}
	}
	if w.PmtWaveforms != nil {
		if err := w.PmtWaveforms.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PMT waveforms: %w", err))
		}
	}
	if w.Baselines != nil {
		if err := w.Baselines.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PMT baselines: %w", err))
		}
	}
	if w.SipmWaveforms != nil {
		if err := w.SipmWaveforms.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing SiPM waveforms: %w", err))
		}
	}
	if w.FiberWaveformsLG != nil {
		if err := w.FiberWaveformsLG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber LG waveforms: %w", err))
		}
	}
	if w.FiberWaveformsHG != nil {
		if err := w.FiberWaveformsHG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber HG waveforms: %w", err))
		}
	}
	if w.FiberBaselinesLG != nil {
		if err := w.FiberBaselinesLG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber LG baselines: %w", err))
		}
	}
	if w.FiberBaselinesHG != nil {
		if err := w.FiberBaselinesHG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber HG baselines: %w", err))
		}
	}
	if w.ExtTrgWaveform != nil {
		if err := w.ExtTrgWaveform.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing external trigger waveform: %w", err))
		}
	}
	if w.PmtSumWaveform != nil {
		if err := w.PmtSumWaveform.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PMT sum waveform: %w", err))
		}
	}
	if w.PmtSumBaseline != nil {
		if err := w.PmtSumBaseline.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PMT sum baselines: %w", err))
		}
	}
	if w.BlrWaveforms != nil {
		if err := w.BlrWaveforms.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing BLR waveforms: %w", err))
		}
	}
	if w.BlrBaselines != nil {
		if err := w.BlrBaselines.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing BLR baselines: %w", err))
		}
	}
	if w.PmtMappingTable != nil {
		if err := w.PmtMappingTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing PMT mapping table: %w", err))
		}
	}
	if w.BlrMappingTable != nil {
		if err := w.BlrMappingTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing BLR mapping table: %w", err))
		}
	}
	if w.SipmMappingTable != nil {
		if err := w.SipmMappingTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing SiPM mapping table: %w", err))
		}
	}
	if w.FiberMappingTableLG != nil {
		if err := w.FiberMappingTableLG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber LG mapping table: %w", err))
		}
	}
	if w.FiberMappingTableHG != nil {
		if err := w.FiberMappingTableHG.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing Fiber HG mapping table: %w", err))
		}
	}
	if w.TriggerParamsTable != nil {
		if err := w.TriggerParamsTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing trigger params table: %w", err))
		}
	}
	if w.TriggerLostTable != nil {
		if err := w.TriggerLostTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing trigger lost table: %w", err))
		}
	}
	if w.TriggerTypeTable != nil {
		if err := w.TriggerTypeTable.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing trigger type table: %w", err))
		}
	}
	if w.TriggerChannels != nil {
		if err := w.TriggerChannels.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing trigger channels: %w", err))
		}
	}
	if w.RunGroup != nil {
		if err := w.RunGroup.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing run group: %w", err))
		}
	}
	if w.RDGroup != nil {
		if err := w.RDGroup.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing RD group: %w", err))
		}
	}
	if w.SensorsGroup != nil {
		if err := w.SensorsGroup.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing sensors group: %w", err))
		}
	}
	if w.TriggerGroup != nil {
		if err := w.TriggerGroup.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing trigger group: %w", err))
		}
	}
	if w.File != nil {
		if err := w.File.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing file: %w", err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (w *Writer) writeTriggerConfiguration(params TriggerData) {
	t := reflect.TypeOf(params)
	n := t.NumField()
	entries := make([]TriggerParamsHDF5, n)

	fieldsToWrite := 0
	for i := 0; i < n; i++ {
		f := t.Field(i)
		paramName := f.Tag.Get("hdf5")
		// Write only single-value fields, not the slices with trigger channels
		switch {
		case f.Type.Kind() == reflect.Uint16:
			value := reflect.ValueOf(params).Field(i).Interface().(uint16)
			entry := TriggerParamsHDF5{
				paramStr: convertToHdf5String(paramName),
				value:    int32(value),
			}
			entries[fieldsToWrite] = entry
			fieldsToWrite++
		case f.Type.Kind() == reflect.Uint32:
			value := reflect.ValueOf(params).Field(i).Interface().(uint32)
			entry := TriggerParamsHDF5{
				paramStr: convertToHdf5String(paramName),
				value:    int32(value),
			}
			entries[fieldsToWrite] = entry
			fieldsToWrite++
		}
	}
	toWrite := entries[:fieldsToWrite]
	writeArrayToTable(w.TriggerParamsTable, &toWrite, w.EvtCounter)
}

func ProcessDecodedEvent(event EventType, configuration Configuration,
	writer *Writer, writer2 *Writer) {
	if configuration.WriteData && !event.Error {
		if configuration.SplitTrg {
			switch int(event.TriggerType) {
			case configuration.TrgCode1:
				writer.WriteEvent(&event)
			case configuration.TrgCode2:
				writer2.WriteEvent(&event)
			}
		} else {
			writer.WriteEvent(&event)
		}
	}
}
