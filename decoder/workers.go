package main

import (
	"fmt"
	"io"
	"time"

	decoder "github.com/next-exp/decoder_go/pkg"
)

type WorkerData struct {
	Data   []byte
	Header decoder.EventHeaderStruct
}

func worker(id int, jobs <-chan WorkerData, results chan<- decoder.EventType) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Worker %d recovered from panic: %v\n", id, r)
			results <- decoder.EventType{Error: true}
		}
	}()

	for event := range jobs {
		fmt.Printf("Worker %d processing event %d\n", id, event.Header.EventId)
		//fmt.Println("Data size:", len(event.Data), "Header: ", event.Header)
		event, _ := decoder.ReadGDC(event.Data, event.Header)
		results <- event
	}
}

func sendEventsToWorkers(fileReader *FileReader, jobs chan<- WorkerData) {
	for {
		header, eventData, err := fileReader.getNextEvent()
		fmt.Printf("Reading event %d\n", decoder.EventIdGetNbInRun(header.EventId))
		if err != nil {
			fmt.Println("Error reading event:", err)
			break
		}
		if err == io.EOF {
			break
		}
		jobs <- WorkerData{Data: eventData, Header: header}
	}
	close(jobs)
}

func processWorkerResults(results chan decoder.EventType, writer *decoder.Writer,
	writer2 *decoder.Writer, evtsToRead int) {
	evtsProcessed := 0
	var totalTime int64 = 0
	fmt.Println("Waiting for events")
	for event := range results {
		fmt.Println("Processed event: ", evtsProcessed, event.EventID)
		start := time.Now()
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

		evtsProcessed++
		if evtsProcessed >= evtsToRead {
			break
		}

		duration := time.Since(start)
		totalTime += duration.Milliseconds()
	}
	fmt.Println("Total time writing: ", totalTime)
}
