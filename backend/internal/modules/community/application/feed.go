package application

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

// FeedFilter scopes comparable current community prices to one canonical city.
// No coordinates or contributor identifier participates in this public read.
type FeedFilter struct {
	State, Municipality, Product, Unit, Order string
	Limit                                     int
	HasCursor                                 bool
	AfterTime                                 time.Time
	AfterID                                   string
	AfterAmount                               int64
}

var ErrFeedFilter = errors.New("community: invalid feed filter")
var feedCity = regexp.MustCompile(`^[0-9]{7}$`)
var feedID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

func ValidateFeed(state, city, fuel, order string, limit int, key string) (FeedFilter, error) {
	units := map[string]string{"ETHANOL": "L", "GASOLINE_REGULAR": "L", "GASOLINE_ADDITIVED": "L", "DIESEL_S500": "L", "DIESEL_S10": "L", "CNG": "M3", "LPG_P13": "KG_13"}
	states := "|AC|AL|AP|AM|BA|CE|DF|ES|GO|MA|MT|MS|MG|PA|PB|PR|PE|PI|RJ|RN|RS|RO|RR|SC|SP|SE|TO|"
	if len(state) != 2 || !strings.Contains(states, "|"+state+"|") || !feedCity.MatchString(city) || units[fuel] == "" || (order != "recent" && order != "cheapest") || limit < 1 || limit > 50 {
		return FeedFilter{}, ErrFeedFilter
	}
	f := FeedFilter{State: state, Municipality: city, Product: fuel, Unit: units[fuel], Order: order, Limit: limit}
	if key != "" {
		var k feedPosition
		if json.Unmarshal([]byte(key), &k) != nil || k.Time.IsZero() || !feedID.MatchString(k.ID) || k.Amount < 1 || k.Amount > 1000000 {
			return FeedFilter{}, ErrFeedFilter
		}
		f.HasCursor = true
		f.AfterTime = k.Time
		f.AfterID = k.ID
		f.AfterAmount = k.Amount
	}
	return f, nil
}

type feedPosition struct {
	Time   time.Time `json:"time"`
	ID     string    `json:"id"`
	Amount int64     `json:"amount"`
}

func FeedKey(at time.Time, id string, amount int64) string {
	raw, _ := json.Marshal(feedPosition{at, id, amount})
	return string(raw)
}
