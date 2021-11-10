package golevel7

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
)

// Encoder writes hl7 messages to a stream
type Encoder struct {
	w io.Writer
}

// NewEncoder returns a new Encoder that writes to stream w
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the encoding of it to the stream
// It will panic if interface{} is not a pointer to a struct
func (e *Encoder) Encode(it interface{}) error {
	msg := &Message{}
	b, err := Marshal(msg, it)
	if err != nil {
		return err
	}
	i, err := e.w.Write(b)
	if err != nil {
		return err
	}
	if i < len(b) {
		return errors.New("Failed to write all bytes")
	}
	return nil
}

// Marshal will insert values into a message
// It will panic if interface{} is not a pointer to a struct
func Marshal(m *Message, it interface{}) ([]byte, error) {
	st := reflect.ValueOf(it).Elem()
	stt := st.Type()
	repeating := false
	for i := 0; i < st.NumField(); i++ {
		fld := stt.Field(i)
		r := fld.Tag.Get("hl7")
		parts := strings.Split(r, ",")
		if len(parts) == 2 {
			if parts[1] == "repeating" {
				repeating = true
				s := Segment{}
				s.forceField([]rune(parts[0]), 0)
				m.Segments = append(m.Segments, s)
			}
		} else {
			if repeating {
				// for repeating fields we need to set the last item in the segment which will be the new segment
				l := fixZeroOffset(NewLocation(r))
				val := st.Field(i).String()
				if err := m.SetLast(l, val); err != nil {
					return nil, err
				}
			} else if r != "" {
				l := fixZeroOffset(NewLocation(r))
				val := st.Field(i).String()
				if err := m.Set(l, val); err != nil {
					return nil, err
				}
			}
		}
	}

	msg := []byte(string(m.Value))
	msh := append([]byte("MSH"), byte(m.Delimeters.Field))
	delims := append(msh, []byte(m.Delimeters.DelimeterField+string(m.Delimeters.Field))...)
	msg = bytes.ReplaceAll(msg, msh, delims)
	m.Value = []rune(string(msg))

	return msg, nil
}

// Compensate for the fact that marshall should use non-zero offsets for the component and subcomponent locations.
func fixZeroOffset(location *Location) *Location {
	newLoc := &Location{
		Segment:  location.Segment,
		FieldSeq: location.FieldSeq,
		Comp:     -1,
		SubComp:  -1,
	}
	if location.Comp > 0 {
        newLoc.Comp = location.Comp - 1
    }
	if location.SubComp > 0 {
        newLoc.SubComp = location.SubComp - 1
    }
	return newLoc
}
