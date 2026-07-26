package weather

import "testing"

func TestIconKeyForKnownCodes(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{0, "clear"},
		{1, "clear"},
		{2, "partly-cloudy"},
		{3, "cloudy"},
		{45, "fog"},
		{48, "fog"},
		{51, "rain"},
		{56, "rain"},
		{61, "rain"},
		{67, "rain"},
		{80, "rain"},
		{82, "rain"},
		{71, "snow"},
		{77, "snow"},
		{85, "snow"},
		{86, "snow"},
		{95, "storm"},
		{96, "storm"},
		{99, "storm"},
	}
	for _, tc := range cases {
		if got := IconKeyFor(tc.code); got != tc.want {
			t.Errorf("IconKeyFor(%d) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

func TestIconKeyForUnknownCodeFallsBackToCloudy(t *testing.T) {
	if got := IconKeyFor(-1); got != "cloudy" {
		t.Errorf("IconKeyFor(-1) = %q, want %q", got, "cloudy")
	}
}
