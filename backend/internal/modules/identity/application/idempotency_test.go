package application

import (
	"testing"
)

func TestDecideTruthTable(t *testing.T) {
	cases := []struct {
		name                      string
		found, completed, expired bool
		sameHash                  bool
		want                      Action
	}{
		{"absent executes", false, false, false, false, ActionExecute},
		{"absent ignores rest", false, true, true, true, ActionExecute},
		{"completed same replays", true, true, false, true, ActionReplay},
		{"completed changed conflicts", true, true, false, false, ActionConflict},
		{"expired reserves", true, true, true, true, ActionReserve},
		{"expired changed reserves", true, true, true, false, ActionReserve},
		{"unfinished reserves", true, false, false, true, ActionReserve},
		{"unfinished changed reserves", true, false, false, false, ActionReserve},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Decide(c.found, c.completed, c.expired, c.sameHash); got != c.want {
				t.Errorf("got %d, want %d", got, c.want)
			}
		})
	}
}
