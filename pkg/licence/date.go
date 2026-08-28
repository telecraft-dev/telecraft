package licence

import (
	"encoding/json"
	"errors"
	"time"
)

// Date is a whole day, which is the only precision a licence has. A
// licence covers days rather than moments: the day it is issued and the
// day it expires are both covered in full, wherever the host's clock sits
// inside them.
//
// The zero Date is no date at all, which a verified document is refused
// for.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// layout is how a date is written in the document and read back: the one
// unambiguous ordering.
const layout = "2006-01-02"

// DateOf is the day a moment falls on, read in UTC. Dates are judged
// against the host clock, and reading that clock in one zone everywhere
// keeps two Instances of one deployment reading one licence the same way.
func DateOf(t time.Time) Date {
	t = t.UTC()
	return Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
}

// ParseDate reads a date as it is written in a licence document.
func ParseDate(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, errors.New("the date " + s + " is not a date written as " + layout)
	}
	return DateOf(t), nil
}

// String writes the date as the document carries it.
func (d Date) String() string {
	if d.zero() {
		return ""
	}
	return d.time().Format(layout)
}

// Written is the date as a person reads it, for a surface: 3 March 2027.
func (d Date) Written() string {
	if d.zero() {
		return ""
	}
	return d.time().Format("2 January 2006")
}

// Before reports whether d falls on an earlier day than other.
func (d Date) Before(other Date) bool { return d.time().Before(other.time()) }

func (d Date) zero() bool { return d == Date{} }

func (d Date) time() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}

// MarshalJSON writes the date as a string, so the document reads as a
// document to whoever opens the file.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON reads it back. A date the document cannot express is a
// document this build does not read, which is the unreadable state.
func (d *Date) UnmarshalJSON(raw []byte) error {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	if s == "" {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
