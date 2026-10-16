// Package schedule validates conversation schedule drafts and computes the next fire time.
package schedule

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

var timeOfDayPattern = regexp.MustCompile(`^\d{2}:\d{2}$`)

// Draft is an in-memory schedule before persistence.
type Draft struct {
	Kind        string
	Instruction string
	Timezone    string
	FireAt      time.Time
	TimeOfDay   string
	Weekday     int
}

// Validate checks draft fields and returns the next run instant in the draft timezone.
func Validate(d Draft, now time.Time) (time.Time, error) {
	if err := validateKind(d.Kind); err != nil {
		return time.Time{}, err
	}
	if _, err := loadLocation(d.Timezone); err != nil {
		return time.Time{}, err
	}
	if err := validateKindFields(d, now); err != nil {
		return time.Time{}, err
	}
	if err := validateInstruction(d.Instruction); err != nil {
		return time.Time{}, err
	}
	return Next(d, now)
}

// Next computes the next trigger instant for a draft. It does not validate instruction length.
func Next(d Draft, now time.Time) (time.Time, error) {
	loc, err := loadLocation(d.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	switch d.Kind {
	case "once":
		return d.FireAt, nil
	case "daily":
		hour, min, err := parseTimeOfDay(d.TimeOfDay)
		if err != nil {
			return time.Time{}, err
		}
		return nextDaily(loc, hour, min, now), nil
	case "weekly":
		hour, min, err := parseTimeOfDay(d.TimeOfDay)
		if err != nil {
			return time.Time{}, err
		}
		if d.Weekday < 1 || d.Weekday > 7 {
			return time.Time{}, errors.New("schedule: weekday must be 1–7")
		}
		return nextWeekly(loc, d.Weekday, hour, min, now), nil
	default:
		return time.Time{}, fmt.Errorf("schedule: unknown kind %q", d.Kind)
	}
}

func validateKind(kind string) error {
	switch kind {
	case "once", "daily", "weekly":
		return nil
	default:
		return fmt.Errorf("schedule: invalid kind %q", kind)
	}
}

func loadLocation(tz string) (*time.Location, error) {
	if strings.TrimSpace(tz) == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, fmt.Errorf("schedule: invalid timezone %q", tz)
	}
	return loc, nil
}

func validateKindFields(d Draft, now time.Time) error {
	switch d.Kind {
	case "once":
		if !d.FireAt.After(now) {
			return errors.New("schedule: once fire_at must be after now")
		}
		return nil
	case "daily", "weekly":
		if _, _, err := parseTimeOfDay(d.TimeOfDay); err != nil {
			return err
		}
		if d.Kind == "weekly" {
			if d.Weekday < 1 || d.Weekday > 7 {
				return errors.New("schedule: weekday must be 1–7")
			}
		}
		return nil
	default:
		return fmt.Errorf("schedule: invalid kind %q", d.Kind)
	}
}

func validateInstruction(instruction string) error {
	trimmed := strings.TrimSpace(instruction)
	if trimmed == "" {
		return errors.New("schedule: instruction required")
	}
	if utf8.RuneCountInString(trimmed) > 2000 {
		return errors.New("schedule: instruction too long")
	}
	return nil
}

func parseTimeOfDay(s string) (hour, min int, err error) {
	if !timeOfDayPattern.MatchString(s) {
		return 0, 0, errors.New("schedule: time_of_day must be HH:MM")
	}
	hour = int(s[0]-'0')*10 + int(s[1]-'0')
	min = int(s[3]-'0')*10 + int(s[4]-'0')
	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return 0, 0, errors.New("schedule: invalid time_of_day")
	}
	return hour, min, nil
}

func nextDaily(loc *time.Location, hour, min int, now time.Time) time.Time {
	nowLoc := now.In(loc)
	today := time.Date(nowLoc.Year(), nowLoc.Month(), nowLoc.Day(), hour, min, 0, 0, loc)
	if today.After(now) {
		return today
	}
	return today.AddDate(0, 0, 1)
}

func nextWeekly(loc *time.Location, weekday, hour, min int, now time.Time) time.Time {
	target := specWeekdayToGo(weekday)
	nowLoc := now.In(loc)
	start := time.Date(nowLoc.Year(), nowLoc.Month(), nowLoc.Day(), hour, min, 0, 0, loc)
	for delta := 0; delta < 14; delta++ {
		cand := start.AddDate(0, 0, delta)
		if cand.Weekday() == target && cand.After(now) {
			return cand
		}
	}
	return start.AddDate(0, 0, 7)
}

func specWeekdayToGo(weekday int) time.Weekday {
	if weekday == 7 {
		return time.Sunday
	}
	return time.Weekday(weekday)
}
