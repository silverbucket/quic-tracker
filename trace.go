package quictracker

import (
	"os/exec"
	"time"
	"strings"
	"unsafe"
)

// Contains the result of a test run against a given host.
type Trace struct {
	Commit              string                 `json:"commit"`     // The git commit that versions the code that produced the trace
	Scenario            string                 `json:"scenario"`   // The id of the scenario that produced the trace
	ScenarioVersion     int                    `json:"scenario_version"`
	Host                string                 `json:"host"`       // The host against which the scenario was run
	Ip                  string                 `json:"ip"`         // The IP that was resolved for the given host
	Results             map[string]interface{} `json:"results"`    // A dictionary that allows to report scenario-specific results
	StartedAt           int64                  `json:"started_at"` // The time at which the scenario started in epoch seconds
	Duration            uint64                 `json:"duration"`   // Its duration in epoch milliseconds
	ErrorCode           uint8                  `json:"error_code"` // A scenario-specific error code that reports its verdict
	Stream              []TracePacket          `json:"stream"`     // A clear-text copy of the packets that were sent and received
	Pcap                []byte                 `json:"pcap"`       // The packet capture file associated with the trace
	DecryptedPcap       []byte                 `json:"decrypted_pcap"`
	ClientRandom        []byte                 `json:"client_random"`
	ExporterSecret      []byte                 `json:"exporter_secret"`
	EarlyExporterSecret []byte                 `json:"early_exporter_secret"`
}

func NewTrace(scenarioName string, scenarioVersion int, host string) *Trace {
	trace := Trace{
		Scenario:        scenarioName,
		ScenarioVersion: scenarioVersion,
		Commit:          GitCommit(),
		Host:            host,
		StartedAt:       time.Now().Unix(),
		Results:         make(map[string]interface{}),
	}

	return &trace
}

func (t *Trace) AddPcap(conn *Connection, cmd *exec.Cmd) error {
	content, err := StopPcapCapture(conn, cmd)
	if err != nil {
		return err
	}
	t.Pcap = content
	return err
}

func (t *Trace) MarkError(error uint8, message string, packet Packet) {
	t.ErrorCode = error
	if message != "" {
		t.Results["error"] = message
	}
	if packet == nil {
		return
	}
	for _, tp := range t.Stream {
		if tp.Pointer == packet.Pointer() {
			tp.IsOfInterest = true
			return
		}
	}
}

func (t *Trace) AttachTo(conn *Connection) {
	conn.ReceivedPacketHandler = func(data []byte, origin unsafe.Pointer) {
		t.Stream = append(t.Stream, TracePacket{Direction: ToClient, Timestamp: time.Now().UnixNano() / 1e6, Data: data, Pointer: origin})
	}
	conn.SentPacketHandler = func(data []byte, origin unsafe.Pointer) {
		t.Stream = append(t.Stream, TracePacket{Direction: ToServer, Timestamp: time.Now().UnixNano() / 1e6, Data: data, Pointer: origin})
	}
}

func (t *Trace) Complete(conn *Connection) {
	t.ClientRandom = conn.ClientRandom
	t.ExporterSecret = conn.ExporterSecret
	t.EarlyExporterSecret = conn.Tls.EarlyExporterSecret()
}

type Direction string

const ToServer Direction = "to_server"
const ToClient Direction = "to_client"

type TracePacket struct {
	Direction    Direction      `json:"direction"`
	Timestamp    int64          `json:"timestamp"`
	Data         []byte         `json:"data"`
	IsOfInterest bool           `json:"is_of_interest"`
	Pointer      unsafe.Pointer `json:"-"`
}

func GitCommit() string {
	var (
		cmdOut []byte
		err    error
	)
	cmdName := "git"
	cmdArgs := []string{"rev-parse", "--verify", "HEAD"}
	if cmdOut, err = exec.Command(cmdName, cmdArgs...).Output(); err != nil {
		panic(err)
	}
	return strings.TrimSpace(string(cmdOut))
}