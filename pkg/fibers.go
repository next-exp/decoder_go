package decoder

import (
	"fmt"
)

func ReadFiberFEC(data []uint16, evtFormat *EventFormat, dateHeader *EventHeaderStruct, event *EventType) error {
	position := 0
	var time int = -1
	var current_bit int = 31

	fFecId := evtFormat.FecID
	// Fibers do not have ZS, only compression mode (same as PMTs)
	Compression := evtFormat.ZeroSuppression
	Baseline := evtFormat.Baseline
	bufferSamples := evtFormat.BufferSamples
	if evtFormat.FWVersion == 10 {
		if evtFormat.TriggerType >= 8 {
			bufferSamples = evtFormat.BufferSamples2
		}
	}

	if configuration.Verbosity > 2 {
		message := fmt.Sprintf("ReadFiberFEC: FecID=0x%02x, Compression=%t, Baseline=%t, bufferSamples=%d",
			fFecId, Compression, Baseline, bufferSamples)
		logger.Info(message, "fibers")
	}

	// Reading the payload
	var nextFT int32 = -1 //At start we don't know next FT value
	var nextFThm int32 = -1

	channelMask, chPositions, err := fibersChannelMask(evtFormat)
	if err != nil {
		logger.Error(err.Error())
		return err
	}
	initializeWaveforms(event.FibersLG, channelMask, bufferSamples)
	wfPointers := computeFiberWaveformPointerArray(event.FibersLG, channelMask, chPositions)

	// Write pedestal
	if Baseline {
		writeFiberPedestals(evtFormat, channelMask, event.FiberBaselinesLG)
	}

	for true {
		time++

		// Stop reading if we reach the last time bin of the waveform
		if time == int(bufferSamples) {
			break
		}

		if Compression {
			// Skip FTm
			if time == 0 {
				position++
			}
			position = decodeChargeIndiaFiberCompressed(data, position, wfPointers,
				&current_bit, huffmanCodesPmts, chPositions, uint32(time))
		} else {
			var FT int32 = int32(data[position]) & 0x0FFFF
			position++

			//If not ZS check next FT value, if not expected (0xffff) end of data
			computeNextFThm(&nextFT, &nextFThm, evtFormat)
			if FT != (nextFThm & 0x0FFFF) {
				errMessage := fmt.Errorf("evt %d, fecID: %d, nextFThm != FT: 0x%04x, 0x%04x",
					EventIdGetNbInRun(dateHeader.EventId), fFecId, (nextFThm & 0x0ffff), FT)
				logger.Error(errMessage.Error())
				break
			}
			position = decodeCharge(data, position, wfPointers, chPositions, uint32(time))
		}
	}
	return nil
}

func decodeChargeIndiaFiberCompressed(data []uint16, position int, waveforms []*[]int16,
	current_bit *int, huffman *HuffmanNode, channelMask []uint16, time uint32) int {
	var dataword uint32 = 0

	for _, channelID := range channelMask {
		if *current_bit < 16 {
			position++
			*current_bit += 16
		}
		// Pack two 16-bit words into a 32-bit word in the correct order
		dataword = (uint32(data[position]) << 16) | uint32(data[position+1])

		// Get previous value
		waveform := *waveforms[channelID%100]
		var previous int16 = 0
		if time > 0 {
			previous = waveform[time-1]
		}

		var control_code int32 = 123456
		wfvalue := int16(decode_compressed_value(int32(previous), dataword, control_code, current_bit, huffman))

		if configuration.Verbosity > 3 {
			message := fmt.Sprintf("Fiber ElecID %d, time %d, charge 0x%04x", channelID, time, wfvalue)
			logger.Info(message, "fibers")
		}

		waveform[time] = wfvalue
	}
	return position
}

