package shoppinglist

import "testing"

func TestParseItems(t *testing.T) {
	cases := []struct {
		name   string
		values [][]interface{}
		want   []string
	}{
		{"empty sheet", nil, []string{}},
		{
			"plain list",
			[][]interface{}{{"Leche"}, {"Pan"}, {"Huevos"}},
			[]string{"Leche", "Pan", "Huevos"},
		},
		{
			"blank row in the middle",
			[][]interface{}{{"Leche"}, {}, {"Pan"}},
			[]string{"Leche", "Pan"},
		},
		{
			"whitespace-only cell",
			[][]interface{}{{"Leche"}, {"   "}, {"Pan"}},
			[]string{"Leche", "Pan"},
		},
		{
			"leading/trailing whitespace is trimmed",
			[][]interface{}{{"  Leche  "}},
			[]string{"Leche"},
		},
		{
			"non-string cell is skipped",
			[][]interface{}{{"Leche"}, {42}, {"Pan"}},
			[]string{"Leche", "Pan"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseItems(tc.values)
			if len(got) != len(tc.want) {
				t.Fatalf("parseItems() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parseItems() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
