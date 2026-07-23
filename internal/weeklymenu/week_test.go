package weeklymenu

import (
	"testing"
	"time"
)

// weekFixture is a representative 12-row grid: row 0 is the header
// (Monday-first), rows 1-5 are lunch, row 6 is the blank spacer, rows
// 7-11 are dinner. It intentionally includes a short row, a blank cell,
// and a non-string cell to exercise collectColumn's tolerance.
func weekFixture() [][]interface{} {
	return [][]interface{}{
		{"Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"}, // row 0: header
		{"Lentejas", "Pasta", "Arroz", "Pizza", "Sopa", "Paella", "Cocido"},
		{"Ensalada", "", "Pollo", "  ", "", "", ""},
		{"Tortilla"}, // short row: only column 0 has a value
		{},           // blank row
		{"", "", "", "", "", "", "Fideuá"},
		{}, // row 6: blank spacer, never read
		{"Tortilla", "Sopa", "Sopa", "Pizza", "Pizza", "Pizza", "Cocido"}, // row 7: dinner start
		{"Fruta", "", "", "", "", "", ""},
		{},
		{42, "", "", "", "", "", ""}, // non-string cell
		{"", "", "", "", "", "", ""}, // row 11: dinner end, all blank
	}
}

func TestParseWeekRotation(t *testing.T) {
	cases := []struct {
		name      string
		today     time.Weekday
		wantOrder []string
	}{
		{"Monday, no rotation", time.Monday, []string{"Lun", "Mar", "Mié", "Jue", "Vie", "Sáb", "Dom"}},
		{"Wednesday", time.Wednesday, []string{"Mié", "Jue", "Vie", "Sáb", "Dom", "Lun", "Mar"}},
		{"Sunday wraps around", time.Sunday, []string{"Dom", "Lun", "Mar", "Mié", "Jue", "Vie", "Sáb"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			days := parseWeek(weekFixture(), tc.today)
			if len(days) != weekColumns {
				t.Fatalf("len(days) = %d, want %d", len(days), weekColumns)
			}
			for i, day := range days {
				if day.Label != tc.wantOrder[i] {
					t.Fatalf("days[%d].Label = %q, want %q", i, day.Label, tc.wantOrder[i])
				}
			}
		})
	}
}

func TestParseWeekEntries(t *testing.T) {
	days := parseWeek(weekFixture(), time.Monday)

	monday := days[0]
	if got, want := monday.Lunch, []string{"Lentejas", "Ensalada", "Tortilla"}; !equalSlices(got, want) {
		t.Fatalf("Monday.Lunch = %v, want %v", got, want)
	}
	if got, want := monday.Dinner, []string{"Tortilla", "Fruta"}; !equalSlices(got, want) {
		t.Fatalf("Monday.Dinner = %v, want %v", got, want)
	}

	tuesday := days[1]
	if got, want := tuesday.Lunch, []string{"Pasta"}; !equalSlices(got, want) {
		t.Fatalf("Tuesday.Lunch = %v, want %v (blank cell and short row should be skipped)", got, want)
	}

	sunday := days[6]
	if got, want := sunday.Lunch, []string{"Cocido", "Fideuá"}; !equalSlices(got, want) {
		t.Fatalf("Sunday.Lunch = %v, want %v", got, want)
	}
	if got, want := sunday.Dinner, []string{"Cocido"}; !equalSlices(got, want) {
		t.Fatalf("Sunday.Dinner = %v, want %v (blank/non-string/missing rows dropped)", got, want)
	}
}

func TestParseWeekEmptyInput(t *testing.T) {
	days := parseWeek(nil, time.Monday)
	if len(days) != weekColumns {
		t.Fatalf("len(days) = %d, want %d", len(days), weekColumns)
	}
	for i, day := range days {
		if day.Label != "" || len(day.Lunch) != 0 || len(day.Dinner) != 0 {
			t.Fatalf("days[%d] = %+v, want zero value", i, day)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
