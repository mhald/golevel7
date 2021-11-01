package golevel7

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strings"

	"golang.org/x/net/html/charset"
)

// Message is an HL7 message
type Message struct {
	Segments   []Segment
	Value      []rune
	Delimeters Delimeters
}

// NewMessage returns a new message with the v byte value
func NewMessage(v []byte) *Message {
	var utf8V []byte
	if len(v) != 0 {
		reader, err := charset.NewReader(bytes.NewReader(v), "text/plain")
		if err != nil {
			return nil
		}
		utf8V, err = ioutil.ReadAll(reader)
	} else {
		utf8V = v
	}
	newMessage := &Message{
		Value:      []rune(string(utf8V)),
		Delimeters: *NewDelimeters(),
	}
	if err := newMessage.parse(); err != nil {
		log.Fatal(fmt.Sprintf("Parse Error: %+v", err))
	}
	return newMessage
}

func (m *Message) String() string {
	// var str string
	str := "-------- Message --------\n"
	for _, s := range m.Segments {
		str += s.String()
	}
	str += "---------- End ----------\n\n"
	return str
}

// Segment returns the first matching segmane with name s
func (m *Message) Segment(s string) (*Segment, error) {
	for i, seg := range m.Segments {
		fld, err := seg.Field(0)
		if err != nil {
			continue
		}
		if string(fld.Value) == s {
			return &m.Segments[i], nil
		}
	}
	return nil, fmt.Errorf("Segment not found")
}

func (m *Message) LastSegment(s string) (*Segment, error) {
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := m.Segments[i]
		fld, err := seg.Field(0)
		if err != nil {
			continue
		}
		if string(fld.Value) == s {
			return &m.Segments[i], nil
		}
	}
	return nil, fmt.Errorf("Segment not found")
}

// AllSegments returns the first matching segmane with name s
func (m *Message) AllSegments(s string) ([]*Segment, error) {
	segs := []*Segment{}
	for i, seg := range m.Segments {
		fld, err := seg.Field(0)
		if err != nil {
			continue
		}
		if string(fld.Value) == s {
			segs = append(segs, &m.Segments[i])
		}
	}
	if len(segs) == 0 {
		return segs, fmt.Errorf("Segment not found")
	}
	return segs, nil
}

// Find gets a value from a message using location syntax
// finds the first occurence of the segment and first of repeating fields
// if the loc is not valid an error is returned
func (m *Message) Find(loc string) (string, error) {
	return m.Get(NewLocation(loc))
}

// FindAll gets all values from a message using location syntax
// finds all occurrences of the segments and all repeating fields
// if the loc is not valid an error is returned
func (m *Message) FindAll(loc string) ([]string, error) {
	return m.GetAll(NewLocation(loc))
}

func (m *Message) findObjects(loc string) ([]ValueGetter, error) {
	return m.getObjects(NewLocation(loc))
}

type ValueGetter interface {
    Get(loc *Location) (string, error)
}

// Get returns the first value specified by the Location
func (m *Message) Get(l *Location) (string, error) {
	if l.Segment == "" {
		return string(m.Value), nil
	}
	seg, err := m.Segment(l.Segment)
	if err != nil {
		return "", err
	}
	return seg.Get(l)
}

// GetAll returns all values specified by the Location
func (m *Message) GetAll(l *Location) ([]string, error) {
	vals := []string{}
	if l.Segment == "" {
		vals = append(vals, string(m.Value))
		return vals, nil
	}
	segs, err := m.AllSegments(l.Segment)
	if err != nil {
		return vals, err
	}
	for _, s := range segs {
		vs, err := s.GetAll(l)
		if err != nil {
			return vals, err
		}
		vals = append(vals, vs...)
	}
	return vals, nil
}

func (m *Message) getObjects(l *Location) ([]ValueGetter, error) {
	vals := []ValueGetter{}
	if l.Segment == "" {
		vals = append(vals, m)
		return vals, nil
	}
	segs, err := m.AllSegments(l.Segment)
	if err != nil {
		return vals, err
	}
	for _, s := range segs {
		vs, err := s.getObjects(l)
		if err != nil {
			return vals, err
		}
		vals = append(vals, vs...)
	}
	return vals, nil
}

// Set will insert a value into a message at Location
func (m *Message) Set(l *Location, val string) error {
	if l.Segment == "" {
		return errors.New("Segment is required")
	}
	seg, err := m.Segment(l.Segment)
	if err != nil {
		s := Segment{}
		s.forceField([]rune(l.Segment), 0)
		s.Set(l, val, &m.Delimeters)
		m.Segments = append(m.Segments, s)
	} else {
		seg.Set(l, val, &m.Delimeters)
	}
	m.Value = m.encode()
	return nil
}