func computeFiberWaveformPointerArray(waveforms map[uint16][]int16, chmask []uint16, positions []uint16) []*[]int16 {
	MAX_FIBERS_PER_FEC := 12
	wfPointers := make([]*[]int16, MAX_FIBERS_PER_FEC)
	for i, elecID := range chmask {
		position := positions[i]
		wf := waveforms[elecID]
		wfPointers[position] = &wf
	}
	return wfPointers
}

func fibersChannelMask(evtFormat *EventFormat) ([]uint16, []uint16, error) {
	channelMaskVec := make([]uint16, 0)
	// To avoid using the map for every waveform sample we are keeping another
	// vector with the pointers to the waveforms. This positions vector indicates
	// the position of the waveform in the waveforms pointer array.
	positions := make([]uint16, 0)

	var t uint16
	for t = 0; t < 16; t++ {
		active := CheckBit(evtFormat.ChannelMask, t)
		if active {
			elecID, err := computeFiberElecID(evtFormat.FecID, t, evtFormat.FWVersion)
			if err != nil {
				return nil, nil, err
			}
			channelMaskVec = append(channelMaskVec, elecID)
			positions = append(positions, computeFiberPosition(elecID))
		}
	}

	if configuration.Verbosity > 2 {
		message := fmt.Sprintf("Fiber channel mask: %v", channelMaskVec)
		logger.Info(message, "fibers")
	}
	return channelMaskVec, positions, nil
}

func computeFiberPosition(elecID uint16) uint16 {
	position := elecID % 100 / 2
	return position
}

func computeFiberElecID(fecID uint16, channel uint16, fwversion uint16) (uint16, error) {
	var elecID uint16

	if fwversion >= 10 {
		base, ok := fecElecIDBase[fecID]
		if !ok {
			return 0, fmt.Errorf("no elecID base found for FEC %d", fecID)
		}
		elecID = channel*2 + (fecID % 2) + base
	}

	return elecID, nil
}

func writeFiberPedestals(evtFormat *EventFormat, channelMask []uint16, baselines map[uint16]uint16) {
	for _, elecID := range channelMask {
		// Same 12-to-6 baseline mapping as PMTs
		baseline_index := ((elecID % 100) % 12) / 2
		baselines[elecID] = evtFormat.Baselines[baseline_index]
	}
}

func processFiberIds(event *EventType, configuration Configuration) {
	if configuration.Verbosity > 2 {
		message := fmt.Sprintf("processFiberIds: Initial FibersLG count=%d, ChannelsHG=%t",
			len(event.FibersLG), event.FiberConfig.ChannelsHG)
		logger.Info(message, "fibers")
	}

	// Iterate over all fiber waveforms read into FibersLG
	for elecID, waveform := range event.FibersLG {
		// High-low gain channels
		// Odd channels (101, 103, 105, ...) -> High Gain, remapped to even elecID (100, 102, 104, ...)
		if event.FiberConfig.ChannelsHG {
			if elecID%2 == 1 {
				evenElecID := elecID - 1
				// Hardware wiring bug: channels X17 and X19 have swapped HG/LG pairing
				// X17 pairs with X18 (not X16), X19 pairs with X16 (not X18)
				switch elecID % 100 {
				case 17:
					evenElecID = elecID + 1
				case 19:
					evenElecID = elecID - 3
				}
				event.FibersHG[evenElecID] = waveform
				event.FiberBaselinesHG[evenElecID] = event.FiberBaselinesLG[elecID]
				delete(event.FibersLG, elecID)
				delete(event.FiberBaselinesLG, elecID)
				if configuration.Verbosity > 2 {
					message := fmt.Sprintf("ChannelsHG: Moved elecID %d to FibersHG as elecID %d", elecID, evenElecID)
					logger.Info(message, "fibers")
				}
			}
		}
	}

	if configuration.Verbosity > 2 {
		message := fmt.Sprintf("processFiberIds: Final FibersLG count=%d, FibersHG count=%d",
			len(event.FibersLG), len(event.FibersHG))
		logger.Info(message, "fibers")
	}
}
