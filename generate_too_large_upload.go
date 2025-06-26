package main

import (
	"bufio"
	"encoding/json"
	"os"
	"time"
)

type PayloadEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type Payload struct {
	Data []PayloadEntry `json:"data"`
}

func main() {
	// Approximate size of each DataPoint when marshaled to JSON:
	// "timestamp": "2018-01-01T00:00:00Z" (around 25-30 chars)
	// "value": 1.0 (around 3-10 chars depending on value)
	// plus JSON overhead (quotes, comma, braces)
	// Let's estimate about 50 bytes per PayloadEntry for a rough calculation.
	// We want 10MB, which is 10 * 1024 * 1024 bytes = 10485760 bytes.
	// So, number of entries = 10485760 / 50 = 209715.2. Let's aim for 210,000 entries.
	// This is an estimation, and the actual size might vary slightly.

	const targetPayloadMB = 10
	const estimatedBytesPerDataPoint = 50 // Roughly estimated JSON size of one PayloadEntry
	const numEntries = (targetPayloadMB * 1024 * 1024) / estimatedBytesPerDataPoint

	data := make([]PayloadEntry, 0, numEntries) // Pre-allocate slice for efficiency

	// Starting timestamp
	currentTime := time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)

	for i := range numEntries {
		dp := PayloadEntry{
			Timestamp: currentTime,
			Value:     float64(i + 1), // Incrementing value
		}
		data = append(data, dp)

		// Increment timestamp by 1 second for the next entry
		currentTime = currentTime.Add(time.Second)
	}

	payload := Payload{
		Data: data,
	}

	jsonOutput, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		panic(err)
	}

	f, err := os.Create("./example_export_upload_large.json")
	if err != nil {
		panic(err)
	}

	defer f.Close()

	writer := bufio.NewWriter(f)
	_, err = writer.Write(jsonOutput)
	if err != nil {
		panic(err)
	}
	writer.Flush()
}