func (m *Message) SetLast(l *Location, val string) error {
	if l.Segment == "" {
		return errors.New("Segment is required")
	}
	seg, err := m.LastSegment(l.Segment)
	if err != nil {
		s := Segment{}
		s.forceField([]rune(l.Segment), 0)
		s.Set(l, val, &m.Delimeters)
		m.Segments = append(m.Segments, s)
	} else {
		seg.Set(l, val, &m.Delimeters)
	}
	m.Value = m.encode()
	return nil
}

func (m *Message) parse() error {
	m.Value = []rune(strings.Trim(string(m.Value), "\n\r\x1c\x0b"))
	if m.Delimeters.DelimeterField == "" { // BUGFIX: only parse if needed
		if err := m.parseSep(); err != nil {
			return err
		}
	}
	r := strings.NewReader(string(m.Value))
	i := 0
	ii := 0
	for {
		ch, _, err := r.ReadRune()
		if err != nil {
			ch = eof
		}
		ii++
		switch {
		case ch == eof || (ch == endMsg && m.Delimeters.LFTermMsg):
			//just for safety: cannot reproduce this on windows
			safeii := map[bool]int{true: len(m.Value), false: ii}[ii > len(m.Value)]
			v := m.Value[i:safeii]
			if len(v) > 4 { // seg name + field sep
				seg := Segment{Value: v}
				seg.parse(&m.Delimeters)
				m.Segments = append(m.Segments, seg)
			}
			return nil
		case ch == segTerm:
			seg := Segment{Value: m.Value[i : ii-1]}
			seg.parse(&m.Delimeters)
			m.Segments = append(m.Segments, seg)
			i = ii
		case ch == m.Delimeters.Escape:
			ii++
			r.ReadRune()
		}
	}
}

func (m *Message) parseSep() error {
	if len(m.Value) < 8 {
		return errors.New("Invalid message length less than 8 bytes")
	}
	if string(m.Value[:3]) != "MSH" {
		return fmt.Errorf("Invalid message: Missing MSH segment -> %v", m.Value[:3])
	}

	r := bytes.NewReader([]byte(string(m.Value)))
	for i := 0; i < 8; i++ {
		ch, _, _ := r.ReadRune()
		if ch == eof {
			return fmt.Errorf("Invalid message: eof while parsing MSH")
		}
		switch i {
		case 3:
			m.Delimeters.Field = ch
		case 4:
			m.Delimeters.DelimeterField = string(ch)
			m.Delimeters.Component = ch
		case 5:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.Repetition = ch
		case 6:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.Escape = ch
		case 7:
			m.Delimeters.DelimeterField += string(ch)
			m.Delimeters.SubComponent = ch
		}
	}
	return nil
}

func (m *Message) encode() []rune {
	buf := [][]byte{}
	for _, s := range m.Segments {
		buf = append(buf, []byte(string(s.Value)))
	}
	return []rune(string(bytes.Join(buf, []byte(string(segTerm)))))
}

// IsValid checks a message for validity based on a set of criteria
// it returns valid and any failed validation rules
func (m *Message) IsValid(val []Validation) (bool, []Validation) {
	failures := []Validation{}
	valid := true
	for _, v := range val {
		values, err := m.FindAll(v.Location)
		if err != nil || len(values) == 0 {
			valid = false
			failures = append(failures, v)
		}
		for _, value := range values {
			if value == "" || (v.VCheck == SpecificValue && v.Value != value) {
				valid = false
				failures = append(failures, v)
			}
		}
	}

	return valid, failures
}

