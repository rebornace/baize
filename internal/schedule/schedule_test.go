package schedule_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/schedule"
)

func TestNextDailyWhenTimePassed(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 16, 15, 0, 0, 0, loc)
	got, err := schedule.Next(schedule.Draft{
		Kind: "daily", TimeOfDay: "09:00", Timezone: "Asia/Shanghai",
	}, now)
	want := time.Date(2026, 10, 17, 9, 0, 0, 0, loc)
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestNextWeeklyWhenThisWeekPassed(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 16, 15, 0, 0, 0, loc) // Friday
	got, err := schedule.Next(schedule.Draft{
		Kind: "weekly", Weekday: 1, TimeOfDay: "09:00", Timezone: "Asia/Shanghai",
	}, now)
	want := time.Date(2026, 10, 19, 9, 0, 0, 0, loc) // next Monday
	if err != nil || !got.Equal(want) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestNextOnceIsFireAt(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	fire := time.Date(2026, 10, 17, 9, 0, 0, 0, loc)
	got, err := schedule.Next(schedule.Draft{
		Kind: "once", FireAt: fire, Timezone: "Asia/Shanghai",
	}, time.Date(2026, 10, 16, 15, 0, 0, 0, loc))
	if err != nil || !got.Equal(fire) {
		t.Fatalf("got %v err %v", got, err)
	}
}

func TestRejectBadTimezoneAndPastOnce(t *testing.T) {
	now := time.Date(2026, 10, 16, 15, 0, 0, 0, time.UTC)
	if _, err := schedule.Validate(schedule.Draft{Kind: "daily", TimeOfDay: "09:00", Timezone: "Not/AZone"}, now); err == nil {
		t.Fatal("bad timezone")
	}
	if _, err := schedule.Validate(schedule.Draft{
		Kind: "once", FireAt: now.Add(-time.Minute), Timezone: "UTC", Instruction: "提醒",
	}, now); err == nil {
		t.Fatal("past once")
	}
}
