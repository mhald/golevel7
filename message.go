package golevel7

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"reflect"
	"strconv"
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

// Segment returns the first matching segment with name s
func (m *Message) Segment(s string) (*Segment, error) {
	for i, seg := range m.Segments {
		fld := seg.Field(0)
		if fld == nil {
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
		fld := seg.Field(0)
		if fld == nil {
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
		fld := seg.Field(0)
		if fld == nil {
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
	GetAll(loc *Location) ([]string, error)
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

var stringArray []string

func (m *Message) ToStruct(v interface{}) error {
	return unmarshalMessageStruct(reflect.ValueOf(v), m)
}


// levels of a HL7 message are as follows:
// - message
// - segment
// - field
// - component
// - subcomponent

func unmarshalMessageStruct(v reflect.Value, m *Message) error {
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Ensure we're dealing with a struct
	if v.Kind() != reflect.Struct {
		return nil // Or an appropriate error
	}

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := v.Type().Field(i)
		hl7Tag := fieldType.Tag.Get("hl7")

		if hl7Tag != "" {
			segmentName := strings.Split(hl7Tag, ".")[0]

			segments, err := m.AllSegments(segmentName)
			if err != nil {
				continue
			}

			for _, segment := range segments {
				// For simple string fields, just find and set
				if field.Kind() == reflect.String {
					if val, _ := segment.Find(hl7Tag); val != "" {
						field.SetString(strings.TrimSpace(val))
					}
				} else if field.Kind() == reflect.Struct {
					// Recurse for nested structs
					err := unmarshalSegmentStruct(field.Addr(), segment)
					if err != nil {
						return err
					}
				} else if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.Struct {
					// Handle slice of structs
					elementType := field.Type().Elem()
					for {
						// Assuming a way to iterate over or determine the correct segment(s) for this slice
						// This part is highly dependent on your data structure and HL7 message format
						newElementPtr := reflect.New(elementType)
						err := unmarshalSegmentStruct(newElementPtr, segment)
						if err != nil {
							break // or handle the error as needed
						}
						field.Set(reflect.Append(field, newElementPtr.Elem()))
						break
					}
				}
			}
		}
	}
	return nil
}

func unmarshalSegmentStruct(addr reflect.Value, s *Segment) error {
	for i := 0; i < addr.Elem().NumField(); i++ {
		field := addr.Elem().Field(i)
		fieldType := addr.Elem().Type().Field(i)
		hl7Tag := fieldType.Tag.Get("hl7")

		if hl7Tag != "" {
			// For simple string fields, just find and set
			if field.Kind() == reflect.String {
				if val, _ := s.Find(hl7Tag); val != "" {
					field.SetString(strings.TrimSpace(val))
				}
			} else {
				// get the last . part of the tag 
				parts := strings.Split(hl7Tag, ".")
				addrFieldIdString := parts[len(parts)-1]
				addrFieldId, err := strconv.Atoi(addrFieldIdString)
				if err != nil {
					continue
				}
				allFields, err := s.AllFields(addrFieldId)
				if err != nil {
					continue
				}

				for _, f := range allFields {
					if field.Kind() == reflect.Struct {
						// Recurse for nested structs
						err = unmarshalFieldStruct(field.Addr(), f)
						if err != nil {
							return err
						}
					} else if field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.Struct {
						// Handle slice of structs
						elementType := field.Type().Elem()

						for {
							// Assuming a way to iterate over or determine the correct segment(s) for this slice
							// This part is highly dependent on your data structure and HL7 message format
							newElementPtr := reflect.New(elementType)
							err := unmarshalFieldStruct(newElementPtr, f)
							if err != nil {
								break // or handle the error as needed
							}
							field.Set(reflect.Append(field, newElementPtr.Elem()))
							break
						}
					}
				}
			}
		}
	}
	return nil
}

func unmarshalFieldStruct(ptr reflect.Value, f *Field) error {
	for i := 0; i < ptr.Elem().NumField(); i++ {
		component := ptr.Elem().Field(i)
		componentType := ptr.Elem().Type().Field(i)
		hl7Tag := componentType.Tag.Get("hl7")

		if hl7Tag != "" {
			// For simple string fields, just find and set
			if component.Kind() == reflect.String {
				location := f.RelativeLocation(hl7Tag)
				if val, _ := f.Get(location); val != "" {
					component.SetString(strings.TrimSpace(val))
				}
			} else {
				// get the last . part of the tag
				parts := strings.Split(hl7Tag, ".")
				addrFieldIdString := parts[len(parts)-1]
				addrFieldId, err := strconv.Atoi(addrFieldIdString)
				if err != nil {
					continue
				}

				c, err := f.Component(addrFieldId)
				if err != nil {
					continue
				}

				if component.Kind() == reflect.Struct {
					// Recurse for nested structs
					err = unmarshalComponentStruct(component.Addr(), c)
					if err != nil {
						return err
					}
				} else if component.Kind() == reflect.Slice && component.Type().Elem().Kind() == reflect.Struct {
					// Handle slice of structs
					elementType := component.Type().Elem()

					for {
						// Assuming a way to iterate over or determine the correct segment(s) for this slice
						// This part is highly dependent on your data structure and HL7 message format
						newElementPtr := reflect.New(elementType)
						err := unmarshalComponentStruct(newElementPtr, c)
						if err != nil {
							break // or handle the error as needed
						}
						component.Set(reflect.Append(component, newElementPtr.Elem()))
						break
					}
				}
			}
		}
	}
	return nil
}

func unmarshalComponentStruct(addr reflect.Value, c *Component) error {
	for i := 0; i < addr.Elem().NumField(); i++ {
		subcomponent := addr.Elem().Field(i)
		subcomponentType := addr.Elem().Type().Field(i)
		hl7Tag := subcomponentType.Tag.Get("hl7")

		if hl7Tag != "" {
			// For simple string fields, just find and set
			if subcomponent.Kind() == reflect.String {
				location := c.RelativeLocation(hl7Tag)
				if val, _ := c.Get(location); val != "" {
					subcomponent.SetString(strings.TrimSpace(val))
				}
			} else {
				// get the last . part of the tag
				parts := strings.Split(hl7Tag, ".")
				addrFieldIdString := parts[len(parts)-1]
				addrFieldId, err := strconv.Atoi(addrFieldIdString)
				if err != nil {
					continue
				}
				sc, err := c.SubComponent(addrFieldId)
				if err != nil {
					continue
				}

				if subcomponent.Kind() == reflect.Struct {
					// Recurse for nested structs
					err = unmarshalSubComponentStruct(subcomponent.Addr(), sc)
					if err != nil {
						return err
					}
				} else if subcomponent.Kind() == reflect.Slice && subcomponent.Type().Elem().Kind() == reflect.Struct {
					// Handle slice of structs
					elementType := subcomponent.Type().Elem()

					for {
						// Assuming a way to iterate over or determine the correct segment(s) for this slice
						// This part is highly dependent on your data structure and HL7 message format
						newElementPtr := reflect.New(elementType)
						err := unmarshalSubComponentStruct(newElementPtr, sc)
						if err != nil {
							break // or handle the error as needed
						}
						subcomponent.Set(reflect.Append(subcomponent, newElementPtr.Elem()))
						break
					}
				}
			}
		}
	}
	return nil
}

func unmarshalSubComponentStruct(ptr reflect.Value, sc *SubComponent) error {
	for i := 0; i < ptr.Elem().NumField(); i++ {
		field := ptr.Elem().Field(i)
		fieldType := ptr.Elem().Type().Field(i)
		hl7Tag := fieldType.Tag.Get("hl7")

		if hl7Tag != "" {
			// For simple string fields, just find and set
			if field.Kind() == reflect.String {
				location := sc.RelativeLocation(hl7Tag)
				if val, _ := sc.Get(location); val != "" {
					field.SetString(strings.TrimSpace(val))
				}
			}
		}
	}
	return nil
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

		if !st.Field(i).CanSet() {
			continue
		}

		r := fld.Tag.Get("hl7")
		if r == "" {
			continue
		}

		if st.Field(i).Kind() == reflect.String {
			if val, _ := m.Find(r); val != "" {
				st.Field(i).SetString(strings.TrimSpace(val))
			}
			continue
		}

		if st.Field(i).Kind() == reflect.Slice {
			if fld.Type == reflect.TypeOf(stringArray) {
				location := strings.Split(r, ",")[0]
				vals, err := m.GetAll(NewLocation(location))
				if err != nil {
					return err
				}
				stringSlice := reflect.MakeSlice(reflect.TypeOf(stringArray), len(vals), len(vals))
				for idx := range vals {
					stringSlice.Index(idx).Set(reflect.ValueOf(strings.TrimSpace(vals[idx])))
				}
				st.Field(i).Set(stringSlice)
				continue
			}
		}

		if st.Field(i).Type().Kind() == reflect.Slice {
			// TODO: add check to ensure that the slice is a struct and not a string (which is handled above)
			// TODO: recurse back into this function to allow arbitrary nesting of structs

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
			slice := reflect.MakeSlice(st.Field(i).Type(), 0, 0)
			valuePtr := reflect.New(st.Field(i).Type().Elem()).Elem()
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

						location := strings.Split(sliceFieldTag, ",")[0]
						if sliceField.Type.Kind() == reflect.String {
							newVal, err := obj.Get(NewLocation(location))
							if err != nil {
								return err
							}
							newSliceObj.Field(sliceFieldIdx).SetString(strings.TrimSpace(newVal)) // TODO: support fields other than string
							continue
						}

						if sliceField.Type.Kind() == reflect.Slice {
							if reflect.SliceOf(sliceField.Type.Elem()) == reflect.TypeOf(stringArray) {
								vals, err := obj.GetAll(NewLocation(location))
								if err != nil {
									return err
								}
								stringSlice := reflect.MakeSlice(reflect.TypeOf(stringArray), len(vals), len(vals))
								for idx := range vals {
									stringSlice.Index(idx).Set(reflect.ValueOf(strings.TrimSpace(vals[idx])))
								}
								newSliceObj.Field(sliceFieldIdx).Set(stringSlice)
							}
							continue
						}
					}
					slice = reflect.Append(slice, newSliceObj)
				}
			}
			st.Field(i).Set(slice)
			continue
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
					fldT := st.Field(i).Type()
					switch fldT.Kind() {
					case reflect.String:
						st.Field(i).SetString(strings.TrimSpace(val))
					// detect if this is a slice or a pointer to a slice
					case reflect.Slice:
						obj := reflect.New(fldT.Elem())
						st.Field(i).Set(obj)
					case reflect.Ptr:
						// instantiate a new slice and set the pointer to it
						obj := reflect.New(fldT.Elem())
						st.Field(i).Set(obj)
					default:
						// TODO: support other types
					}
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
	field:= s.Field(0)
	if field == nil {
		return ""
	}
	return field.SegName
}