// Unmarshal fills a structure from an HL7 message
// It will panic if interface{} is not a pointer to a struct
// Unmarshal will decode the entire message before trying to set values
// it will set the first matching segment / first matching field
// repeating segments and fields is not well suited to this
// for the moment all unmarshal target fields must be strings
func (m *Message) Unmarshal(it interface{}) error {
	st := reflect.ValueOf(it).Elem()
	stt := st.Type()
	for i := 0; i < st.NumField(); i++ {
		fld := stt.Field(i)
		r := fld.Tag.Get("hl7")
		if fld.Type.Kind() == reflect.Slice {
			//
			// We are unmarshalling into a slice, so we will create the slice, find the relevant objects via the
			// hl7 tag, and then iterating over the associated struct to creating new slice elements populated
			// with the desired hl7 data.
			//
			// Limitations:
			// - original struct cannot use pointers to the slice elements (eg []Foo and not []*Foo)
			// - only supports one level of nesting
			// - only supports string fields
			//
			slice := reflect.MakeSlice(fld.Type, 0, 0)
			valuePtr := reflect.New(fld.Type.Elem()).Elem()
			if r != "" {
				tagParts := strings.Split(r, ",")
				objs, err := m.findObjects(tagParts[0])
				if err != nil {
					return err
				}

				for _, obj := range objs {
					newSliceObj := reflect.ValueOf(valuePtr.Addr().Interface()).Elem()
					newSliceObjType := newSliceObj.Type()
					for sliceFieldIdx := 0; sliceFieldIdx < newSliceObj.NumField(); sliceFieldIdx++ {
						sliceField := newSliceObjType.Field(sliceFieldIdx)
						sliceFieldTag := sliceField.Tag.Get("hl7")
						newVal, err := obj.Get(NewLocation(sliceFieldTag))
						if err != nil {
							return err
						}
						newSliceObj.Field(sliceFieldIdx).SetString(strings.TrimSpace(newVal)) // TODO: support fields other than string
					}
					slice = reflect.Append(slice, newSliceObj)
				}
			}
			st.Field(i).Set(slice)
			continue
		}
		if r != "" {
			if val, _ := m.Find(r); val != "" {
				if st.Field(i).CanSet() {
					// TODO support fields other than string
					//fldT := st.Field(i).Type()
					st.Field(i).SetString(strings.TrimSpace(val))
				}
			}
		}
	}

	return nil
}

func has(i []string, s string) bool {
	for idx := range i {
		if i[idx] == s {
			return true
		}
	}
	return false
}

func (m *Message) unmarshalSlice(it interface{}) error {
	st := reflect.ValueOf(it).Elem()
	stt := st.Type()
	for i := 0; i < st.NumField(); i++ {
		fld := stt.Field(i)
		r := fld.Tag.Get("hl7")
		fmt.Printf("XXX %s %s %s %s\n", r, fld.Name, fld.Type, fld.Type.Kind())
		if fld.Type.Kind() == reflect.Slice {
			fmt.Printf("XXX is array\n")
			slice := reflect.New(fld.Type)
			// m.unmarshalSlice(slice.Pointer())
			st.Field(i).Set(slice)
			continue
		}
		if r != "" {
			if val, _ := m.Find(r); val != "" {
				if st.Field(i).CanSet() {
					// TODO support fields other than string
					//fldT := st.Field(i).Type()
					st.Field(i).SetString(strings.TrimSpace(val))
				}
			}
		}
	}

	return nil
}

// Info returns the MsgInfo for the message
func (m *Message) Info() (MsgInfo, error) {
	mi := MsgInfo{}
	err := m.Unmarshal(&mi)
	return mi, err
}

func (m *Message) ScanSegments() []Segment {
	return m.Segments
}

func (m *Message) FieldStringToComponents(val string) []Component {
	ret := []Component{}
	ret = append(ret, Component{}) // root component
	parts := strings.Split(val, string(m.Delimeters.Component))
	for idx := range parts {
		ret = append(ret, Component{
			SubComponents: nil,
			Value: []rune(parts[idx]),
		})
		fmt.Printf("setting field %i to %s\n", idx, parts[idx])
	}
	return ret
}

// Unmarshal fills a structure from an HL7 message
// It will panic if interface{} is not a pointer to a struct
// Unmarshal will decode the entire message before trying to set values
// it will set the first matching segment / first matching field
// repeating segments and fields is not well suited to this
// for the moment all unmarshal target fields must be strings
func (s Segment) Unmarshal(it interface{}) error {
	st := reflect.ValueOf(it).Elem()
	stt := st.Type()
	for i := 0; i < st.NumField(); i++ {
		fld := stt.Field(i)
		r := fld.Tag.Get("hl7")
		if r != "" {
			if val, _ := s.Find(r); val != "" {
				if st.Field(i).CanSet() {
					// TODO support fields other than string
					//fldT := st.Field(i).Type()
					st.Field(i).SetString(strings.TrimSpace(val))
				}
			}
		}
	}

	return nil
}

// Find gets a value from a segment using location syntax
func (s Segment) Find(loc string) (string, error) {
	return s.Get(NewLocation(loc))
}

func (s Segment) Name() string {
	if len(s.Fields) == 0 {
		return ""
	}
	field, err := s.Field(0)
	if err != nil {
		return ""
	}
	return field.SegName
}
