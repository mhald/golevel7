package golevel7

import (
	"strconv"
	"strings"
)

// SubComponent is an HL7 subcomponent
type SubComponent struct {
	Value []rune
}

func (s SubComponent) GetAll(loc *Location) ([]string, error) {
	return []string{string(s.Value)}, nil
}

func (s SubComponent) Get(loc *Location) (string, error) {
	return string(s.Value), nil
}

func (s SubComponent) RelativeLocation(tag string) *Location {
	location := &Location{}
	parts := strings.Split(tag, ".")
	parts = reverse(parts[1:])
	for i, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			continue
		}
		switch i {
		case 0:
			location.SubComp = number
		case 1:
			location.Comp = number
		case 2:
			location.FieldSeq = number
		}
	}
	return location
}

func reverse(parts []string) []string {
	for i := 0; i < len(parts)/2; i++ {
		j := len(parts) - i - 1
		parts[i], parts[j] = parts[j], parts[i]
	}
	return parts
}