package golevel7

// SubComponent is an HL7 subcomponent
type SubComponent struct {
	Value []rune
}

func (s SubComponent) Get(loc *Location) (string, error) {
	return string(s.Value), nil
}