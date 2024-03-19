package golevel7

import (
	"bytes"
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/net/html/charset"
)

func readFile(fname string) ([]byte, error) {
	file, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader, err := charset.NewReader(file, "text/plain")
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func TestMessage(t *testing.T) {
	data, err := readFile("./testdata/msg.hl7")
	if err != nil {
		t.Fatal(err)
	}

	msg := &Message{Value: []rune(string(data))}
	err = msg.parse()
	if err != nil {
		t.Error(err)
	}
	if len(msg.Segments) != 5 {
		t.Errorf("Expected 5 segments got %d\n", len(msg.Segments))
	}

	data, err = readFile("./testdata/msg2.hl7")
	if err != nil {
		t.Fatal(err)
	}
	msg = &Message{Value: []rune(string(data))}
	err = msg.parse()
	if err != nil {
		t.Error(err)
	}
	if len(msg.Segments) != 5 {
		t.Errorf("Expected 5 segments got %d\n", len(msg.Segments))
	}

	data, err = readFile("./testdata/msg3.hl7")
	if err != nil {
		t.Fatal(err)
	}
	msg = &Message{Value: []rune(string(data))}
	err = msg.parse()
	if err != nil {
		t.Error(err)
	}
	if len(msg.Segments) != 9 {
		t.Errorf("Expected 9 segments got %d\n", len(msg.Segments))
	}

	data, err = readFile("./testdata/msg4.hl7")
	if err != nil {
		t.Fatal(err)
	}
	msg = &Message{Value: []rune(string(data))}
	err = msg.parse()
	if err != nil {
		t.Error(err)
	}
	if len(msg.Segments) != 9 {
		t.Errorf("Expected 9 segments got %d\n", len(msg.Segments))
	}
}

func TestMsgUnmarshal(t *testing.T) {
	fname := "./testdata/msg.hl7"
	file, err := os.Open(fname)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	msgs, err := NewDecoder(file).Messages()
	if err != nil {
		t.Fatal(err)
	}
	st := my7{}
	msgs[0].Unmarshal(&st)

	if st.FirstName != "John" {
		t.Errorf("Expected John got %s\n", st.FirstName)
	}
	if st.LastName != "Jones" {
		t.Errorf("Expected Jones got %s\n", st.LastName)
	}
}

type Observation struct {
	segment      struct{} `hl7:"OBX,repeating"`
	SetID        string   `hl7:"OBX.1"`
	ValueType    string   `hl7:"OBX.2"`
	Identifier   string   `hl7:"OBX.3"`
	SubID        string   `hl7:"OBX.4"`
	Value        string   `hl7:"OBX.5"`
	Units        string   `hl7:"OBX.6"`
	ResultStatus string   `hl7:"OBX.11"`
	// SpecimenReceivedDateTime string   `hl7:"OBX.14"`
	// EquipmentInstanceId      string   `hl7:"OBX.18"`
}

type MessageHeader struct {
	segment                       struct{}    `hl7:"MSH"`
	SendingApp                    string      `hl7:"MSH.3"`
	SendingFacility               string      `hl7:"MSH.4"`
	ReceivingApp                  string      `hl7:"MSH.5"`
	ReceivingFacility             string      `hl7:"MSH.6"`
	MsgDate                       string      `hl7:"MSH.7"`
	Security                      string      `hl7:"MSH.8"`
	MessageCode                   MessageType `hl7:"MSH.9"`
	SimpleMessageCode             string      `hl7:"MSH.9.1"`
	ControlID                     string      `hl7:"MSH.10"`
	ProcessingID                  string      `hl7:"MSH.11"`
	VersionID                     string      `hl7:"MSH.12"`
	AcceptAcknowledgmentType      string      `hl7:"MSH.15"`
	ApplicationAcknowledgmentType string      `hl7:"MSH.16"`
}

type MessageType struct {
	segment          struct{} `hl7:"MSH.9"`
	MessageCode      string   `hl7:"MSH.9.1"`
	TriggerEvent     string   `hl7:"MSH.9.2"`
	MessageStructure string   `hl7:"MSH.9.3"`
}

type PatientVisit struct {
	segment         struct{}                `hl7:"PV1"`
	PatientClass    string                  `hl7:"PV1.2"`
	Location        AssignedPatientLocation `hl7:"PV1.3"`
	AttendingDoctor []PersonIdentifier      `hl7:"PV1.7"`
}

type PersonIdentifier struct {
	segment            struct{} `hl7:"PID.3.repeating"`
	PersonalIdentifier string   `hl7:"PID.3.1"`
}

type OBRMessage struct {
	Header       MessageHeader `hl7:"MSH"`
	Visit        PatientVisit  `hl7:"PV1"`
	Observations []Observation `hl7:"OBX"`
	Roles        []RoleSegment `hl7:"ROL"`
}

type AssignedPatientLocation struct {
	segment     struct{} `hl7:"PV1.3"`
	PointOfCare string   `hl7:"PV1.3.1"`
	Room        string   `hl7:"PV1.3.2"`
	Bed         string   `hl7:"PV1.3.3"`
	Facility    string   `hl7:"PV1.3.4"`
}

type RoleSegment struct {
	segment        struct{}           `hl7:"ROL,repeating"`
	RoleInstanceId string             `hl7:"ROL.1"`
	RoleActionCode string             `hl7:"ROL.2"`
	Role           string             `hl7:"ROL.3"`
	RolePersons    []PersonIdentifier `hl7:"ROL.4"`
	RoleBegin      string             `hl7:"ROL.5"`
	RoleEnd        string             `hl7:"ROL.6"`
	// RoleDuration        string `hl7:"ROL.7"`
}

func TestTaggedStructParsing(t *testing.T) {
	t.Logf("TestTaggedStructParsing")
	data, err := readFile("./testdata/msg5.hl7")
	if err != nil {
		t.Fatal(err)
	}

	msgs, err := NewDecoder(bytes.NewReader(data)).Messages()
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, 1, len(msgs))
	// t.Logf("msgs: %#v", msgs[0])

	t.Log("Unmarshalling")
	obr := &OBRMessage{}
	err = msgs[0].ToStruct(obr)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("obr: %#v", obr)

	t.Logf("Header: %#v", obr.Header)
	t.Logf("Visit: %#v", obr.Visit)
	for _, obx := range obr.Observations {
		t.Logf("obx: %#v", obx)
	}

	assert.Equal(t, "ADT1", obr.Header.SendingApp)
	assert.Equal(t, "MCM", obr.Header.SendingFacility)
	assert.Equal(t, "FINGER", obr.Header.ReceivingApp)
	assert.Equal(t, "MCM", obr.Header.ReceivingFacility)
	assert.Equal(t, "198808181126", obr.Header.MsgDate)
	assert.Equal(t, "ADT", obr.Header.MessageCode.MessageCode)
	assert.Equal(t, "A01", obr.Header.MessageCode.TriggerEvent)
	assert.Equal(t, "ADT", obr.Header.SimpleMessageCode)
	assert.Equal(t, "MSG00001", obr.Header.ControlID)
	assert.Equal(t, "P", obr.Header.ProcessingID)
	assert.Equal(t, "2.3.1", obr.Header.VersionID)

	// check the role segments
	assert.Equal(t, 5, len(obr.Roles))
	role := obr.Roles[0]
	assert.Equal(t, "1", role.RoleInstanceId)
	assert.Equal(t, "UP", role.RoleActionCode)
	assert.Equal(t, "Patient Care", role.Role)
	assert.Equal(t, 2, len(role.RolePersons))
	assert.Equal(t, "10535", role.RolePersons[0].PersonalIdentifier)
	assert.Equal(t, "10536", role.RolePersons[1].PersonalIdentifier)

	role = obr.Roles[1]
	assert.Equal(t, "2", role.RoleInstanceId)
	assert.Equal(t, "D", role.RoleActionCode)
	assert.Equal(t, "Consult MD", role.Role)
	assert.Equal(t, 1, len(role.RolePersons))

	role = obr.Roles[2]
	assert.Equal(t, "3", role.RoleInstanceId)
	assert.Equal(t, "UP", role.RoleActionCode)
	assert.Equal(t, "Respiratory", role.Role)
	assert.Equal(t, 1, len(role.RolePersons))

	role = obr.Roles[3]
	assert.Equal(t, "4", role.RoleInstanceId)
	assert.Equal(t, "UP", role.RoleActionCode)
	assert.Equal(t, "RN", role.Role)
	assert.Equal(t, 1, len(role.RolePersons))

	// check the visit
	assert.Equal(t, "I", obr.Visit.PatientClass)
	assert.Equal(t, 1, len(obr.Visit.AttendingDoctor))
	assert.Equal(t, "004777", obr.Visit.AttendingDoctor[0].PersonalIdentifier)

	// check visit location
	assert.Equal(t, "2000", obr.Visit.Location.PointOfCare)
	assert.Equal(t, "2012", obr.Visit.Location.Room)
	assert.Equal(t, "01", obr.Visit.Location.Bed)
	assert.Equal(t, "", obr.Visit.Location.Facility)
}
